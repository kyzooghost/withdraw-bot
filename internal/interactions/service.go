package interactions

import "context"

type AlertMessage struct {
	Text string
}

type ReportMessage struct {
	Text string
}

type CommandResponse struct {
	ChatID int64
	Text   string
}

type Service interface {
	Start(ctx context.Context) error
	SendAlert(ctx context.Context, msg AlertMessage) error
	SendReport(ctx context.Context, msg ReportMessage) error
	SendCommandResponse(ctx context.Context, msg CommandResponse) error
}
