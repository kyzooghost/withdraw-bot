package logging

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const (
	testLogFileName     = "withdraw-bot.log"
	testLogMessage      = "logger configured"
	testLogMessageField = "msg"
	testLogSourceField  = "source"
	testLogKey          = "module"
	testLogValue        = "share_price_loss"
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
	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("decode log JSON: %v", err)
	}
	if entry[testLogMessageField] != testLogMessage {
		t.Fatalf("expected log message %q, got %v", testLogMessage, entry[testLogMessageField])
	}
	if entry[testLogKey] != testLogValue {
		t.Fatalf("expected log field %q, got %v", testLogValue, entry[testLogKey])
	}
	if _, ok := entry[testLogSourceField].(map[string]any); !ok {
		t.Fatalf("expected source field, got %v", entry[testLogSourceField])
	}
}
