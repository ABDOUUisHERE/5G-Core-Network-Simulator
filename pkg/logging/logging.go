package logging

import (
	"log/slog"
	"os"
)

func Configure(service string) {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})).With("service", service))
}
