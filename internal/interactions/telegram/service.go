package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/events"
	"withdraw-bot/internal/interactions"
	"withdraw-bot/internal/withdraw"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	defaultMaxResponseChars         = 3500
	securityRejectMessage           = "telegram command rejected"
	thresholdSetSubcommand          = "set"
	logsInfoArg                     = "info"
	eventFieldChatID                = "chat_id"
	eventFieldUserID                = "user_id"
	eventFieldCommand               = "command"
	responseStatsUnavailable        = "stats unavailable"
	responseWithdrawUnavailable     = "withdraw unavailable"
	responseWithdrawNoop            = "withdraw dry run: no shares to withdraw"
	responseWithdrawDryRun          = "withdraw dry run: %s"
	responseConfirmUsage            = "usage: /confirm <id>"
	responseConfirmUnavailable      = "confirm unavailable"
	responseThresholdsUnavailable   = "thresholds unavailable"
	responseThresholdSetUsage       = "usage: /threshold set <module> <key> <value>"
	responseThresholdSetUnavailable = "threshold updates unavailable"
	responseLogsUnavailable         = "logs unavailable"
	responseUnknownCommand          = "unknown command: %s"
)

type ReportProvider interface {
	Stats(ctx context.Context) (string, error)
}

type WithdrawService interface {
	DryRunFullExit(ctx context.Context) (withdraw.WithdrawalResult, error)
}

type ThresholdService interface {
	List(ctx context.Context) (string, error)
	BuildSetConfirmation(ctx context.Context, module string, key string, value string) (string, error)
	Confirm(ctx context.Context, id string) (string, error)
}

type LogProvider interface {
	Recent(ctx context.Context, includeInfo bool) (string, error)
}

type Service struct {
	Bot              *tgbotapi.BotAPI
	Authorization    Authorization
	Reports          ReportProvider
	Withdraw         WithdrawService
	Thresholds       ThresholdService
	Logs             LogProvider
	Events           events.Recorder
	Clock            core.Clock
	MaxResponseChars int
}

func (service Service) Start(ctx context.Context) error {
	return nil
}

func (service Service) SendAlert(ctx context.Context, msg interactions.AlertMessage) error {
	return service.sendText(service.Authorization.ChatID, msg.Text)
}

func (service Service) SendReport(ctx context.Context, msg interactions.ReportMessage) error {
	return service.sendText(service.Authorization.ChatID, msg.Text)
}

func (service Service) SendCommandResponse(ctx context.Context, msg interactions.CommandResponse) error {
	return service.sendText(msg.ChatID, msg.Text)
}

func (service Service) HandleCommand(ctx context.Context, chatID int64, userID int64, text string) (interactions.CommandResponse, error) {
	parsed := ParseCommand(text)
	if err := service.Authorization.Check(chatID, userID); err != nil {
		service.recordSecurityReject(ctx, parsed, chatID, userID)
		return interactions.CommandResponse{}, err
	}
	response, err := service.dispatch(ctx, parsed)
	if err != nil {
		return interactions.CommandResponse{}, err
	}
	return interactions.CommandResponse{ChatID: chatID, Text: service.boundResponse(response)}, nil
}

func (service Service) dispatch(ctx context.Context, command ParsedCommand) (string, error) {
	switch command.Name {
	case string(core.CommandStats):
		return service.stats(ctx)
	case string(core.CommandWithdraw):
		return service.withdraw(ctx)
	case string(core.CommandConfirm):
		return service.confirm(ctx, command.Args)
	case string(core.CommandThresholds):
		return service.thresholds(ctx)
	case string(core.CommandThresholdSet):
		return service.thresholdSet(ctx, command.Args)
	case string(core.CommandLogs):
		return service.logs(ctx, command.Args)
	case string(core.CommandHelp), "":
		return helpText(), nil
	default:
		return fmt.Sprintf(responseUnknownCommand, command.Name), nil
	}
}

func (service Service) stats(ctx context.Context) (string, error) {
	if service.Reports == nil {
		return responseStatsUnavailable, nil
	}
	return service.Reports.Stats(ctx)
}

func (service Service) withdraw(ctx context.Context) (string, error) {
	if service.Withdraw == nil {
		return responseWithdrawUnavailable, nil
	}
	result, err := service.Withdraw.DryRunFullExit(ctx)
	if err != nil {
		return "", err
	}
	if result.Noop {
		return responseWithdrawNoop, nil
	}
	return fmt.Sprintf(responseWithdrawDryRun, result.Status), nil
}

func (service Service) confirm(ctx context.Context, args []string) (string, error) {
	if len(args) != 1 {
		return responseConfirmUsage, nil
	}
	if service.Thresholds == nil {
		return responseConfirmUnavailable, nil
	}
	return service.Thresholds.Confirm(ctx, args[0])
}

func (service Service) thresholds(ctx context.Context) (string, error) {
	if service.Thresholds == nil {
		return responseThresholdsUnavailable, nil
	}
	return service.Thresholds.List(ctx)
}

func (service Service) thresholdSet(ctx context.Context, args []string) (string, error) {
	if len(args) != 4 || args[0] != thresholdSetSubcommand {
		return responseThresholdSetUsage, nil
	}
	if service.Thresholds == nil {
		return responseThresholdSetUnavailable, nil
	}
	return service.Thresholds.BuildSetConfirmation(ctx, args[1], args[2], args[3])
}

func (service Service) logs(ctx context.Context, args []string) (string, error) {
	if service.Logs == nil {
		return responseLogsUnavailable, nil
	}
	includeInfo := len(args) == 1 && args[0] == logsInfoArg
	return service.Logs.Recent(ctx, includeInfo)
}

func (service Service) sendText(chatID int64, text string) error {
	if service.Bot == nil {
		return nil
	}
	message := tgbotapi.NewMessage(chatID, service.boundResponse(text))
	_, err := service.Bot.Send(message)
	return err
}

func (service Service) boundResponse(text string) string {
	maxChars := service.MaxResponseChars
	if maxChars <= 0 {
		maxChars = defaultMaxResponseChars
	}
	if len(text) <= maxChars {
		return text
	}
	if maxChars <= 3 {
		return text[:maxChars]
	}
	return text[:maxChars-3] + "..."
}

func (service Service) recordSecurityReject(ctx context.Context, command ParsedCommand, chatID int64, userID int64) {
	if service.Events == nil {
		return
	}
	_ = service.Events.Record(ctx, core.EventSecurity, securityRejectMessage, map[string]string{
		eventFieldChatID:  fmt.Sprint(chatID),
		eventFieldUserID:  fmt.Sprint(userID),
		eventFieldCommand: command.Name,
	}, service.now())
}

func (service Service) now() time.Time {
	if service.Clock == nil {
		return core.SystemClock{}.Now()
	}
	return service.Clock.Now()
}

func helpText() string {
	return strings.Join([]string{
		string(core.CommandStats),
		string(core.CommandWithdraw),
		string(core.CommandConfirm) + " <id>",
		string(core.CommandThresholds),
		string(core.CommandThresholdSet) + " " + thresholdSetSubcommand + " <module> <key> <value>",
		string(core.CommandLogs),
		string(core.CommandLogs) + " " + logsInfoArg,
		string(core.CommandHelp),
	}, "\n")
}
