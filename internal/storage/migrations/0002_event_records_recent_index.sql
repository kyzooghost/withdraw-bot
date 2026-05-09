-- +goose Up
CREATE INDEX idx_event_records_created_at_id ON event_records(created_at DESC, id DESC);

-- +goose Down
DROP INDEX idx_event_records_created_at_id;
