-- +goose Up
CREATE TABLE monitor_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id TEXT NOT NULL,
    status TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    metrics_json TEXT NOT NULL,
    findings_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_monitor_snapshots_module_time ON monitor_snapshots(module_id, observed_at);

CREATE TABLE event_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    message TEXT NOT NULL,
    fields_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_event_records_type_time ON event_records(event_type, created_at);

CREATE TABLE threshold_overrides (
    module_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_by_user_id INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (module_id, key)
);

CREATE TABLE pending_confirmations (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    requested_by_user_id INTEGER NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE withdrawal_attempts (
    id TEXT PRIMARY KEY,
    trigger_kind TEXT NOT NULL,
    trigger_module_id TEXT NOT NULL,
    trigger_finding_key TEXT NOT NULL,
    status TEXT NOT NULL,
    tx_hash TEXT NOT NULL,
    nonce INTEGER,
    gas_units INTEGER,
    max_fee_per_gas_wei TEXT,
    max_priority_fee_per_gas_wei TEXT,
    expected_asset_units TEXT,
    simulation_success INTEGER NOT NULL,
    failure_reason TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE withdrawal_attempts;
DROP TABLE pending_confirmations;
DROP TABLE threshold_overrides;
DROP TABLE event_records;
DROP TABLE monitor_snapshots;
