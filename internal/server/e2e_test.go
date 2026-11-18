// E2E-тесты API: реальный gRPC-сервер + клиент через bufconn, SQLite in-memory.
// Сценарии — из секции 7 плана. Фикстуры тем создаются напрямую через Repo
// (RPC создания тем в контракте нет — наполнение отдельный скоуп).
package server_test

import (
	"context"
	"database/sql"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	trainerv1 "github.com/evgshumakova/mini-owl/gen/trainer/v1"
	"github.com/evgshumakova/mini-owl/internal/catalog"
	"github.com/evgshumakova/mini-owl/internal/server"
	"github.com/evgshumakova/mini-owl/internal/storage"
)

// testEnv — полный стек для e2e: gRPC-клиент + прямой Repo для фикстур.
type testEnv struct {
	Client trainerv1.TrainerServiceClient
	Repo   *catalog.Repo
}

// newEnv поднимает gRPC-сервер на bufconn поверх SQLite in-memory с миграциями.
func newEnv(t *testing.T) testEnv {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1) // все соединения — одна и та же память
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := catalog.NewRepo(db)
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	trainerv1.RegisterTrainerServiceServer(srv, server.New(repo))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return testEnv{Client: trainerv1.NewTrainerServiceClient(conn), Repo: repo}
}

// mustTopic создаёт тему через Repo (напрямую) и возвращает id.
func (e testEnv) mustTopic(t *testing.T, lang, title string, tt catalog.TopicType, level string) int64 {
	t.Helper()
	id, err := e.Repo.UpsertTopic(lang, title, tt, level)
	if err != nil {
		t.Fatalf("upsert topic %q: %v", title, err)
	}
	return id
}

// mustUpsertCard создаёт карточку через gRPC (проверяя контракт).
func (e testEnv) mustUpsertCard(t *testing.T, req *trainerv1.UpsertCardRequest) *trainerv1.Card {
	t.Helper()
	resp, err := e.Client.UpsertCard(context.Background(), req)
	if err != nil {
		t.Fatalf("UpsertCard %q: %v", req.GetSourceText(), err)
	}
	return resp.GetCard()
}

func mustDraw(t *testing.T, c trainerv1.TrainerServiceClient, req *trainerv1.DrawCardsRequest) *trainerv1.DrawCardsResponse {
	t.Helper()
	resp, err := c.DrawCards(context.Background(), req)
	if err != nil {
		t.Fatalf("DrawCards: %v", err)
	}
	return resp
}
