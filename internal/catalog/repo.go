// Репозиторий каталога: SQL-запросы поверх SQLite.
// Формат ошибок: ErrNotFound — сущность не найдена, остальные — обёрнутые ошибки SQL.
package catalog

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound — сущность не найдена (маппится в gRPC NotFound).
var ErrNotFound = errors.New("not found")

// ErrInvalid — некорректные данные (маппится в gRPC InvalidArgument).
var ErrInvalid = errors.New("invalid argument")

// Repo — доступ к SQLite. Один *sql.DB принадлежит только процессу api.
type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// ListLanguages возвращает целевые языки, отсортированные по коду.
func (r *Repo) ListLanguages() ([]Language, error) {
	rows, err := r.db.Query(`SELECT code, name FROM language ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("list languages: %w", err)
	}
	defer rows.Close()

	var langs []Language
	for rows.Next() {
		var l Language
		if err := rows.Scan(&l.Code, &l.Name); err != nil {
			return nil, fmt.Errorf("scan language: %w", err)
		}
		langs = append(langs, l)
	}
	return langs, rows.Err()
}

// ListLevels возвращает уровни CEFR, отсортированные по sort_order.
func (r *Repo) ListLevels() ([]Level, error) {
	rows, err := r.db.Query(`SELECT code, sort_order FROM level ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("list levels: %w", err)
	}
	defer rows.Close()

	var levels []Level
	for rows.Next() {
		var l Level
		if err := rows.Scan(&l.Code, &l.SortOrder); err != nil {
			return nil, fmt.Errorf("scan level: %w", err)
		}
		levels = append(levels, l)
	}
	return levels, rows.Err()
}

// grammarTopics возвращает грамматические темы языка; при непустом levelCode —
// темы этого уровня плюс темы без привязки к уровню.
func (r *Repo) grammarTopics(langCode, levelCode string) ([]GrammarTopic, error) {
	q := `SELECT gt.id, gt.title, COALESCE(l.code, '')
	      FROM grammar_topic gt
	      JOIN language lg ON lg.id = gt.language_id
	      LEFT JOIN level l ON l.id = gt.level_id
	      WHERE lg.code = ?`
	args := []any{langCode}
	if levelCode != "" {
		q += ` AND (l.code = ? OR l.code IS NULL)`
		args = append(args, levelCode)
	}
	q += ` ORDER BY gt.title`

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list grammar topics: %w", err)
	}
	defer rows.Close()

	var topics []GrammarTopic
	for rows.Next() {
		var t GrammarTopic
		if err := rows.Scan(&t.ID, &t.Title, &t.LevelCode); err != nil {
			return nil, fmt.Errorf("scan grammar topic: %w", err)
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

// lexicaTopics возвращает лексические темы языка.
func (r *Repo) lexicaTopics(langCode string) ([]LexicaTopic, error) {
	rows, err := r.db.Query(`
		SELECT lt.id, lt.title
		FROM lexica_topic lt
		JOIN language lg ON lg.id = lt.language_id
		WHERE lg.code = ?
		ORDER BY lt.title`, langCode)
	if err != nil {
		return nil, fmt.Errorf("list lexica topics: %w", err)
	}
	defer rows.Close()

	var topics []LexicaTopic
	for rows.Next() {
		var t LexicaTopic
		if err := rows.Scan(&t.ID, &t.Title); err != nil {
			return nil, fmt.Errorf("scan lexica topic: %w", err)
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

// DrawCards возвращает count случайных карточек по фильтру (без повторов)
// и общее число карточек, подходящих под фильтр.
func (r *Repo) DrawCards(f DrawFilter) ([]Card, int, error) {
	if err := f.Validate(); err != nil {
		return nil, 0, errors.Join(ErrInvalid, err)
	}

	where, args, err := drawWhere(f)
	if err != nil {
		return nil, 0, errors.Join(ErrInvalid, err)
	}

	var available int
	if err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM card c
		JOIN language lg ON lg.id = c.language_id
		JOIN level lv ON lv.id = c.level_id
		`+where, args...).Scan(&available); err != nil {
		return nil, 0, fmt.Errorf("count cards: %w", err)
	}
	if available == 0 {
		return []Card{}, 0, nil
	}

	rows, err := r.db.Query(`
		SELECT c.id, lg.code, lv.code, c.source_text, c.target_text, c.note
		FROM card c
		JOIN language lg ON lg.id = c.language_id
		JOIN level lv ON lv.id = c.level_id
		`+where+`
		ORDER BY RANDOM()
		LIMIT ?`, append(args, f.Count)...)
	if err != nil {
		return nil, 0, fmt.Errorf("draw cards: %w", err)
	}
	defer rows.Close()

	cards := []Card{}
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.LanguageCode, &c.LevelCode, &c.SourceText, &c.TargetText, &c.Note); err != nil {
			return nil, 0, fmt.Errorf("scan card: %w", err)
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows cards: %w", err)
	}

	if err := r.attachTopics(cards); err != nil {
		return nil, 0, err
	}
	return cards, available, nil
}

// drawWhere собирает WHERE-условия для DrawCards. Возвращает ошибку,
// если список topic_ids некорректен.
func drawWhere(f DrawFilter) (string, []any, error) {
	var conds []string
	args := []any{f.LanguageCode}

	conds = append(conds, `lg.code = ?`)
	if f.LevelCode != "" {
		conds = append(conds, `lv.code = ?`)
		args = append(args, f.LevelCode)
	}
	if len(f.GrammarTopicIDs) > 0 {
		ph, a, err := placeholders(f.GrammarTopicIDs)
		if err != nil {
			return "", nil, err
		}
		conds = append(conds, `EXISTS (SELECT 1 FROM card_grammar_topic cgt
			WHERE cgt.card_id = c.id AND cgt.grammar_topic_id IN (`+ph+`))`)
		args = append(args, a...)
	}
	if len(f.LexicaTopicIDs) > 0 {
		ph, a, err := placeholders(f.LexicaTopicIDs)
		if err != nil {
			return "", nil, err
		}
		conds = append(conds, `EXISTS (SELECT 1 FROM card_lexica_topic clt
			WHERE clt.card_id = c.id AND clt.lexica_topic_id IN (`+ph+`))`)
		args = append(args, a...)
	}
	return ` WHERE ` + strings.Join(conds, ` AND `), args, nil
}

func placeholders(ids []int64) (string, []any, error) {
	if len(ids) == 0 {
		return "", nil, fmt.Errorf("пустой список topic_ids")
	}
	var b strings.Builder
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		if id <= 0 {
			return "", nil, fmt.Errorf("некорректный topic id: %d", id)
		}
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
		args = append(args, id)
	}
	return b.String(), args, nil
}

// attachTopics дозагружает темы для списка карточек двумя запросами.
func (r *Repo) attachTopics(cards []Card) error {
	if len(cards) == 0 {
		return nil
	}
	byID := make(map[int64]int, len(cards))
	ids := make([]int64, 0, len(cards))
	for i, c := range cards {
		byID[c.ID] = i
		ids = append(ids, c.ID)
	}

	ph, args, err := placeholders(ids)
	if err != nil {
		return err
	}

	// Грамматические темы.
	rows, err := r.db.Query(`
		SELECT cgt.card_id, gt.id, gt.title
		FROM card_grammar_topic cgt
		JOIN grammar_topic gt ON gt.id = cgt.grammar_topic_id
		WHERE cgt.card_id IN (`+ph+`)
		ORDER BY gt.title`, args...)
	if err != nil {
		return fmt.Errorf("card grammar topics: %w", err)
	}
	for rows.Next() {
		var cardID int64
		var t Topic
		if err := rows.Scan(&cardID, &t.ID, &t.Title); err != nil {
			rows.Close()
			return fmt.Errorf("scan card grammar topic: %w", err)
		}
		cards[byID[cardID]].GrammarTopics = append(cards[byID[cardID]].GrammarTopics, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows card grammar topics: %w", err)
	}

	// Лексические темы.
	rows, err = r.db.Query(`
		SELECT clt.card_id, lt.id, lt.title
		FROM card_lexica_topic clt
		JOIN lexica_topic lt ON lt.id = clt.lexica_topic_id
		WHERE clt.card_id IN (`+ph+`)
		ORDER BY lt.title`, args...)
	if err != nil {
		return fmt.Errorf("card lexica topics: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cardID int64
		var t Topic
		if err := rows.Scan(&cardID, &t.ID, &t.Title); err != nil {
			return fmt.Errorf("scan card lexica topic: %w", err)
		}
		cards[byID[cardID]].LexicaTopics = append(cards[byID[cardID]].LexicaTopics, t)
	}
	return rows.Err()
}
