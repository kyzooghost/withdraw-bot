package monitor

import (
	"context"
	"errors"
	"strings"
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
	results, err := service.RunOnce(ctx)
	snapshot := service.Snapshot()

	// Assert
	if err != nil {
		t.Fatalf("run monitor once: %v", err)
	}
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

func TestRunOnceReturnsUnknownWhenModuleFails(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	observedAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	module := fakeModule{id: core.ModuleSharePriceLoss, err: errors.New("reader failed")}
	service := NewService([]Module{module}, storage.NewRepositories(db), core.FixedClock{Value: observedAt})

	// Act
	results, err := service.RunOnce(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected module error")
	}
	if !strings.Contains(err.Error(), string(core.ModuleSharePriceLoss)) {
		t.Fatalf("expected error to include module id, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.MonitorStatusUnknown {
		t.Fatalf("expected unknown status, got %q", results[0].Status)
	}
	if results[0].ObservedAt != observedAt {
		t.Fatalf("expected observed time %s, got %s", observedAt, results[0].ObservedAt)
	}
}

func TestRunOnceReturnsStorageError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	module := fakeModule{
		id: core.ModuleSharePriceLoss,
		result: core.MonitorResult{
			ModuleID: core.ModuleSharePriceLoss,
			Status:   core.MonitorStatusOK,
		},
	}
	service := NewService([]Module{module}, storage.NewRepositories(db), core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)})

	// Act
	_, err = service.RunOnce(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected storage error")
	}
	if !strings.Contains(err.Error(), string(core.ModuleSharePriceLoss)) {
		t.Fatalf("expected error to include module id, got %v", err)
	}
}

func TestRunOnceClonesReturnedResultsAndSnapshot(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	module := fakeModule{
		id: core.ModuleSharePriceLoss,
		result: core.MonitorResult{
			ModuleID: core.ModuleSharePriceLoss,
			Status:   core.MonitorStatusWarn,
			Findings: []core.Finding{{
				Key:      core.FindingSharePriceLoss,
				Severity: core.SeverityWarn,
				Message:  "loss",
				Evidence: map[string]string{"loss_bps": "50"},
			}},
		},
	}
	service := NewService([]Module{module}, storage.NewRepositories(db), core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)})

	// Act
	results, err := service.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run monitor once: %v", err)
	}
	results[0].Findings[0].Evidence["loss_bps"] = "mutated"
	snapshot := service.Snapshot()
	snapshot[core.ModuleSharePriceLoss].Findings[0].Evidence["loss_bps"] = "snapshot-mutated"
	nextSnapshot := service.Snapshot()

	// Assert
	if nextSnapshot[core.ModuleSharePriceLoss].Findings[0].Evidence["loss_bps"] != "50" {
		t.Fatalf("expected snapshot evidence to remain cloned, got %q", nextSnapshot[core.ModuleSharePriceLoss].Findings[0].Evidence["loss_bps"])
	}
}

func TestRunOnceReturnsErrorForNilModule(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	service := NewService([]Module{nil}, storage.NewRepositories(db), core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)})

	// Act
	_, err = service.RunOnce(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected nil module error")
	}
}

func TestRunOnceReturnsErrorForEmptyModuleID(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	service := NewService([]Module{fakeModule{}}, storage.NewRepositories(db), core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)})

	// Act
	_, err = service.RunOnce(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected empty module id error")
	}
}

func TestRunOnceDefaultsNilClockOnModuleError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	service := NewService([]Module{fakeModule{id: core.ModuleSharePriceLoss, err: errors.New("failed")}}, storage.NewRepositories(db), nil)

	// Act
	results, err := service.RunOnce(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected module error")
	}
	if results[0].Status != core.MonitorStatusUnknown {
		t.Fatalf("expected unknown status, got %q", results[0].Status)
	}
	if results[0].ObservedAt.IsZero() {
		t.Fatal("expected observed time from default clock")
	}
}

func TestRunLoopReturnsErrorForZeroInterval(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	service := NewService(nil, storage.NewRepositories(db), core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)})

	// Act
	err = service.RunLoop(ctx, 0)

	// Assert
	if err == nil {
		t.Fatal("expected interval error")
	}
}

func TestRunLoopReturnsRunOnceError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	module := fakeModule{
		id: core.ModuleSharePriceLoss,
		result: core.MonitorResult{
			ModuleID: core.ModuleSharePriceLoss,
			Status:   core.MonitorStatusOK,
		},
	}
	service := NewService([]Module{module}, storage.NewRepositories(db), core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)})

	// Act
	err = service.RunLoop(ctx, time.Hour)

	// Assert
	if err == nil {
		t.Fatal("expected run once error")
	}
	if !strings.Contains(err.Error(), string(core.ModuleSharePriceLoss)) {
		t.Fatalf("expected error to include module id, got %v", err)
	}
}

type fakeModule struct {
	id     core.MonitorModuleID
	result core.MonitorResult
	err    error
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
	return module.result, module.err
}
