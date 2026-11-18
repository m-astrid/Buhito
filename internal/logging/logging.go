// Package logging настраивает структурный логгер (stdout) для всех компонентов.
package logging

import (
	"log/slog"
	"os"
)

// New возвращает slog-логгер, пишущий JSON в stdout.
func New(service string) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(h).With("service", service)
}
