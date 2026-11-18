// E2E-сценарии (секция 7 плана): справочники, happy path, фильтры, валидация, CRUD.
package server_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	trainerv1 "github.com/evgshumakova/mini-owl/gen/trainer/v1"
	"github.com/evgshumakova/mini-owl/internal/catalog"
)

// Сценарий 1: справочники.
func TestE2E_ReferenceData(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()

	langs, err := env.Client.ListLanguages(ctx, &trainerv1.ListLanguagesRequest{})
	if err != nil {
		t.Fatalf("ListLanguages: %v", err)
	}
	if len(langs.GetLanguages()) != 2 || langs.GetLanguages()[0].GetCode() != "en" || langs.GetLanguages()[1].GetCode() != "it" {
		t.Fatalf("ожидались [en it], получено %+v", langs.GetLanguages())
	}

	levels, err := env.Client.ListLevels(ctx, &trainerv1.ListLevelsRequest{})
	if err != nil {
		t.Fatalf("ListLevels: %v", err)
	}
	if len(levels.GetLevels()) != 5 || levels.GetLevels()[0].GetCode() != "A1" || levels.GetLevels()[4].GetCode() != "C1" {
		t.Fatalf("ожидались A1..C1, получено %+v", levels.GetLevels())
	}

	a1 := env.mustTopic(t, "it", "Presente", catalog.TopicGrammar, "A1")
	b1 := env.mustTopic(t, "it", "Congiuntivo", catalog.TopicGrammar, "B1")
	anyLevel := env.mustTopic(t, "it", "Артикли", catalog.TopicGrammar, "")
	env.mustTopic(t, "it", "Еда", catalog.TopicLexica, "")

	// GRAMMAR + level=B1: темы B1 + темы без уровня, без A1.
	got, err := env.Client.ListTopics(ctx, &trainerv1.ListTopicsRequest{
		LanguageCode: "it", TopicType: trainerv1.TopicType_TOPIC_TYPE_GRAMMAR, LevelCode: "B1",
	})
	if err != nil {
		t.Fatalf("ListTopics(GRAMMAR,B1): %v", err)
	}
	ids := topicIDs(got.GetTopics())
	if !contains(ids, b1) || !contains(ids, anyLevel) || contains(ids, a1) {
		t.Fatalf("ожидались B1 + без уровня (без A1), получено %v", ids)
	}

	// LEXICA без фильтра уровня.
	lex, err := env.Client.ListTopics(ctx, &trainerv1.ListTopicsRequest{
		LanguageCode: "it", TopicType: trainerv1.TopicType_TOPIC_TYPE_LEXICA,
	})
	if err != nil {
		t.Fatalf("ListTopics(LEXICA): %v", err)
	}
	if len(lex.GetTopics()) != 1 || lex.GetTopics()[0].GetTitle() != "Еда" {
		t.Fatalf("ожидалась [Еда], получено %+v", lex.GetTopics())
	}
}

// Сценарий 2: DrawCards happy path — count уважен, без повторов.
func TestE2E_DrawCardsHappyPath(t *testing.T) {
	env := newEnv(t)
	g := env.mustTopic(t, "it", "Passato Prossimo", catalog.TopicGrammar, "B1")
	l := env.mustTopic(t, "it", "Еда", catalog.TopicLexica, "")
	for i := 0; i < 20; i++ {
		env.mustUpsertCard(t, &trainerv1.UpsertCardRequest{
			LanguageCode: "it", LevelCode: "B1",
			SourceText: "фраза", TargetText: "frase",
			GrammarTopicIds: []int64{g}, LexicaTopicIds: []int64{l},
		})
	}

	resp := mustDraw(t, env.Client, &trainerv1.DrawCardsRequest{
		LanguageCode: "it", LevelCode: "B1", GrammarTopicIds: []int64{g}, LexicaTopicIds: []int64{l}, Count: 10,
	})
	if resp.GetAvailable() != 20 {
		t.Fatalf("available = %d, ожидалось 20", resp.GetAvailable())
	}
	if len(resp.GetCards()) != 10 {
		t.Fatalf("получено %d карточек, ожидалось 10", len(resp.GetCards()))
	}
	seen := map[int64]bool{}
	for _, c := range resp.GetCards() {
		if seen[c.GetId()] {
			t.Fatalf("дубль карточки id=%d", c.GetId())
		}
		seen[c.GetId()] = true
		if c.GetLanguageCode() != "it" || c.GetLevelCode() != "B1" {
			t.Fatalf("фильтры не соблюдены: %+v", c)
		}
	}
}

// Сценарий 3: фильтры DrawCards — карточки вне тем не возвращаются.
func TestE2E_DrawCardsFilters(t *testing.T) {
	env := newEnv(t)
	gFood := env.mustTopic(t, "en", "Present Simple", catalog.TopicGrammar, "A1")
	lFood := env.mustTopic(t, "en", "Food", catalog.TopicLexica, "")
	env.mustUpsertCard(t, &trainerv1.UpsertCardRequest{
		LanguageCode: "en", LevelCode: "A1", SourceText: "мне нравится", TargetText: "I like",
		GrammarTopicIds: []int64{gFood}, LexicaTopicIds: []int64{lFood},
	})
	env.mustUpsertCard(t, &trainerv1.UpsertCardRequest{
		LanguageCode: "en", LevelCode: "A1", SourceText: "другое", TargetText: "other",
	})

	// По грамматической теме.
	resp := mustDraw(t, env.Client, &trainerv1.DrawCardsRequest{
		LanguageCode: "en", GrammarTopicIds: []int64{gFood}, Count: 10,
	})
	if resp.GetAvailable() != 1 || len(resp.GetCards()) != 1 || resp.GetCards()[0].GetSourceText() != "мне нравится" {
		t.Fatalf("фильтр по грамматике не сработал: %+v", resp)
	}

	// По обеим темам сразу.
	resp = mustDraw(t, env.Client, &trainerv1.DrawCardsRequest{
		LanguageCode: "en", GrammarTopicIds: []int64{gFood}, LexicaTopicIds: []int64{lFood}, Count: 10,
	})
	if len(resp.GetCards()) != 1 {
		t.Fatalf("фильтр по двум темам не сработал: %+v", resp)
	}

	// Тема с несуществующим id — пусто, без ошибки.
	resp = mustDraw(t, env.Client, &trainerv1.DrawCardsRequest{
		LanguageCode: "en", GrammarTopicIds: []int64{999}, Count: 10,
	})
	if len(resp.GetCards()) != 0 || resp.GetAvailable() != 0 {
		t.Fatalf("несуществующая тема должна давать пусто: %+v", resp)
	}
}

// Сценарий 4: пустая выдача — валидный ответ, не ошибка.
func TestE2E_DrawCardsEmpty(t *testing.T) {
	env := newEnv(t)
	resp := mustDraw(t, env.Client, &trainerv1.DrawCardsRequest{LanguageCode: "it", Count: 5})
	if len(resp.GetCards()) != 0 || resp.GetAvailable() != 0 {
		t.Fatalf("ожидался пустой ответ, получено %+v", resp)
	}
}

// Сценарий 5: карточек меньше count — возвращаются все + available.
func TestE2E_DrawCardsUndercount(t *testing.T) {
	env := newEnv(t)
	env.mustUpsertCard(t, &trainerv1.UpsertCardRequest{
		LanguageCode: "en", LevelCode: "A1", SourceText: "привет", TargetText: "hello",
	})
	resp := mustDraw(t, env.Client, &trainerv1.DrawCardsRequest{LanguageCode: "en", Count: 10})
	if len(resp.GetCards()) != 1 || resp.GetAvailable() != 1 {
		t.Fatalf("ожидалась 1/1, получено %d/%d", len(resp.GetCards()), resp.GetAvailable())
	}
}

// Сценарий 6: валидация — InvalidArgument на плохие фильтры.
// Поведение count > 50 зафиксировано: InvalidArgument.
func TestE2E_DrawCardsValidation(t *testing.T) {
	env := newEnv(t)
	cases := []struct {
		name string
		req  *trainerv1.DrawCardsRequest
	}{
		{"пустой язык", &trainerv1.DrawCardsRequest{LanguageCode: "", Count: 5}},
		{"count=0", &trainerv1.DrawCardsRequest{LanguageCode: "it", Count: 0}},
		{"count=-1", &trainerv1.DrawCardsRequest{LanguageCode: "it", Count: -1}},
		{"count>50", &trainerv1.DrawCardsRequest{LanguageCode: "it", Count: catalog.MaxDrawCount + 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.Client.DrawCards(context.Background(), tc.req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("ожидался InvalidArgument, получено %v", err)
			}
		})
	}
}

// Сценарий 7: CRUD-жизненный цикл через gRPC.
func TestE2E_CardLifecycle(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	l := env.mustTopic(t, "it", "Еда", catalog.TopicLexica, "")

	card := env.mustUpsertCard(t, &trainerv1.UpsertCardRequest{
		LanguageCode: "it", LevelCode: "A1", SourceText: "вода", TargetText: "acqua",
		LexicaTopicIds: []int64{l},
	})
	if card.GetId() == 0 || len(card.GetLexicaTopics()) != 1 {
		t.Fatalf("создание не вернуло карточку с темами: %+v", card)
	}

	// Обновление той же карточки.
	updated := env.mustUpsertCard(t, &trainerv1.UpsertCardRequest{
		Id: card.GetId(), LanguageCode: "it", LevelCode: "A1",
		SourceText: "вода", TargetText: "l'acqua", LexicaTopicIds: []int64{l},
	})
	if updated.GetId() != card.GetId() || updated.GetTargetText() != "l'acqua" {
		t.Fatalf("ожидалось обновление, получено %+v", updated)
	}

	// Карточка доступна в выдаче.
	resp := mustDraw(t, env.Client, &trainerv1.DrawCardsRequest{LanguageCode: "it", Count: 10})
	if len(resp.GetCards()) != 1 {
		t.Fatalf("карточка не в выдаче: %+v", resp)
	}

	// Удаление.
	if _, err := env.Client.DeleteCard(ctx, &trainerv1.DeleteCardRequest{Id: card.GetId()}); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	if _, err := env.Client.DeleteCard(ctx, &trainerv1.DeleteCardRequest{Id: card.GetId()}); status.Code(err) != codes.NotFound {
		t.Fatalf("повторное удаление: ожидался NotFound, получено %v", err)
	}
	if _, err := env.Client.UpsertCard(ctx, &trainerv1.UpsertCardRequest{
		Id: card.GetId(), LanguageCode: "it", LevelCode: "A1", SourceText: "x", TargetText: "y",
	}); status.Code(err) != codes.NotFound {
		t.Fatalf("upsert несуществующей: ожидался NotFound, получено %v", err)
	}

	// Чужая тема другого языка — InvalidArgument.
	enTopic := env.mustTopic(t, "en", "Food", catalog.TopicLexica, "")
	if _, err := env.Client.UpsertCard(ctx, &trainerv1.UpsertCardRequest{
		LanguageCode: "it", LevelCode: "A1", SourceText: "x", TargetText: "y",
		LexicaTopicIds: []int64{enTopic},
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("чужая тема: ожидался InvalidArgument, получено %v", err)
	}
}

// Сценарий 8: изоляция языков it/en.
func TestE2E_LanguageIsolation(t *testing.T) {
	env := newEnv(t)
	env.mustUpsertCard(t, &trainerv1.UpsertCardRequest{
		LanguageCode: "it", LevelCode: "A1", SourceText: "ciao-ru", TargetText: "ciao",
	})
	env.mustUpsertCard(t, &trainerv1.UpsertCardRequest{
		LanguageCode: "en", LevelCode: "A1", SourceText: "hi-ru", TargetText: "hi",
	})
	for _, lang := range []string{"it", "en"} {
		resp := mustDraw(t, env.Client, &trainerv1.DrawCardsRequest{LanguageCode: lang, Count: 10})
		if resp.GetAvailable() != 1 {
			t.Fatalf("%s: available = %d, ожидалось 1", lang, resp.GetAvailable())
		}
		for _, c := range resp.GetCards() {
			if c.GetLanguageCode() != lang {
				t.Fatalf("утечка языков: запрошен %s, получена %s", lang, c.GetLanguageCode())
			}
		}
	}
}

func topicIDs(ts []*trainerv1.Topic) []int64 {
	out := make([]int64, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.GetId())
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
