package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/withdraw"
)

const (
	errPendingConfirmationExpired        = "pending confirmation %q expired"
	errPendingConfirmationNotFound       = "pending confirmation %q not found"
	errPendingConfirmationWrongRequester = "pending confirmation %q was requested by a different user"
	operationBeginConfirmationTx         = "begin pending confirmation transaction"
	operationCommitConfirmationTx        = "commit pending confirmation transaction"
	operationDecodeEventFields           = "decode event fields"
	operationDeleteConfirmation          = "delete pending confirmation"
	operationInsertConfirmation          = "insert pending confirmation"
	operationInsertWithdrawal            = "insert withdrawal attempt"
	operationListEvent                   = "list event record"
	operationListRecentEvents            = "list recent events"
	operationListThresholdOverride       = "list threshold override"
	operationListThresholds              = "list threshold overrides"
	operationParseEventCreated           = "parse event created_at"
	operationParseConfirmationTime       = "parse pending confirmation %s"
	operationParseOverrideUpdated        = "parse threshold override updated_at"
	operationQueryConfirmation           = "query pending confirmation"
	operationUpsertThreshold             = "upsert threshold override"
	queryListRecentEvents                = `SELECT event_type, message, fields_json, created_at FROM event_records ORDER BY created_at DESC, id DESC LIMIT ?`
	queryListRecentEventsFiltered        = `SELECT event_type, message, fields_json, created_at FROM event_records
		 WHERE event_type IN (?, ?, ?, ?)
		 ORDER BY created_at DESC, id DESC LIMIT ?`
	querySelectConfirmation = `SELECT id, kind, payload_json, requested_by_user_id, expires_at, created_at FROM pending_confirmations WHERE id = ?`
	queryDeleteConfirmation = `DELETE FROM pending_confirmations WHERE id = ?`
	queryInsertConfirmation = `INSERT INTO pending_confirmations(id, kind, payload_json, requested_by_user_id, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	queryInsertWithdrawal   = `INSERT INTO withdrawal_attempts(
		 id, trigger_kind, trigger_module_id, trigger_finding_key, status, tx_hash, nonce, gas_units,
		 max_fee_per_gas_wei, max_priority_fee_per_gas_wei, expected_asset_units, simulation_success,
		 failure_reason, created_at, updated_at
	 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	queryListThresholds  = `SELECT module_id, key, value, updated_by_user_id, updated_at FROM threshold_overrides ORDER BY module_id, key`
	queryUpsertThreshold = `INSERT INTO threshold_overrides(module_id, key, value, updated_by_user_id, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(module_id, key) DO UPDATE SET
		 value = excluded.value,
		 updated_by_user_id = excluded.updated_by_user_id,
		 updated_at = excluded.updated_at`
)

const (
	minRecentEventsLimit = 1
	maxRecentEventsLimit = 50
)

type Repositories struct {
	DB *sql.DB
}

type ThresholdOverride struct {
	ModuleID        string
	Key             string
	Value           string
	UpdatedByUserID int64
	UpdatedAt       time.Time
}

type PendingConfirmation struct {
	ID                string
	Kind              string
	PayloadJSON       string
	RequestedByUserID int64
	ExpiresAt         time.Time
	CreatedAt         time.Time
}

type EventRecord struct {
	EventType core.EventType
	Message   string
	Fields    map[string]string
	CreatedAt time.Time
}

func NewRepositories(db *sql.DB) Repositories {
	return Repositories{DB: db}
}

func (repos Repositories) InsertMonitorResult(ctx context.Context, result core.MonitorResult, createdAt time.Time) error {
	metrics, err := json.Marshal(result.Metrics)
	if err != nil {
		return fmt.Errorf("encode monitor metrics: %w", err)
	}
	findings, err := json.Marshal(result.Findings)
	if err != nil {
		return fmt.Errorf("encode monitor findings: %w", err)
	}
	_, err = repos.DB.ExecContext(
		ctx,
		`INSERT INTO monitor_snapshots(module_id, status, observed_at, metrics_json, findings_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		string(result.ModuleID),
		string(result.Status),
		result.ObservedAt.UTC().Format(time.RFC3339Nano),
		string(metrics),
		string(findings),
		createdAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert monitor result for %s: %w", result.ModuleID, err)
	}
	return nil
}

func (repos Repositories) InsertEvent(ctx context.Context, eventType core.EventType, message string, fields map[string]string, createdAt time.Time) error {
	data, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("encode event fields: %w", err)
	}
	_, err = repos.DB.ExecContext(
		ctx,
		`INSERT INTO event_records(event_type, message, fields_json, created_at) VALUES (?, ?, ?, ?)`,
		string(eventType),
		message,
		string(data),
		createdAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert event record: %w", err)
	}
	return nil
}

func (repos Repositories) Record(ctx context.Context, eventType core.EventType, message string, fields map[string]string, at time.Time) error {
	return repos.InsertEvent(ctx, eventType, message, fields, at)
}

func (repos Repositories) ListRecentEvents(ctx context.Context, includeInfo bool, limit int) ([]EventRecord, error) {
	clampedLimit := clampRecentEventsLimit(limit)
	var rows *sql.Rows
	var err error
	if includeInfo {
		rows, err = repos.DB.QueryContext(ctx, queryListRecentEvents, clampedLimit)
	} else {
		rows, err = repos.DB.QueryContext(
			ctx,
			queryListRecentEventsFiltered,
			string(core.EventWarning),
			string(core.EventError),
			string(core.EventSecurity),
			string(core.EventWithdrawal),
			clampedLimit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf(operationListRecentEvents+": %w", err)
	}
	defer rows.Close()

	events := []EventRecord{}
	for rows.Next() {
		event, err := scanEventRecord(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(operationListRecentEvents+": %w", err)
	}
	return events, nil
}

func (repos Repositories) InsertWithdrawalAttempt(ctx context.Context, attempt withdraw.WithdrawalAttempt) error {
	simulationSuccess := 0
	if attempt.SimulationSuccess {
		simulationSuccess = 1
	}
	_, err := repos.DB.ExecContext(
		ctx,
		queryInsertWithdrawal,
		attempt.ID,
		string(attempt.Trigger.Kind),
		string(attempt.Trigger.ModuleID),
		string(attempt.Trigger.FindingKey),
		string(attempt.Status),
		attempt.TxHash.String(),
		int64(attempt.Nonce),
		int64(attempt.GasUnits),
		nullableBigString(attempt.FeeCaps.MaxFeePerGas),
		nullableBigString(attempt.FeeCaps.MaxPriorityFeePerGas),
		nullableBigString(attempt.ExpectedAssetUnits),
		simulationSuccess,
		attempt.FailureReason,
		attempt.CreatedAt.UTC().Format(time.RFC3339Nano),
		attempt.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf(operationInsertWithdrawal+": %w", err)
	}
	return nil
}

func (repos Repositories) UpsertThresholdOverride(ctx context.Context, moduleID string, key string, value string, userID int64, at time.Time) error {
	_, err := repos.DB.ExecContext(
		ctx,
		queryUpsertThreshold,
		moduleID,
		key,
		value,
		userID,
		at.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf(operationUpsertThreshold+": %w", err)
	}
	return nil
}

func (repos Repositories) ListThresholdOverrides(ctx context.Context) ([]ThresholdOverride, error) {
	rows, err := repos.DB.QueryContext(ctx, queryListThresholds)
	if err != nil {
		return nil, fmt.Errorf(operationListThresholds+": %w", err)
	}
	defer rows.Close()

	var overrides []ThresholdOverride
	for rows.Next() {
		var override ThresholdOverride
		var updatedAt string
		if err := rows.Scan(&override.ModuleID, &override.Key, &override.Value, &override.UpdatedByUserID, &updatedAt); err != nil {
			return nil, fmt.Errorf(operationListThresholdOverride+": %w", err)
		}
		parsedUpdatedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf(operationParseOverrideUpdated+": %w", err)
		}
		override.UpdatedAt = parsedUpdatedAt
		overrides = append(overrides, override)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(operationListThresholds+": %w", err)
	}
	return overrides, nil
}

func (repos Repositories) InsertPendingConfirmation(ctx context.Context, confirmation PendingConfirmation) error {
	_, err := repos.DB.ExecContext(
		ctx,
		queryInsertConfirmation,
		confirmation.ID,
		confirmation.Kind,
		confirmation.PayloadJSON,
		confirmation.RequestedByUserID,
		confirmation.ExpiresAt.UTC().Format(time.RFC3339Nano),
		confirmation.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf(operationInsertConfirmation+": %w", err)
	}
	return nil
}

func (repos Repositories) ConsumePendingConfirmation(ctx context.Context, id string, at time.Time) (PendingConfirmation, error) {
	return repos.consumePendingConfirmation(ctx, id, at, 0, false)
}

func (repos Repositories) ConsumePendingConfirmationForUser(ctx context.Context, id string, userID int64, at time.Time) (PendingConfirmation, error) {
	return repos.consumePendingConfirmation(ctx, id, at, userID, true)
}

func (repos Repositories) consumePendingConfirmation(ctx context.Context, id string, at time.Time, userID int64, requireUser bool) (PendingConfirmation, error) {
	tx, err := repos.DB.BeginTx(ctx, nil)
	if err != nil {
		return PendingConfirmation{}, fmt.Errorf(operationBeginConfirmationTx+": %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	confirmation, err := selectPendingConfirmation(ctx, tx, id)
	if err != nil {
		return PendingConfirmation{}, err
	}
	if requireUser && confirmation.RequestedByUserID != userID {
		return PendingConfirmation{}, fmt.Errorf(errPendingConfirmationWrongRequester, id)
	}
	if _, err := tx.ExecContext(ctx, queryDeleteConfirmation, id); err != nil {
		return PendingConfirmation{}, fmt.Errorf(operationDeleteConfirmation+": %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PendingConfirmation{}, fmt.Errorf(operationCommitConfirmationTx+": %w", err)
	}
	committed = true

	if !at.Before(confirmation.ExpiresAt) {
		return PendingConfirmation{}, fmt.Errorf(errPendingConfirmationExpired, id)
	}
	return confirmation, nil
}

func selectPendingConfirmation(ctx context.Context, tx *sql.Tx, id string) (PendingConfirmation, error) {
	var confirmation PendingConfirmation
	var expiresAt string
	var createdAt string
	if err := tx.QueryRowContext(ctx, querySelectConfirmation, id).Scan(&confirmation.ID, &confirmation.Kind, &confirmation.PayloadJSON, &confirmation.RequestedByUserID, &expiresAt, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return PendingConfirmation{}, fmt.Errorf(errPendingConfirmationNotFound, id)
		}
		return PendingConfirmation{}, fmt.Errorf(operationQueryConfirmation+": %w", err)
	}
	parsedExpiresAt, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return PendingConfirmation{}, fmt.Errorf(operationParseConfirmationTime+": %w", "expires_at", err)
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return PendingConfirmation{}, fmt.Errorf(operationParseConfirmationTime+": %w", "created_at", err)
	}
	confirmation.ExpiresAt = parsedExpiresAt
	confirmation.CreatedAt = parsedCreatedAt
	return confirmation, nil
}

func scanEventRecord(rows *sql.Rows) (EventRecord, error) {
	var event EventRecord
	var eventType string
	var fieldsJSON string
	var createdAt string
	if err := rows.Scan(&eventType, &event.Message, &fieldsJSON, &createdAt); err != nil {
		return EventRecord{}, fmt.Errorf(operationListEvent+": %w", err)
	}
	fields := map[string]string{}
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		return EventRecord{}, fmt.Errorf(operationDecodeEventFields+": %w", err)
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return EventRecord{}, fmt.Errorf(operationParseEventCreated+": %w", err)
	}
	event.EventType = core.EventType(eventType)
	event.Fields = fields
	event.CreatedAt = parsedCreatedAt
	return event, nil
}

func clampRecentEventsLimit(limit int) int {
	if limit < minRecentEventsLimit {
		return minRecentEventsLimit
	}
	if limit > maxRecentEventsLimit {
		return maxRecentEventsLimit
	}
	return limit
}

func nullableBigString(value *big.Int) any {
	if value == nil {
		return nil
	}
	return value.String()
}
