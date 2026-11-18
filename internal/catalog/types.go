// Package catalog — домен каталога карточек: справочники, карточки, отбор.
package catalog

import "fmt"

// Language — целевой язык (родной всегда русский).
type Language struct {
	Code string // "it", "en"
	Name string
}

// Level — уровень CEFR.
type Level struct {
	Code      string // "A1".."C1"
	SortOrder int
}

// Topic — общие поля темы (грамматической или лексической).
type Topic struct {
	ID    int64
	Title string
}

// GrammarTopic — грамматическая тема, опционально привязана к уровню
// (LevelCode == "" — тема для любого уровня).
type GrammarTopic struct {
	Topic
	LevelCode string
}

// LexicaTopic — лексическая тема (к уровню не привязана).
type LexicaTopic struct {
	Topic
}

// Card — карточка: русская фраза + правильный перевод + темы.
type Card struct {
	ID            int64
	LanguageCode  string
	LevelCode     string
	SourceText    string
	TargetText    string
	Note          string
	GrammarTopics []Topic
	LexicaTopics  []Topic
}

// MaxDrawCount — верхняя граница количества карточек за запрос.
const MaxDrawCount = 50

// DrawFilter — фильтры отбора карточек. Пустые списки = «любые».
type DrawFilter struct {
	LanguageCode    string
	LevelCode       string
	GrammarTopicIDs []int64
	LexicaTopicIDs  []int64
	Count           int
}

// Validate проверяет фильтр: непустой язык, count в диапазоне 1..MaxDrawCount.
func (f DrawFilter) Validate() error {
	if f.LanguageCode == "" {
		return fmt.Errorf("language_code обязателен")
	}
	if f.Count < 1 || f.Count > MaxDrawCount {
		return fmt.Errorf("count должен быть в диапазоне 1..%d, получено %d", MaxDrawCount, f.Count)
	}
	return nil
}
