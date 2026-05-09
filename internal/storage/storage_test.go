package storage

import (
	"context"
	"testing"
	"time"

	"withdraw-bot/internal/core"
)

func TestOpenAppliesMigrations(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	db, err := Open(ctx, ":memory:")

	// Assert
	if err != nil {
		t.Fatalf("expected open to apply migrations: %v", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "SELECT COUNT(*) FROM monitor_snapshots"); err != nil {
		t.Fatalf("expected monitor_snapshots table to exist: %v", err)
	}
}

func TestInsertMonitorResultPersistsModuleStatus(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := NewRepositories(db)
	observedAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	result := core.MonitorResult{
		ModuleID:   core.ModuleSharePriceLoss,
		Status:     core.MonitorStatusWarn,
		ObservedAt: observedAt,
	}

	// Act
	err = repos.InsertMonitorResult(ctx, result, observedAt)

	// Assert
	if err != nil {
		t.Fatalf("insert monitor result: %v", err)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM monitor_snapshots WHERE module_id = ?", string(core.ModuleSharePriceLoss)).Scan(&status); err != nil {
		t.Fatalf("query monitor result: %v", err)
	}
	if status != string(core.MonitorStatusWarn) {
		t.Fatalf("expected status %q, got %q", core.MonitorStatusWarn, status)
	}
}
