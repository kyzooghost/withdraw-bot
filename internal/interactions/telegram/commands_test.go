package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/withdraw"
)

func TestAuthorizeRejectsUnknownUser(t *testing.T) {
	// Arrange
	auth := Authorization{ChatID: 100, AllowedUserIDs: map[int64]bool{1: true}}

	// Act
	err := auth.Check(100, 2)

	// Assert
	if err == nil {
		t.Fatalf("expected unknown user to be rejected")
	}
}

func TestParseCommandTrimsTextAndSplitsArgs(t *testing.T) {
	// Arrange
	text := "  /threshold set withdraw_liquidity idle_urgent_threshold_usdc 500000  "

	// Act
	result := ParseCommand(text)

	// Assert
	if result.Name != string(core.CommandThresholdSet) {
		t.Fatalf("expected command %q, got %q", core.CommandThresholdSet, result.Name)
	}
	expectedArgs := []string{"set", "withdraw_liquidity", "idle_urgent_threshold_usdc", "500000"}
	if strings.Join(result.Args, ",") != strings.Join(expectedArgs, ",") {
		t.Fatalf("expected args %v, got %v", expectedArgs, result.Args)
	}
}

func TestHandleCommandReturnsStatsResponse(t *testing.T) {
	// Arrange
	ctx := context.Background()
	service := Service{
		Authorization: Authorization{ChatID: 100, AllowedUserIDs: map[int64]bool{1: true}},
		Reports:       fakeReportProvider{stats: "Status: OK"},
		Clock:         core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	result, err := service.HandleCommand(ctx, 100, 1, string(core.CommandStats))

	// Assert
	if err != nil {
		t.Fatalf("handle command: %v", err)
	}
	if result.Text != "Status: OK" {
		t.Fatalf("expected stats response, got %q", result.Text)
	}
}

func TestHandleCommandRecordsSecurityEventForRejectedUser(t *testing.T) {
	// Arrange
	ctx := context.Background()
	recorder := &fakeEventRecorder{}
	service := Service{
		Authorization: Authorization{ChatID: 100, AllowedUserIDs: map[int64]bool{1: true}},
		Events:        recorder,
		Clock:         core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	_, err := service.HandleCommand(ctx, 100, 2, string(core.CommandStats))

	// Assert
	if err == nil {
		t.Fatal("expected authorization error")
	}
	if len(recorder.events) != 1 {
		t.Fatalf("expected one security event, got %d", len(recorder.events))
	}
	if recorder.events[0].eventType != core.EventSecurity {
		t.Fatalf("expected security event, got %s", recorder.events[0].eventType)
	}
	if _, ok := recorder.events[0].fields["user_id"]; !ok {
		t.Fatal("expected sanitized user id field")
	}
}

func TestHandleCommandBoundsLongResponse(t *testing.T) {
	// Arrange
	ctx := context.Background()
	service := Service{
		Authorization:    Authorization{ChatID: 100, AllowedUserIDs: map[int64]bool{1: true}},
		Reports:          fakeReportProvider{stats: strings.Repeat("x", 20)},
		MaxResponseChars: 10,
	}

	// Act
	result, err := service.HandleCommand(ctx, 100, 1, string(core.CommandStats))

	// Assert
	if err != nil {
		t.Fatalf("handle command: %v", err)
	}
	if len(result.Text) > 10 {
		t.Fatalf("expected bounded response, got %d chars", len(result.Text))
	}
}

func TestHandleCommandReturnsWithdrawDryRunResponse(t *testing.T) {
	// Arrange
	ctx := context.Background()
	service := Service{
		Authorization: Authorization{ChatID: 100, AllowedUserIDs: map[int64]bool{1: true}},
		Withdraw:      fakeWithdrawService{result: withdraw.WithdrawalResult{Status: withdraw.WithdrawalStatusSimulated}},
	}

	// Act
	result, err := service.HandleCommand(ctx, 100, 1, string(core.CommandWithdraw))

	// Assert
	if err != nil {
		t.Fatalf("handle command: %v", err)
	}
	if result.Text != "withdraw dry run: simulated" {
		t.Fatalf("expected withdraw dry run response, got %q", result.Text)
	}
}

func TestHandleCommandReturnsThresholdConfirmationResponse(t *testing.T) {
	// Arrange
	ctx := context.Background()
	service := Service{
		Authorization: Authorization{ChatID: 100, AllowedUserIDs: map[int64]bool{1: true}},
		Thresholds:    fakeThresholdService{confirmation: "confirm threshold change: abc"},
	}

	// Act
	result, err := service.HandleCommand(ctx, 100, 1, "/threshold set withdraw_liquidity idle_urgent_threshold_usdc 500000")

	// Assert
	if err != nil {
		t.Fatalf("handle command: %v", err)
	}
	if result.Text != "confirm threshold change: abc" {
		t.Fatalf("expected threshold confirmation response, got %q", result.Text)
	}
}

func TestHandleCommandReturnsLogsInfoResponse(t *testing.T) {
	// Arrange
	ctx := context.Background()
	service := Service{
		Authorization: Authorization{ChatID: 100, AllowedUserIDs: map[int64]bool{1: true}},
		Logs:          fakeLogProvider{info: "info events"},
	}

	// Act
	result, err := service.HandleCommand(ctx, 100, 1, "/logs info")

	// Assert
	if err != nil {
		t.Fatalf("handle command: %v", err)
	}
	if result.Text != "info events" {
		t.Fatalf("expected info logs, got %q", result.Text)
	}
}

func TestHandleCommandReturnsHelpResponse(t *testing.T) {
	// Arrange
	ctx := context.Background()
	service := Service{
		Authorization: Authorization{ChatID: 100, AllowedUserIDs: map[int64]bool{1: true}},
	}

	// Act
	result, err := service.HandleCommand(ctx, 100, 1, string(core.CommandHelp))

	// Assert
	if err != nil {
		t.Fatalf("handle command: %v", err)
	}
	if !strings.Contains(result.Text, string(core.CommandStats)) {
		t.Fatalf("expected help response to include %s, got %q", core.CommandStats, result.Text)
	}
}

type fakeReportProvider struct {
	stats string
}

func (provider fakeReportProvider) Stats(ctx context.Context) (string, error) {
	return provider.stats, nil
}

type fakeWithdrawService struct {
	result withdraw.WithdrawalResult
}

func (service fakeWithdrawService) DryRunFullExit(ctx context.Context) (withdraw.WithdrawalResult, error) {
	return service.result, nil
}

type fakeThresholdService struct {
	list         string
	confirmation string
	confirmed    string
}

func (service fakeThresholdService) List(ctx context.Context) (string, error) {
	return service.list, nil
}

func (service fakeThresholdService) BuildSetConfirmation(ctx context.Context, module string, key string, value string) (string, error) {
	return service.confirmation, nil
}

func (service fakeThresholdService) Confirm(ctx context.Context, id string) (string, error) {
	return service.confirmed, nil
}

type fakeLogProvider struct {
	warnings string
	info     string
}

func (provider fakeLogProvider) Recent(ctx context.Context, includeInfo bool) (string, error) {
	if includeInfo {
		return provider.info, nil
	}
	return provider.warnings, nil
}

type recordedEvent struct {
	eventType core.EventType
	message   string
	fields    map[string]string
	at        time.Time
}

type fakeEventRecorder struct {
	events []recordedEvent
}

func (recorder *fakeEventRecorder) Record(ctx context.Context, eventType core.EventType, message string, fields map[string]string, at time.Time) error {
	recorder.events = append(recorder.events, recordedEvent{eventType: eventType, message: message, fields: fields, at: at})
	return nil
}
