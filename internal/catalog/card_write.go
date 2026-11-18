// Создание/обновление/удаление карточек (наполнение каталога).
package catalog

import (
	"database/sql"
	"errors"
	"fmt"
)

// UpsertCardCard — данные карточки для создания/обновления (без id и собранных тем).
type UpsertCardCard struct {
	LanguageCode    string
	LevelCode       string
	SourceText      string
	TargetText      string
	Note            string
	GrammarTopicIDs []int64
	LexicaTopicIDs  []int64
}

// UpsertCard создаёт карточку (id == 0) или обновляет существующую (id != 0).
// Связи с темами полностью перезаписываются присланными списками.
func (r *Repo) UpsertCard(id int64, c UpsertCardCard) (Card, error) {
	if c.LanguageCode == "" || c.LevelCode == "" || c.SourceText == "" || c.TargetText == "" {
		return Card{}, errors.Join(ErrInvalid, fmt.Errorf("language_code, level_code, source_text и target_text обязательны"))
	}

	var (
		res   sql.Result
		isNew = id == 0
		err   error
	)
	if isNew {
		res, err = r.db.Exec(`
			INSERT INTO card (language_id, level_id, source_text, target_text, note)
			VALUES ((SELECT id FROM language WHERE code = ?),
			        (SELECT id FROM level WHERE code = ?), ?, ?, ?)`,
			c.LanguageCode, c.LevelCode, c.SourceText, c.TargetText, c.Note)
	} else {
		res, err = r.db.Exec(`
			UPDATE card SET
				language_id = (SELECT id FROM language WHERE code = ?),
				level_id = (SELECT id FROM level WHERE code = ?),
				source_text = ?, target_text = ?, note = ?
			WHERE id = ?`,
			c.LanguageCode, c.LevelCode, c.SourceText, c.TargetText, c.Note, id)
	}
	if err != nil {
		return Card{}, fmt.Errorf("upsert card: %w", err)
	}
	if !isNew {
		if n, _ := res.RowsAffected(); n == 0 {
			return Card{}, ErrNotFound
		}
	}
	if isNew {
		id, err = res.LastInsertId()
		if err != nil {
			return Card{}, fmt.Errorf("last insert id: %w", err)
		}
	}

	if err := r.replaceTopics(id, c); err != nil {
		return Card{}, err
	}

	// Подложечные ID тем могут не существовать — INSERT ... SELECT ничего не вставит
	// молча. Проверим количество связей.
	if err := r.verifyTopicLinks(id, c); err != nil {
		return Card{}, err
	}

	card, err := r.GetCard(id)
	if err != nil {
		return Card{}, err
	}
	return *card, nil
}

// replaceTopics перезаписывает M2M-связи карточки с темами.
func (r *Repo) replaceTopics(cardID int64, c UpsertCardCard) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, table := range []struct {
		link, topic, col string
		ids              []int64
	}{
		{"card_grammar_topic", "grammar_topic", "grammar_topic_id", c.GrammarTopicIDs},
		{"card_lexica_topic", "lexica_topic", "lexica_topic_id", c.LexicaTopicIDs},
	} {
		if _, err := tx.Exec(`DELETE FROM `+table.link+` WHERE card_id = ?`, cardID); err != nil {
			return fmt.Errorf("clear %s: %w", table.link, err)
		}
		for _, topicID := range table.ids {
			if _, err := tx.Exec(
				`INSERT INTO `+table.link+` (card_id, `+table.col+`)
				 SELECT ?, id FROM `+table.topic+` WHERE id = ? AND language_id =
				   (SELECT language_id FROM card WHERE id = ?)`,
				cardID, topicID, cardID); err != nil {
				return fmt.Errorf("link %s: %w", table.link, err)
			}
		}
	}
	return tx.Commit()
}

// verifyTopicLinks убеждается, что все присланные темы реально привязались
// (защита от несуществующих topic_id или тем другого языка).
func (r *Repo) verifyTopicLinks(cardID int64, c UpsertCardCard) error {
	var nGram, nLex int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM card_grammar_topic WHERE card_id = ?`, cardID).Scan(&nGram); err != nil {
		return fmt.Errorf("verify grammar links: %w", err)
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM card_lexica_topic WHERE card_id = ?`, cardID).Scan(&nLex); err != nil {
		return fmt.Errorf("verify lexica links: %w", err)
	}
	if nGram != len(dedup(c.GrammarTopicIDs)) || nLex != len(dedup(c.LexicaTopicIDs)) {
		return errors.Join(ErrInvalid, fmt.Errorf("часть topic_id не существует или относится к другому языку"))
	}
	return nil
}

// GetCard возвращает карточку по id.
func (r *Repo) GetCard(id int64) (*Card, error) {
	var c Card
	err := r.db.QueryRow(`
		SELECT c.id, lg.code, lv.code, c.source_text, c.target_text, c.note
		FROM card c
		JOIN language lg ON lg.id = c.language_id
		JOIN level lv ON lv.id = c.level_id
		WHERE c.id = ?`, id).
		Scan(&c.ID, &c.LanguageCode, &c.LevelCode, &c.SourceText, &c.TargetText, &c.Note)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get card: %w", err)
	}
	cards := []Card{c}
	if err := r.attachTopics(cards); err != nil {
		return nil, err
	}
	return &cards[0], nil
}

// DeleteCard удаляет карточку; возвращает ErrNotFound, если её не было.
func (r *Repo) DeleteCard(id int64) error {
	res, err := r.db.Exec(`DELETE FROM card WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete card: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func dedup(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
