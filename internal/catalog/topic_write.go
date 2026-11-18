// Создание/обновление тем (наполнение каталога).
package catalog

import "fmt"

// UpsertTopic создаёт тему или возвращает id существующей (уникальность: язык + title).
// levelCode учитывается только для грамматических тем; "" — тема без привязки к уровню.
func (r *Repo) UpsertTopic(langCode, title string, tt TopicType, levelCode string) (int64, error) {
	if langCode == "" || title == "" {
		return 0, ErrInvalid
	}

	var q string
	switch tt {
	case TopicGrammar:
		q = `INSERT INTO grammar_topic (language_id, level_id, title)
		     VALUES ((SELECT id FROM language WHERE code = ?),
		             (SELECT id FROM level WHERE code = ?), ?)
		     ON CONFLICT (language_id, title) DO UPDATE SET level_id = excluded.level_id
		     RETURNING id`
	case TopicLexica:
		if levelCode != "" {
			return 0, fmt.Errorf("лексические темы не привязываются к уровню: %w", ErrInvalid)
		}
		q = `INSERT INTO lexica_topic (language_id, title)
		     VALUES ((SELECT id FROM language WHERE code = ?), ?)
		     ON CONFLICT (language_id, title) DO UPDATE SET title = excluded.title
		     RETURNING id`
	default:
		return 0, fmt.Errorf("неизвестный тип темы: %d", tt)
	}

	args := []any{langCode, title}
	if tt == TopicGrammar {
		args = []any{langCode, levelCode, title}
	}

	var id int64
	if err := r.db.QueryRow(q, args...).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert topic: %w", err)
	}
	return id, nil
}
