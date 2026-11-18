// Package config читает конфигурацию компонентов из переменных окружения.
package config

import (
	"errors"
	"fmt"
	"os"
)

// API — конфигурация gRPC-сервера (cmd/api).
type API struct {
	DBPath   string // путь к файлу SQLite
	GRPCAddr string // адрес, который слушает gRPC-сервер
}

// Bot — конфигурация Telegram-бота (cmd/bot).
type Bot struct {
	Token    string // токен от @BotFather (обязательный)
	GRPCAddr string // адрес gRPC API
}

// Дефолты из плана (секция «Запуск и эксплуатация»).
const (
	defaultDBPath   = "./mini-owl.db"
	defaultGRPCAddr = ":50051"
	defaultBotGRPC  = "localhost:50051"
)

// LoadAPI читает конфигурацию API из окружения.
func LoadAPI() (API, error) {
	cfg := API{
		DBPath:   envOr("DB_PATH", defaultDBPath),
		GRPCAddr: envOr("GRPC_ADDR", defaultGRPCAddr),
	}
	if cfg.DBPath == "" {
		return cfg, errors.New("DB_PATH не может быть пустым")
	}
	if cfg.GRPCAddr == "" {
		return cfg, errors.New("GRPC_ADDR не может быть пустым")
	}
	return cfg, nil
}

// LoadBot читает конфигурацию бота из окружения.
func LoadBot() (Bot, error) {
	cfg := Bot{
		Token:    os.Getenv("BOT_TOKEN"),
		GRPCAddr: envOr("GRPC_ADDR", defaultBotGRPC),
	}
	if cfg.Token == "" {
		return cfg, errors.New("BOT_TOKEN не задан: получи токен у @BotFather и экспортируй переменную окружения")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// String используется в логах (без секретов).
func (b Bot) String() string {
	return fmt.Sprintf("GRPCAddr=%s Token=***%s", b.GRPCAddr, maskTail(b.Token))
}

func maskTail(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[len(s)-4:]
}
