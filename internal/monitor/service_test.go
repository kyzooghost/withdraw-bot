package monitor

import (
	"context"
	"testing"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/storage"
)

func TestRunOnceRecordsLatestResult(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	observedAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	module := fakeModule{
		id: core.ModuleSharePriceLoss,
		result: core.MonitorResult{
			ModuleID:   core.ModuleSharePriceLoss,
			Status:     core.MonitorStatusWarn,
			ObservedAt: observedAt,
		},
	}
	service := NewService([]Module{module}, storage.NewRepositories(db), core.FixedClock{Value: observedAt})

	// Act
	results := service.RunOnce(ctx)
	snapshot := service.Snapshot()

	// Assert
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if snapshot[core.ModuleSharePriceLoss].Status != core.MonitorStatusWarn {
		t.Fatalf("expected latest status %q, got %q", core.MonitorStatusWarn, snapshot[core.ModuleSharePriceLoss].Status)
	}
	var storedStatus string
	if err := db.QueryRowContext(ctx, "SELECT status FROM monitor_snapshots WHERE module_id = ?", string(core.ModuleSharePriceLoss)).Scan(&storedStatus); err != nil {
		t.Fatalf("query monitor snapshot: %v", err)
	}
	if storedStatus != string(core.MonitorStatusWarn) {
		t.Fatalf("expected stored status %q, got %q", core.MonitorStatusWarn, storedStatus)
	}
}

type fakeModule struct {
	id     core.MonitorModuleID
	result core.MonitorResult
}

func (module fakeModule) ID() core.MonitorModuleID {
	return module.id
}

func (module fakeModule) ValidateConfig(ctx context.Context) error {
	return nil
}

func (module fakeModule) Bootstrap(ctx context.Context) (map[string]any, error) {
	return nil, nil
}

func (module fakeModule) Monitor(ctx context.Context) (core.MonitorResult, error) {
	return module.result, nil
}
