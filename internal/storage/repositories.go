package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"withdraw-bot/internal/core"
)

type Repositories struct {
	DB *sql.DB
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
		result.ObservedAt.Format(time.RFC3339Nano),
		string(metrics),
		string(findings),
		createdAt.Format(time.RFC3339Nano),
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
		createdAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert event record: %w", err)
	}
	return nil
}
