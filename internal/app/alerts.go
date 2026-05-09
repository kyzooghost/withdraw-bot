package app

import (
	"context"

	"withdraw-bot/internal/core"
)

type AutoWithdrawer interface {
	HandleUrgent(ctx context.Context, result core.MonitorResult, finding core.Finding) error
}

type Notifier interface {
	SendAlert(ctx context.Context, text string) error
}

type AlertService struct {
	Withdrawer AutoWithdrawer
	Notifier   Notifier
	Clock      core.Clock
}

func (service AlertService) HandleMonitorResults(ctx context.Context, results []core.MonitorResult) error {
	for _, result := range results {
		for _, finding := range result.Findings {
			if finding.Severity != core.SeverityUrgent {
				continue
			}
			if err := service.Notifier.SendAlert(ctx, finding.Message); err != nil {
				return err
			}
			if err := service.Withdrawer.HandleUrgent(ctx, result, finding); err != nil {
				return err
			}
		}
	}
	return nil
}
