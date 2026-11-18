# Buhito

Тренажёр спикинга: Telegram-бот даёт русскую фразу, пользователь произносит перевод
вслух (голосовое) или пишет текстом, затем сверяется с правильным ответом.

- Целевые языки: итальянский (`it`), английский (`en`); родной — русский.
- Отбор карточек: случайный, без повторов в сессии, по уровню CEFR,
  грамматическим и лексическим темам, в заданном количестве за сессию.
- Архитектура: два Go-бинарника — `api` (gRPC-каталог карточек, SQLite) и
  `bot` (Telegram, long polling). Подробности — в [PLAN.md](PLAN.md).

## Запуск локально (для разработки)

Нужны: Go 1.26+, `buf` (`go install github.com/bufbuild/buf/cmd/buf@latest`),
`grpcurl` (опционально).

```bash
make generate    # сгенерировать gRPC-стабы (после правок proto)
make migrate     # применить миграции (создаёт ./mini-owl.db)
make api         # терминал 1: gRPC API на :50051
BOT_TOKEN=... make bot   # терминал 2: бот
make test        # unit + e2e тесты
```

Токен бота — у [@BotFather](https://t.me/BotFather), передаётся только через
переменную окружения `BOT_TOKEN`.

## Запуск в Docker

```bash
BOT_TOKEN=123:ABC docker compose up -d --build
```

- SQLite живёт в docker-volume `owl-data` (`/data/mini-owl.db`).
- Long polling: публичный IP/домен/TLS не нужны.

## Наполнение каталога

RPC `UpsertCard` / `DeleteCard` (gRPC, см. `proto/trainer/v1/trainer.proto`)
и `Repo.UpsertTopic` — под будущий генератор тем и карточек.
Для ручной проверки можно вставлять строки напрямую в SQLite (`mini-owl.db`).

## Бэкап

```bash
sqlite3 /data/mini-owl.db ".backup '/data/backup-$(date +%F).db'"
```

## Перезапуски

- Перезапуск `api` безопасен (бот переживает обрыв соединений).
- Перезапуск `bot` сбрасывает in-memory сессии — пользователи начинают заново с `/train`.

## Сборка локально без Docker

```bash
CGO_ENABLED=0 go build -o bin/api ./cmd/api
CGO_ENABLED=0 go build -o bin/bot ./cmd/bot
```
