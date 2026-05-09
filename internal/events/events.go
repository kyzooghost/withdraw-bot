package events

import (
	"context"
	"time"

	"withdraw-bot/internal/core"
)

type Recorder interface {
	Record(ctx context.Context, eventType core.EventType, message string, fields map[string]string, at time.Time) error
}
