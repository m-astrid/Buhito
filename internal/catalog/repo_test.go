package catalog_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/evgshumakova/mini-owl/internal/catalog"
	"github.com/evgshumakova/mini-owl/internal/storage"
)

// newTestRepo открывает in-memory SQLite с миграциями.
func newTestRepo(t *testing.T) *catalog.Repo {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return catalog.NewRepo(db)
}

// seedCard вставляет карточку и возвращает её id.
func seedCard(t *testing.T, r *catalog.Repo, lang, level, src, tgt string, gt, lt []int64) int64 {
	t.Helper()
	card, err := r.UpsertCard(0, catalog.UpsertCardCard{
		LanguageCode: lang, LevelCode: level, SourceText: src, TargetText: tgt,
		GrammarTopicIDs: gt, LexicaTopicIDs: lt,
	})
	if err != nil {
		t.Fatalf("seed card %q: %v", src, err)
	}
	return card.ID
}

// seedTopic создаёт тему и возвращает её id.
func seedTopics(t *testing.T, r *catalog.Repo, lang, title string, tt catalog.TopicType, level string) int64 {
	t.Helper()
	id, err := r.UpsertTopic(lang, title, tt, level)
	if err != nil {
		t.Fatalf("seed topic %q: %v", title, err)
	}
	return id
}

func TestListLanguages(t *testing.T) {
	r := newTestRepo(t)
	langs, err := r.ListLanguages()
	if err != nil {
		t.Fatalf("ListLanguages: %v", err)
	}
	if len(langs) != 2 || langs[0].Code != "en" || langs[1].Code != "it" {
		t.Fatalf("ожидались [en it], получено %+v", langs)
	}
}

func TestListLevels(t *testing.T) {
	r := newTestRepo(t)
	levels, err := r.ListLevels()
	if err != nil {
		t.Fatalf("ListLevels: %v", err)
	}
	if len(levels) != 5 || levels[0].Code != "A1" || levels[4].Code != "C1" {
		t.Fatalf("ожидались A1..C1 по sort_order, получено %+v", levels)
	}
}

func TestGrammarTopicsLevelFilter(t *testing.T) {
	r := newTestRepo(t)
	a1 := seedTopics(t, r, "it", "Presente", catalog.TopicGrammar, "A1")
	b1 := seedTopics(t, r, "it", "Congiuntivo", catalog.TopicGrammar, "B1")
	any := seedTopics(t, r, "it", "Артикли", catalog.TopicGrammar, "")

	// Фильтр B1: темы B1 + темы без уровня; тема A1 не входит.
	got, err := r.ListTopics("it", catalog.TopicGrammar, "B1")
	if err != nil {
		t.Fatalf("ListTopics: %v", err)
	}
	ids := topicIDs(got)
	if !contains(ids, b1) || !contains(ids, any) || contains(ids, a1) {
		t.Fatalf("ожидались B1 + без уровня (без A1), получено %v", ids)
	}

	// Без фильтра — все темы.
	all, err := r.ListTopics("it", catalog.TopicGrammar, "")
	if err != nil {
		t.Fatalf("ListTopics: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ожидались все 3 темы, получено %d", len(all))
	}
}

func TestLexicaTopicsLanguageScope(t *testing.T) {
	r := newTestRepo(t)
	seedTopics(t, r, "it", "Еда", catalog.TopicLexica, "")
	seedTopics(t, r, "en", "Food", catalog.TopicLexica, "")

	got, err := r.ListTopics("en", catalog.TopicLexica, "")
	if err != nil {
		t.Fatalf("ListTopics: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Food" {
		t.Fatalf("ожидалась только тема en, получено %+v", got)
	}
}

func TestDrawCardsHappyPath(t *testing.T) {
	r := newTestRepo(t)
	g := seedTopics(t, r, "it", "Passato Prossimo", catalog.TopicGrammar, "B1")
	l := seedTopics(t, r, "it", "Еда", catalog.TopicLexica, "")
	for i := 0; i < 20; i++ {
		seedCard(t, r, "it", "B1", "ru-фраза", "итал-фраза", []int64{g}, []int64{l})
	}

	cards, available, err := r.DrawCards(catalog.DrawFilter{
		LanguageCode: "it", LevelCode: "B1", GrammarTopicIDs: []int64{g}, LexicaTopicIDs: []int64{l}, Count: 10,
	})
	if err != nil {
		t.Fatalf("DrawCards: %v", err)
	}
	if available != 20 {
		t.Fatalf("available = %d, ожидалось 20", available)
	}
	if len(cards) != 10 {
		t.Fatalf("получено %d карточек, ожидалось 10", len(cards))
	}
	seen := map[int64]bool{}
	for _, c := range cards {
		if seen[c.ID] {
			t.Fatalf("дубль карточки id=%d", c.ID)
		}
		seen[c.ID] = true
		if c.GrammarTopics[0].Title != "Passato Prossimo" || c.LexicaTopics[0].Title != "Еда" {
			t.Fatalf("темы не прикрепились: %+v", c)
		}
	}
}

func TestDrawCardsUndercount(t *testing.T) {
	r := newTestRepo(t)
	seedCard(t, r, "en", "A1", "привет", "hello", nil, nil)

	cards, available, err := r.DrawCards(catalog.DrawFilter{LanguageCode: "en", Count: 10})
	if err != nil {
		t.Fatalf("DrawCards: %v", err)
	}
	if len(cards) != 1 || available != 1 {
		t.Fatalf("ожидалась 1/1, получено %d/%d", len(cards), available)
	}
}

func TestDrawCardsEmpty(t *testing.T) {
	r := newTestRepo(t)
	cards, available, err := r.DrawCards(catalog.DrawFilter{LanguageCode: "it", Count: 5})
	if err != nil {
		t.Fatalf("DrawCards: %v", err)
	}
	if len(cards) != 0 || available != 0 {
		t.Fatalf("ожидался пустой результат, получено %d карточек, available=%d", len(cards), available)
	}
}

func TestDrawCardsFilterExcludesOthers(t *testing.T) {
	r := newTestRepo(t)
	gFood := seedTopics(t, r, "en", "Present Simple", catalog.TopicGrammar, "A1")
	lFood := seedTopics(t, r, "en", "Food", catalog.TopicLexica, "")
	seedCard(t, r, "en", "A1", "мне нравится", "I like", []int64{gFood}, []int64{lFood})
	seedCard(t, r, "en", "A1", "другое", "other", nil, nil)

	cards, available, err := r.DrawCards(catalog.DrawFilter{
		LanguageCode: "en", GrammarTopicIDs: []int64{gFood}, Count: 10,
	})
	if err != nil {
		t.Fatalf("DrawCards: %v", err)
	}
	if available != 1 || len(cards) != 1 || cards[0].SourceText != "мне нравится" {
		t.Fatalf("фильтр по грамматической теме не сработал: available=%d, cards=%+v", available, cards)
	}
}

func TestDrawCardsLanguageIsolation(t *testing.T) {
	r := newTestRepo(t)
	seedCard(t, r, "it", "A1", "ciao-ru", "ciao", nil, nil)
	seedCard(t, r, "en", "A1", "hi-ru", "hi", nil, nil)

	for _, lang := range []string{"it", "en"} {
		cards, _, err := r.DrawCards(catalog.DrawFilter{LanguageCode: lang, Count: 10})
		if err != nil {
			t.Fatalf("DrawCards(%s): %v", lang, err)
		}
		for _, c := range cards {
			if c.LanguageCode != lang {
				t.Fatalf("утечка между языками: запрошен %s, получена %s", lang, c.LanguageCode)
			}
		}
	}
}

func TestDrawCardsValidation(t *testing.T) {
	r := newTestRepo(t)
	cases := []catalog.DrawFilter{
		{LanguageCode: "", Count: 5},
		{LanguageCode: "it", Count: 0},
		{LanguageCode: "it", Count: -1},
		{LanguageCode: "it", Count: catalog.MaxDrawCount + 1},
	}
	for i, f := range cases {
		if _, _, err := r.DrawCards(f); !errors.Is(err, catalog.ErrInvalid) {
			t.Fatalf("кейс %d: ожидалась ErrInvalid, получено %v", i, err)
		}
	}
}

func TestUpsertCardLifecycle(t *testing.T) {
	r := newTestRepo(t)
	l := seedTopics(t, r, "it", "Еда", catalog.TopicLexica, "")

	card, err := r.UpsertCard(0, catalog.UpsertCardCard{
		LanguageCode: "it", LevelCode: "A1", SourceText: "вода", TargetText: "acqua",
		LexicaTopicIDs: []int64{l},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Обновление той же карточки.
	updated, err := r.UpsertCard(card.ID, catalog.UpsertCardCard{
		LanguageCode: "it", LevelCode: "A1", SourceText: "вода", TargetText: "l'acqua",
		LexicaTopicIDs: []int64{l},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.ID != card.ID || updated.TargetText != "l'acqua" {
		t.Fatalf("ожидалось обновление той же карточки, получено %+v (была %+v)", updated, card)
	}
	if len(updated.LexicaTopics) != 1 {
		t.Fatalf("дубль в M2M после upsert: %+v", updated.LexicaTopics)
	}

	// Карточка доступна в выдаче.
	cards, _, err := r.DrawCards(catalog.DrawFilter{LanguageCode: "it", Count: 10})
	if err != nil || len(cards) != 1 {
		t.Fatalf("после upsert карточка не в выдаче: %v, %d", err, len(cards))
	}

	// Удаление.
	if err := r.DeleteCard(card.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.GetCard(card.ID); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("ожидалась ErrNotFound, получено %v", err)
	}
	if err := r.DeleteCard(card.ID); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("повторное удаление: ожидалась ErrNotFound, получено %v", err)
	}
}

func TestUpsertCardRejectsForeignTopic(t *testing.T) {
	r := newTestRepo(t)
	enTopic := seedTopics(t, r, "en", "Food", catalog.TopicLexica, "")

	// Тема en для карточки it — должна быть отвергнута.
	if _, err := r.UpsertCard(0, catalog.UpsertCardCard{
		LanguageCode: "it", LevelCode: "A1", SourceText: "а", TargetText: "b",
		LexicaTopicIDs: []int64{enTopic},
	}); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("ожидалась ErrInvalid для чужой темы, получено %v", err)
	}
}

func topicIDs(ts []catalog.Topic) []int64 {
	out := make([]int64, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

func contains(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
