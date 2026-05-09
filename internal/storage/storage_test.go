package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"withdraw-bot/internal/core"
)

const (
	testEventMessage      = "risk condition detected"
	testEventFieldKey     = "module"
	testEventFieldValue   = "share_price_loss"
	testEventFieldsJSON   = `{"module":"share_price_loss"}`
	testMelbourneTimezone = "Australia/Melbourne"
	testOverrideKey       = "loss_warn_bps"
	testOverrideOldValue  = "50"
	testOverrideNewValue  = "75"
	testPendingID         = "threshold:share_price_loss:loss_warn_bps:1"
	testPendingKind       = "threshold"
	testPendingPayload    = `{"module_id":"share_price_loss","key":"loss_warn_bps","value":"75"}`
	testTableEvents       = "event_records"
	testTableMonitor      = "monitor_snapshots"
	testTableOverrides    = "threshold_overrides"
	testTablePending      = "pending_confirmations"
	testTableWithdrawals  = "withdrawal_attempts"
	testUTCObservedAt     = "2026-05-08T15:00:00Z"
	testUTCCreatedAt      = "2026-05-08T16:30:00Z"
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
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", testTableMonitor).Scan(&name); err != nil {
		t.Fatalf("expected %s table to exist: %v", testTableMonitor, err)
	}
	if name != testTableMonitor {
		t.Fatalf("expected table name %q, got %q", testTableMonitor, name)
	}
}

func TestOpenCreatesAllStorageTables(t *testing.T) {
	// Arrange
	ctx := context.Background()
	tables := []string{
		testTableMonitor,
		testTableEvents,
		testTableOverrides,
		testTablePending,
		testTableWithdrawals,
	}

	// Act
	db, err := Open(ctx, ":memory:")

	// Assert
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	for _, table := range tables {
		var name string
		if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name); err != nil {
			t.Fatalf("expected %s table to exist: %v", table, err)
		}
		if name != table {
			t.Fatalf("expected table name %q, got %q", table, name)
		}
	}
}

func TestOpenLimitsPoolToSingleConnection(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	db, err := Open(ctx, ":memory:")

	// Assert
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if db.Stats().MaxOpenConnections != 1 {
		t.Fatalf("expected max open connections 1, got %d", db.Stats().MaxOpenConnections)
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

func TestInsertMonitorResultStoresTimestampsInUTC(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := NewRepositories(db)
	location, err := time.LoadLocation(testMelbourneTimezone)
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	observedAt := time.Date(2026, 5, 9, 1, 0, 0, 0, location)
	createdAt := time.Date(2026, 5, 9, 2, 30, 0, 0, location)
	result := core.MonitorResult{
		ModuleID:   core.ModuleSharePriceLoss,
		Status:     core.MonitorStatusWarn,
		ObservedAt: observedAt,
	}

	// Act
	err = repos.InsertMonitorResult(ctx, result, createdAt)

	// Assert
	if err != nil {
		t.Fatalf("insert monitor result: %v", err)
	}
	var storedObservedAt string
	var storedCreatedAt string
	if err := db.QueryRowContext(ctx, "SELECT observed_at, created_at FROM monitor_snapshots WHERE module_id = ?", string(core.ModuleSharePriceLoss)).Scan(&storedObservedAt, &storedCreatedAt); err != nil {
		t.Fatalf("query monitor result: %v", err)
	}
	if storedObservedAt != testUTCObservedAt {
		t.Fatalf("expected observed_at %q, got %q", testUTCObservedAt, storedObservedAt)
	}
	if storedCreatedAt != testUTCCreatedAt {
		t.Fatalf("expected created_at %q, got %q", testUTCCreatedAt, storedCreatedAt)
	}
}

func TestInsertEventPersistsRecord(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := NewRepositories(db)
	createdAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	fields := map[string]string{testEventFieldKey: testEventFieldValue}

	// Act
	err = repos.InsertEvent(ctx, core.EventWarning, testEventMessage, fields, createdAt)

	// Assert
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
	var eventType string
	var message string
	var fieldsJSON string
	if err := db.QueryRowContext(ctx, "SELECT event_type, message, fields_json FROM event_records WHERE event_type = ?", string(core.EventWarning)).Scan(&eventType, &message, &fieldsJSON); err != nil {
		t.Fatalf("query event: %v", err)
	}
	if eventType != string(core.EventWarning) {
		t.Fatalf("expected event type %q, got %q", core.EventWarning, eventType)
	}
	if message != testEventMessage {
		t.Fatalf("expected message %q, got %q", testEventMessage, message)
	}
	if fieldsJSON != testEventFieldsJSON {
		t.Fatalf("expected fields JSON, got %q", fieldsJSON)
	}
}

func TestInsertEventStoresCreatedAtInUTC(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := NewRepositories(db)
	location, err := time.LoadLocation(testMelbourneTimezone)
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	createdAt := time.Date(2026, 5, 9, 2, 30, 0, 0, location)

	// Act
	err = repos.InsertEvent(ctx, core.EventWarning, testEventMessage, nil, createdAt)

	// Assert
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
	var storedCreatedAt string
	if err := db.QueryRowContext(ctx, "SELECT created_at FROM event_records WHERE event_type = ?", string(core.EventWarning)).Scan(&storedCreatedAt); err != nil {
		t.Fatalf("query event: %v", err)
	}
	if storedCreatedAt != testUTCCreatedAt {
		t.Fatalf("expected created_at %q, got %q", testUTCCreatedAt, storedCreatedAt)
	}
}

func TestUpsertThresholdOverrideReplacesExistingValue(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := NewRepositories(db)
	oldAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	newAt := time.Date(2026, 5, 9, 2, 0, 0, 0, time.UTC)

	// Act
	if err := repos.UpsertThresholdOverride(ctx, string(core.ModuleSharePriceLoss), testOverrideKey, testOverrideOldValue, 1, oldAt); err != nil {
		t.Fatalf("upsert old threshold override: %v", err)
	}
	if err := repos.UpsertThresholdOverride(ctx, string(core.ModuleSharePriceLoss), testOverrideKey, testOverrideNewValue, 2, newAt); err != nil {
		t.Fatalf("upsert new threshold override: %v", err)
	}
	result, err := repos.ListThresholdOverrides(ctx)

	// Assert
	if err != nil {
		t.Fatalf("list threshold overrides: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected one threshold override, got %d", len(result))
	}
	if result[0].Value != testOverrideNewValue {
		t.Fatalf("expected replacement value %q, got %q", testOverrideNewValue, result[0].Value)
	}
	if result[0].UpdatedByUserID != 2 {
		t.Fatalf("expected replacement user id 2, got %d", result[0].UpdatedByUserID)
	}
	if !result[0].UpdatedAt.Equal(newAt) {
		t.Fatalf("expected replacement timestamp %s, got %s", newAt, result[0].UpdatedAt)
	}
}

func TestConsumePendingConfirmationReturnsErrorWhenExpired(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := NewRepositories(db)
	createdAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 5, 9, 1, 5, 0, 0, time.UTC)
	now := time.Date(2026, 5, 9, 1, 6, 0, 0, time.UTC)
	confirmation := PendingConfirmation{
		ID:                testPendingID,
		Kind:              testPendingKind,
		PayloadJSON:       testPendingPayload,
		RequestedByUserID: 1,
		ExpiresAt:         expiresAt,
		CreatedAt:         createdAt,
	}
	if err := repos.InsertPendingConfirmation(ctx, confirmation); err != nil {
		t.Fatalf("insert pending confirmation: %v", err)
	}

	// Act
	_, err = repos.ConsumePendingConfirmation(ctx, testPendingID, now)

	// Assert
	if err == nil {
		t.Fatalf("expected expired pending confirmation error")
	}
	_, err = repos.ConsumePendingConfirmation(ctx, testPendingID, createdAt)
	if err == nil {
		t.Fatalf("expected consumed expired confirmation to be unavailable")
	}
	expected := fmt.Sprintf(errPendingConfirmationNotFound, testPendingID)
	if err.Error() != expected {
		t.Fatalf("expected consumed confirmation error %q, got %q", expected, err.Error())
	}
}
