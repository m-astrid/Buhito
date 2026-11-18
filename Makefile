# Правила запуска
#   make generate  — сгенерировать gRPC-стабы из proto (buf)
#   make migrate   — применить миграции к SQLite
#   make api       — запустить gRPC API
#   make bot       — запустить Telegram-бота (BOT_TOKEN=... make bot)
#   make test      — unit + e2e тесты

GO      ?= go
BUF     ?= $(HOME)/go/bin/buf
DB_PATH ?= ./mini-owl.db
GRPC_ADDR ?= :50051

.PHONY: all generate migrate api bot test tidy build clean

all: build

tidy:
	$(GO) mod tidy

build:
	$(GO) build ./...

generate:
	@mkdir -p .buf-cache
	XDG_CACHE_HOME=$$(pwd)/.buf-cache $(BUF) generate

migrate:
	$(GO) run ./cmd/api -migrate-only

api:
	GRPC_ADDR=$(GRPC_ADDR) DB_PATH=$(DB_PATH) $(GO) run ./cmd/api

bot:
	$(GO) run ./cmd/bot

test:
	$(GO) test ./...

clean:
	rm -rf bin .buf-cache
