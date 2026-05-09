package logging

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testLogFileName = "withdraw-bot.log"
	testLogMessage  = "logger configured"
	testLogKey      = "module"
	testLogValue    = "share_price_loss"
)

func TestNewWritesJSONLogToFile(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), testLogFileName)

	// Act
	logger, closer := New(Config{FilePath: path, MaxSizeMB: 1, MaxBackups: 1, MaxAgeDays: 1})
	logger.InfoContext(context.Background(), testLogMessage, testLogKey, testLogValue)
	if err := closer.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	// Assert
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), `"msg":"`+testLogMessage+`"`) {
		t.Fatalf("expected log message, got %q", string(data))
	}
	if !strings.Contains(string(data), `"`+testLogKey+`":"`+testLogValue+`"`) {
		t.Fatalf("expected log field, got %q", string(data))
	}
}
