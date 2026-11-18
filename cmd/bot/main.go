// cmd/bot — Telegram-бот-тренажёр спикинга.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/evgshumakova/mini-owl/internal/bot"
	"github.com/evgshumakova/mini-owl/internal/config"
	"github.com/evgshumakova/mini-owl/internal/logging"
)

func main() {
	log := logging.New("mini-owl-bot")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadBot()
	if err != nil {
		log.Error("конфигурация", "err", err)
		os.Exit(1)
	}
	log.Info("конфигурация", "conn", cfg.String())

	b, err := bot.NewBot(cfg.Token, cfg.GRPCAddr, log)
	if err != nil {
		log.Error("запуск бота", "err", err)
		os.Exit(1)
	}

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		log.Info("останавливаем бота...")
		b.Stop()
		close(done)
	}()

	log.Info("бот запущен (long polling)")
	b.Start() // блокируется до b.Stop()
	<-done
	log.Info("бот остановлен")
}
