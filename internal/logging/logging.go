package logging

import (
	"io"
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	FilePath   string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
}

func New(config Config) (*slog.Logger, io.Closer) {
	rotating := &lumberjack.Logger{
		Filename:   config.FilePath,
		MaxSize:    config.MaxSizeMB,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAgeDays,
		Compress:   true,
	}
	writer := io.MultiWriter(os.Stdout, rotating)
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{AddSource: true})
	return slog.New(handler), rotating
}
