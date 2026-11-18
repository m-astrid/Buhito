// cmd/api — gRPC-сервер каталога карточек.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/evgshumakova/mini-owl/internal/catalog"
	"github.com/evgshumakova/mini-owl/internal/config"
	"github.com/evgshumakova/mini-owl/internal/logging"
	"github.com/evgshumakova/mini-owl/internal/server"
	"github.com/evgshumakova/mini-owl/internal/storage"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "применить миграции и выйти")
	flag.Parse()

	log := logging.New("mini-owl-api")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadAPI()
	if err != nil {
		log.Error("конфигурация", "err", err)
		os.Exit(1)
	}
	log.Info("конфигурация", "db_path", cfg.DBPath, "grpc_addr", cfg.GRPCAddr)

	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Error("открытие БД", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if *migrateOnly {
		log.Info("миграции применены, выход")
		return
	}

	if err := server.Run(cfg.GRPCAddr, server.New(catalog.NewRepo(db)), log); err != nil {
		log.Error("сервер", "err", err)
		os.Exit(1)
	}
	_ = ctx // завершение по сигналу обрабатывается через остановку процесса (MVP)
}
