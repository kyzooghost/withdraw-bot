package storage

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/withdraw"

	"github.com/ethereum/go-ethereum/common"
)

const (
	testEventMessage           = "risk condition detected"
	testEventFieldKey          = "module"
	testEventFieldValue        = "share_price_loss"
	testEventFieldsJSON        = `{"module":"share_price_loss"}`
	testEventClampMessage      = "warning event %02d"
	testEventInfoMessage       = "monitor tick completed"
	testEventWarnMessage       = "warning event"
	testEventErrorMessage      = "error event"
	testEventSecurityMessage   = "security event"
	testEventWithdrawalMessage = "withdrawal event"
	testMelbourneTimezone      = "Australia/Melbourne"
	testOverrideKey            = "loss_warn_bps"
	testOverrideOldValue       = "50"
	testOverrideNewValue       = "75"
	testPendingID              = "threshold:share_price_loss:loss_warn_bps:1"
	testPendingKind            = "threshold"
	testPendingPayload         = `{"module_id":"share_price_loss","key":"loss_warn_bps","value":"75"}`
	testWithdrawalAttemptID    = "20260509T010000.000000000Z-urgent-withdraw_liquidity-idle_liquidity"
	testWithdrawalTxHash       = "0x1111111111111111111111111111111111111111111111111111111111111111"
	testTableEvents            = "event_records"
	testIndexEventsTime        = "idx_event_records_created_at_id"
	testTableMonitor           = "monitor_snapshots"
	testTableOverrides         = "threshold_overrides"
	testTablePending           = "pending_confirmations"
	testTableWithdrawals       = "withdrawal_attempts"
	testUTCObservedAt          = "2026-05-08T15:00:00Z"
	testUTCCreatedAt           = "2026-05-08T16:30:00Z"
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

func TestOpenCreatesRecentEventIndex(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	db, err := Open(ctx, ":memory:")

	// Assert
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?", testIndexEventsTime).Scan(&name); err != nil {
		t.Fatalf("expected %s index to exist: %v", testIndexEventsTime, err)
	}
	if name != testIndexEventsTime {
		t.Fatalf("expected index name %q, got %q", testIndexEventsTime, name)
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

func TestListRecentEventsExcludesInfoByDefault(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := NewRepositories(db)
	at := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	if err := repos.InsertEvent(ctx, core.EventInfo, testEventInfoMessage, nil, at.Add(5*time.Minute)); err != nil {
		t.Fatalf("insert info event: %v", err)
	}
	if err := repos.InsertEvent(ctx, core.EventWarning, testEventWarnMessage, map[string]string{testEventFieldKey: testEventFieldValue}, at.Add(time.Minute)); err != nil {
		t.Fatalf("insert warning event: %v", err)
	}
	if err := repos.InsertEvent(ctx, core.EventSecurity, testEventSecurityMessage, nil, at.Add(2*time.Minute)); err != nil {
		t.Fatalf("insert security event: %v", err)
	}
	if err := repos.InsertEvent(ctx, core.EventWithdrawal, testEventWithdrawalMessage, nil, at.Add(3*time.Minute)); err != nil {
		t.Fatalf("insert withdrawal event: %v", err)
	}
	if err := repos.InsertEvent(ctx, core.EventError, testEventErrorMessage, nil, at.Add(4*time.Minute)); err != nil {
		t.Fatalf("insert error event: %v", err)
	}

	// Act
	result, err := repos.ListRecentEvents(ctx, false, 10)

	// Assert
	if err != nil {
		t.Fatalf("list recent events: %v", err)
	}
	if len(result) != 4 {
		t.Fatalf("expected four non-info events, got %d", len(result))
	}
	if result[0].EventType != core.EventError {
		t.Fatalf("expected newest error first, got %s", result[0].EventType)
	}
	if result[1].EventType != core.EventWithdrawal {
		t.Fatalf("expected withdrawal second, got %s", result[1].EventType)
	}
	if result[2].EventType != core.EventSecurity {
		t.Fatalf("expected security third, got %s", result[2].EventType)
	}
	if result[3].EventType != core.EventWarning {
		t.Fatalf("expected warning fourth, got %s", result[3].EventType)
	}
	if result[3].Fields[testEventFieldKey] != testEventFieldValue {
		t.Fatalf("expected decoded event fields, got %v", result[3].Fields)
	}
}

func TestListRecentEventsIncludesInfoWhenRequested(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := NewRepositories(db)
	at := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	if err := repos.InsertEvent(ctx, core.EventInfo, testEventInfoMessage, nil, at); err != nil {
		t.Fatalf("insert info event: %v", err)
	}
	if err := repos.InsertEvent(ctx, core.EventWarning, testEventWarnMessage, nil, at.Add(time.Minute)); err != nil {
		t.Fatalf("insert warning event: %v", err)
	}

	// Act
	result, err := repos.ListRecentEvents(ctx, true, 10)

	// Assert
	if err != nil {
		t.Fatalf("list recent events: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected two events, got %d", len(result))
	}
	if result[0].EventType != core.EventWarning {
		t.Fatalf("expected warning first, got %s", result[0].EventType)
	}
	if result[1].EventType != core.EventInfo {
		t.Fatalf("expected info second, got %s", result[1].EventType)
	}
}

func TestListRecentEventsClampsLimitToRange(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := NewRepositories(db)
	start := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	for index := 0; index < maxRecentEventsLimit+1; index++ {
		message := fmt.Sprintf(testEventClampMessage, index)
		if err := repos.InsertEvent(ctx, core.EventWarning, message, nil, start.Add(time.Duration(index)*time.Minute)); err != nil {
			t.Fatalf("insert warning event %d: %v", index, err)
		}
	}

	// Act
	maxResult, err := repos.ListRecentEvents(ctx, true, maxRecentEventsLimit+1)
	if err != nil {
		t.Fatalf("list recent events with high limit: %v", err)
	}
	minResult, err := repos.ListRecentEvents(ctx, true, 0)

	// Assert
	if err != nil {
		t.Fatalf("list recent events with low limit: %v", err)
	}
	if len(maxResult) != maxRecentEventsLimit {
		t.Fatalf("expected max limit %d, got %d", maxRecentEventsLimit, len(maxResult))
	}
	if len(minResult) != minRecentEventsLimit {
		t.Fatalf("expected min limit %d, got %d", minRecentEventsLimit, len(minResult))
	}
	expectedNewestMessage := fmt.Sprintf(testEventClampMessage, maxRecentEventsLimit)
	if minResult[0].Message != expectedNewestMessage {
		t.Fatalf("expected low limit to return newest message %q, got %q", expectedNewestMessage, minResult[0].Message)
	}
}

func TestInsertWithdrawalAttemptPersistsSubmittedAttempt(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := NewRepositories(db)
	at := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	attempt := withdraw.WithdrawalAttempt{
		ID: testWithdrawalAttemptID,
		Trigger: withdraw.WithdrawalTrigger{
			Kind:       withdraw.TriggerKindUrgent,
			ModuleID:   core.ModuleWithdrawLiquidity,
			FindingKey: core.FindingIdleLiquidity,
		},
		Status:             withdraw.WithdrawalStatusSubmitted,
		TxHash:             common.HexToHash(testWithdrawalTxHash),
		Nonce:              7,
		GasUnits:           21000,
		FeeCaps:            withdraw.FeeCaps{MaxFeePerGas: big.NewInt(100), MaxPriorityFeePerGas: big.NewInt(2)},
		ExpectedAssetUnits: big.NewInt(12345),
		SimulationSuccess:  true,
		CreatedAt:          at,
		UpdatedAt:          at,
	}

	// Act
	err = repos.InsertWithdrawalAttempt(ctx, attempt)

	// Assert
	if err != nil {
		t.Fatalf("insert withdrawal attempt: %v", err)
	}
	var status string
	var triggerModuleID string
	var txHash string
	var maxFeePerGas string
	var expectedAssetUnits string
	if err := db.QueryRowContext(ctx, `SELECT status, trigger_module_id, tx_hash, max_fee_per_gas_wei, expected_asset_units FROM withdrawal_attempts WHERE id = ?`, testWithdrawalAttemptID).Scan(&status, &triggerModuleID, &txHash, &maxFeePerGas, &expectedAssetUnits); err != nil {
		t.Fatalf("query withdrawal attempt: %v", err)
	}
	if status != string(withdraw.WithdrawalStatusSubmitted) {
		t.Fatalf("expected status %q, got %q", withdraw.WithdrawalStatusSubmitted, status)
	}
	if triggerModuleID != string(core.ModuleWithdrawLiquidity) {
		t.Fatalf("expected trigger module %q, got %q", core.ModuleWithdrawLiquidity, triggerModuleID)
	}
	if txHash != testWithdrawalTxHash {
		t.Fatalf("expected tx hash %q, got %q", testWithdrawalTxHash, txHash)
	}
	if maxFeePerGas != "100" {
		t.Fatalf("expected max fee per gas, got %q", maxFeePerGas)
	}
	if expectedAssetUnits != "12345" {
		t.Fatalf("expected asset units, got %q", expectedAssetUnits)
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
