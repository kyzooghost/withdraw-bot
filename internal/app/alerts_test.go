package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"withdraw-bot/internal/core"
)

func TestHandleMonitorResultsTriggersWithdrawForUrgentFinding(t *testing.T) {
	// Arrange
	withdrawer := &fakeWithdrawer{}
	notifier := &fakeNotifier{}
	service := AlertService{Withdrawer: withdrawer, Notifier: notifier, Clock: core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)}}
	results := []core.MonitorResult{{
		ModuleID: core.ModuleWithdrawLiquidity,
		Status:   core.MonitorStatusUrgent,
		Findings: []core.Finding{{Key: core.FindingIdleLiquidity, Severity: core.SeverityUrgent, Message: "idle liquidity urgent"}},
	}}

	// Act
	err := service.HandleMonitorResults(context.Background(), results)

	// Assert
	if err != nil {
		t.Fatalf("handle monitor results: %v", err)
	}
	if withdrawer.calls != 1 {
		t.Fatalf("expected one withdraw call, got %d", withdrawer.calls)
	}
	if notifier.alerts != 1 {
		t.Fatalf("expected one alert, got %d", notifier.alerts)
	}
}

func TestHandleMonitorResultsIgnoresNonUrgentFinding(t *testing.T) {
	// Arrange
	withdrawer := &fakeWithdrawer{}
	notifier := &fakeNotifier{}
	service := AlertService{Withdrawer: withdrawer, Notifier: notifier}
	results := []core.MonitorResult{{
		ModuleID: core.ModuleWithdrawLiquidity,
		Status:   core.MonitorStatusWarn,
		Findings: []core.Finding{{Key: core.FindingIdleLiquidity, Severity: core.SeverityWarn, Message: "idle liquidity warning"}},
	}}

	// Act
	err := service.HandleMonitorResults(context.Background(), results)

	// Assert
	if err != nil {
		t.Fatalf("handle monitor results: %v", err)
	}
	if withdrawer.calls != 0 {
		t.Fatalf("expected no withdraw calls, got %d", withdrawer.calls)
	}
	if notifier.alerts != 0 {
		t.Fatalf("expected no alerts, got %d", notifier.alerts)
	}
}

func TestHandleMonitorResultsReturnsNotifierError(t *testing.T) {
	// Arrange
	expectedErr := errors.New("notify failed")
	withdrawer := &fakeWithdrawer{}
	notifier := &fakeNotifier{err: expectedErr}
	service := AlertService{Withdrawer: withdrawer, Notifier: notifier}
	results := []core.MonitorResult{{
		ModuleID: core.ModuleWithdrawLiquidity,
		Status:   core.MonitorStatusUrgent,
		Findings: []core.Finding{{Key: core.FindingIdleLiquidity, Severity: core.SeverityUrgent, Message: "idle liquidity urgent"}},
	}}

	// Act
	err := service.HandleMonitorResults(context.Background(), results)

	// Assert
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected notifier error, got %v", err)
	}
	if withdrawer.calls != 0 {
		t.Fatalf("expected withdraw not to run after notifier failure, got %d calls", withdrawer.calls)
	}
}

type fakeWithdrawer struct {
	calls int
}

func (withdrawer *fakeWithdrawer) HandleUrgent(ctx context.Context, result core.MonitorResult, finding core.Finding) error {
	withdrawer.calls++
	return nil
}

type fakeNotifier struct {
	alerts int
	err    error
}

func (notifier *fakeNotifier) SendAlert(ctx context.Context, text string) error {
	notifier.alerts++
	return notifier.err
}
