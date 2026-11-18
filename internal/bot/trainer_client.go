// Тонкий gRPC-клиент к API с таймаутами на каждом вызове.
package bot

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	trainerv1 "github.com/evgshumakova/mini-owl/gen/trainer/v1"
)

// TrainerClient — обёртка над gRPC-стабом: добавляет таймауты (все вызовы API ограничены).
type TrainerClient struct {
	api     trainerv1.TrainerServiceClient
	timeout time.Duration
}

// NewTrainerClient подключается к API (лениво — реальное соединение при первом вызове).
func NewTrainerClient(addr string, timeout time.Duration) (*TrainerClient, error) {
	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("grpc client %s: %w", addr, err)
	}
	return &TrainerClient{api: trainerv1.NewTrainerServiceClient(cc), timeout: timeout}, nil
}

func (t *TrainerClient) ctx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, t.timeout)
}

// ListLanguages — целевые языки.
func (t *TrainerClient) ListLanguages(ctx context.Context) ([]*trainerv1.Language, error) {
	c, cancel := t.ctx(ctx)
	defer cancel()
	resp, err := t.api.ListLanguages(c, &trainerv1.ListLanguagesRequest{})
	if err != nil {
		return nil, fmt.Errorf("list languages: %w", err)
	}
	return resp.GetLanguages(), nil
}

// ListLevels — уровни CEFR.
func (t *TrainerClient) ListLevels(ctx context.Context) ([]*trainerv1.Level, error) {
	c, cancel := t.ctx(ctx)
	defer cancel()
	resp, err := t.api.ListLevels(c, &trainerv1.ListLevelsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list levels: %w", err)
	}
	return resp.GetLevels(), nil
}

// ListTopics — темы языка: grammar=true → грамматика (с фильтром по уровню), иначе лексика.
func (t *TrainerClient) ListTopics(ctx context.Context, langCode string, grammar bool, levelCode string) ([]*trainerv1.Topic, error) {
	tt := trainerv1.TopicType_TOPIC_TYPE_LEXICA
	if grammar {
		tt = trainerv1.TopicType_TOPIC_TYPE_GRAMMAR
	}
	c, cancel := t.ctx(ctx)
	defer cancel()
	resp, err := t.api.ListTopics(c, &trainerv1.ListTopicsRequest{
		LanguageCode: langCode, TopicType: tt, LevelCode: levelCode,
	})
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	return resp.GetTopics(), nil
}

// DrawCards — count случайных карточек по фильтрам.
func (t *TrainerClient) DrawCards(ctx context.Context, langCode, levelCode string, grammarIDs, lexicaIDs []int64, count int) ([]*trainerv1.Card, error) {
	c, cancel := t.ctx(ctx)
	defer cancel()
	resp, err := t.api.DrawCards(c, &trainerv1.DrawCardsRequest{
		LanguageCode:    langCode,
		LevelCode:       levelCode,
		GrammarTopicIds: grammarIDs,
		LexicaTopicIds:  lexicaIDs,
		Count:           int32(count),
	})
	if err != nil {
		return nil, fmt.Errorf("draw cards: %w", err)
	}
	return resp.GetCards(), nil
}
