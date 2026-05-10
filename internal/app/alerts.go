package app

import (
	"context"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/interactions"
	"withdraw-bot/internal/withdraw"
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

type alertSender interface {
	SendAlert(ctx context.Context, msg interactions.AlertMessage) error
}

type telegramAlertNotifier struct {
	sender alertSender
}

func (notifier telegramAlertNotifier) SendAlert(ctx context.Context, text string) error {
	return notifier.sender.SendAlert(ctx, interactions.AlertMessage{Text: text})
}

type withdrawalExecutor interface {
	ExecuteFullExit(ctx context.Context, trigger withdraw.WithdrawalTrigger) (withdraw.WithdrawalResult, error)
}

type urgentWithdrawer struct {
	executor withdrawalExecutor
}

func (withdrawer urgentWithdrawer) HandleUrgent(ctx context.Context, result core.MonitorResult, finding core.Finding) error {
	_, err := withdrawer.executor.ExecuteFullExit(ctx, withdraw.WithdrawalTrigger{
		Kind:       withdraw.TriggerKindUrgent,
		ModuleID:   result.ModuleID,
		FindingKey: finding.Key,
	})
	return err
}
