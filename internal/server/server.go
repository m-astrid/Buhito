// Package server — gRPC-слой поверх каталога: маппинг домена в proto и ошибок в коды gRPC.
package server

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	trainerv1 "github.com/evgshumakova/mini-owl/gen/trainer/v1"
	"github.com/evgshumakova/mini-owl/internal/catalog"
)

// Server реализует trainer.v1.TrainerService.
type Server struct {
	trainerv1.UnimplementedTrainerServiceServer
	repo *catalog.Repo
}

func New(repo *catalog.Repo) *Server {
	return &Server{repo: repo}
}

func (s *Server) ListLanguages(ctx context.Context, _ *trainerv1.ListLanguagesRequest) (*trainerv1.ListLanguagesResponse, error) {
	langs, err := s.repo.ListLanguages()
	if err != nil {
		return nil, internal(err, "list languages")
	}
	resp := &trainerv1.ListLanguagesResponse{Languages: make([]*trainerv1.Language, 0, len(langs))}
	for _, l := range langs {
		resp.Languages = append(resp.Languages, &trainerv1.Language{Code: l.Code, Name: l.Name})
	}
	return resp, nil
}

func (s *Server) ListLevels(ctx context.Context, _ *trainerv1.ListLevelsRequest) (*trainerv1.ListLevelsResponse, error) {
	levels, err := s.repo.ListLevels()
	if err != nil {
		return nil, internal(err, "list levels")
	}
	resp := &trainerv1.ListLevelsResponse{Levels: make([]*trainerv1.Level, 0, len(levels))}
	for _, l := range levels {
		resp.Levels = append(resp.Levels, &trainerv1.Level{Code: l.Code, SortOrder: int32(l.SortOrder)})
	}
	return resp, nil
}

func (s *Server) ListTopics(ctx context.Context, req *trainerv1.ListTopicsRequest) (*trainerv1.ListTopicsResponse, error) {
	tt, err := topicTypeFromProto(req.GetTopicType())
	if err != nil {
		return nil, err
	}
	topics, err := s.repo.ListTopics(req.GetLanguageCode(), tt, req.GetLevelCode())
	if err != nil {
		return nil, internal(err, "list topics")
	}
	return &trainerv1.ListTopicsResponse{Topics: topicsToProto(topics)}, nil
}

func (s *Server) DrawCards(ctx context.Context, req *trainerv1.DrawCardsRequest) (*trainerv1.DrawCardsResponse, error) {
	cards, available, err := s.repo.DrawCards(catalog.DrawFilter{
		LanguageCode:    req.GetLanguageCode(),
		LevelCode:       req.GetLevelCode(),
		GrammarTopicIDs: req.GetGrammarTopicIds(),
		LexicaTopicIDs:  req.GetLexicaTopicIds(),
		Count:           int(req.GetCount()),
	})
	if err != nil {
		return nil, internal(err, "draw cards")
	}
	resp := &trainerv1.DrawCardsResponse{
		Cards:     make([]*trainerv1.Card, 0, len(cards)),
		Available: int32(available),
	}
	for i := range cards {
		resp.Cards = append(resp.Cards, cardToProto(&cards[i]))
	}
	return resp, nil
}

func (s *Server) UpsertCard(ctx context.Context, req *trainerv1.UpsertCardRequest) (*trainerv1.UpsertCardResponse, error) {
	card, err := s.repo.UpsertCard(req.GetId(), catalog.UpsertCardCard{
		LanguageCode:    req.GetLanguageCode(),
		LevelCode:       req.GetLevelCode(),
		SourceText:      req.GetSourceText(),
		TargetText:      req.GetTargetText(),
		Note:            req.GetNote(),
		GrammarTopicIDs: req.GetGrammarTopicIds(),
		LexicaTopicIDs:  req.GetLexicaTopicIds(),
	})
	if err != nil {
		return nil, internal(err, "upsert card")
	}
	return &trainerv1.UpsertCardResponse{Card: cardToProto(&card)}, nil
}

func (s *Server) DeleteCard(ctx context.Context, req *trainerv1.DeleteCardRequest) (*trainerv1.DeleteCardResponse, error) {
	if err := s.repo.DeleteCard(req.GetId()); err != nil {
		return nil, internal(err, "delete card")
	}
	return &trainerv1.DeleteCardResponse{}, nil
}

// --- маппинг домена в proto ---

func topicTypeFromProto(t trainerv1.TopicType) (catalog.TopicType, error) {
	switch t {
	case trainerv1.TopicType_TOPIC_TYPE_GRAMMAR:
		return catalog.TopicGrammar, nil
	case trainerv1.TopicType_TOPIC_TYPE_LEXICA:
		return catalog.TopicLexica, nil
	default:
		return 0, status.Errorf(codes.InvalidArgument, "topic_type не указан")
	}
}

func topicsToProto(ts []catalog.Topic) []*trainerv1.Topic {
	out := make([]*trainerv1.Topic, 0, len(ts))
	for _, t := range ts {
		out = append(out, &trainerv1.Topic{Id: t.ID, Title: t.Title})
	}
	return out
}

func cardToProto(c *catalog.Card) *trainerv1.Card {
	return &trainerv1.Card{
		Id:            c.ID,
		LanguageCode:  c.LanguageCode,
		LevelCode:     c.LevelCode,
		SourceText:    c.SourceText,
		TargetText:    c.TargetText,
		Note:          c.Note,
		GrammarTopics: topicsToProto(c.GrammarTopics),
		LexicaTopics:  topicsToProto(c.LexicaTopics),
	}
}

// --- маппинг ошибок ---

func internal(err error, op string) error {
	switch {
	case errors.Is(err, catalog.ErrInvalid):
		return status.Errorf(codes.InvalidArgument, "%s: %s", op, err)
	case errors.Is(err, catalog.ErrNotFound):
		return status.Errorf(codes.NotFound, "%s: %s", op, err)
	default:
		return status.Errorf(codes.Internal, "%s: %v", op, err)
	}
}
