// Публичный доступ к темам (для gRPC-слоя).
package catalog

import "fmt"

// TopicType — тип темы.
type TopicType int

const (
	TopicGrammar TopicType = iota
	TopicLexica
)

// ListTopics возвращает темы языка по типу. Для грамматических тем при
// непустом levelCode — темы уровня плюс темы без привязки к уровню.
func (r *Repo) ListTopics(langCode string, tt TopicType, levelCode string) ([]Topic, error) {
	switch tt {
	case TopicGrammar:
		gts, err := r.grammarTopics(langCode, levelCode)
		if err != nil {
			return nil, err
		}
		topics := make([]Topic, 0, len(gts))
		for _, gt := range gts {
			topics = append(topics, gt.Topic)
		}
		return topics, nil
	case TopicLexica:
		lts, err := r.lexicaTopics(langCode)
		if err != nil {
			return nil, err
		}
		topics := make([]Topic, 0, len(lts))
		for _, lt := range lts {
			topics = append(topics, lt.Topic)
		}
		return topics, nil
	default:
		return nil, fmt.Errorf("неизвестный тип темы: %d", tt)
	}
}
