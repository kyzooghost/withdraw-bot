package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"withdraw-bot/internal/core"
)

const (
	errPendingConfirmationExpired  = "pending confirmation %q expired"
	errPendingConfirmationNotFound = "pending confirmation %q not found"
	operationBeginConfirmationTx   = "begin pending confirmation transaction"
	operationCommitConfirmationTx  = "commit pending confirmation transaction"
	operationDeleteConfirmation    = "delete pending confirmation"
	operationInsertConfirmation    = "insert pending confirmation"
	operationListThresholdOverride = "list threshold override"
	operationListThresholds        = "list threshold overrides"
	operationParseConfirmationTime = "parse pending confirmation %s"
	operationParseOverrideUpdated  = "parse threshold override updated_at"
	operationQueryConfirmation     = "query pending confirmation"
	operationUpsertThreshold       = "upsert threshold override"
	querySelectConfirmation        = `SELECT id, kind, payload_json, requested_by_user_id, expires_at, created_at FROM pending_confirmations WHERE id = ?`
	queryDeleteConfirmation        = `DELETE FROM pending_confirmations WHERE id = ?`
	queryInsertConfirmation        = `INSERT INTO pending_confirmations(id, kind, payload_json, requested_by_user_id, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	queryListThresholds            = `SELECT module_id, key, value, updated_by_user_id, updated_at FROM threshold_overrides ORDER BY module_id, key`
	queryUpsertThreshold           = `INSERT INTO threshold_overrides(module_id, key, value, updated_by_user_id, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(module_id, key) DO UPDATE SET
		 value = excluded.value,
		 updated_by_user_id = excluded.updated_by_user_id,
		 updated_at = excluded.updated_at`
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
