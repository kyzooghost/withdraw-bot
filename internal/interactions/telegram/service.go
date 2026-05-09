package telegram

import (
	"context"
	"errors"
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
	defaultUpdateTimeoutSeconds     = 30
	securityRejectMessage           = "telegram command rejected"
	errTelegramBotRequiredMessage   = "telegram bot is required"
	sanitizedSecretValue            = "[REDACTED]"
	operationGetTelegramUpdates     = "get telegram updates"
	operationRecordSecurityReject   = "record telegram security reject"
	operationSendTelegramMessage    = "send telegram message"
	telegramCommandPrefix           = "/"
	thresholdSetSubcommand          = "set"
	logsInfoArg                     = "info"
	eventFieldChatID                = "chat_id"
	eventFieldUserID                = "user_id"
	eventFieldCommand               = "command"
	sanitizedUnknownCommand         = "unknown"
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
	responseLogsUsage               = "usage: /logs [info]"
	responseUnknownCommand          = "unknown command: %s"
)

var errTelegramBotRequired = errors.New(errTelegramBotRequiredMessage)

type ReportProvider interface {
	Stats(ctx context.Context) (string, error)
}

type WithdrawService interface {
	DryRunFullExit(ctx context.Context) (withdraw.WithdrawalResult, error)
}

type ThresholdService interface {
	List(ctx context.Context) (string, error)
	BuildSetConfirmation(ctx context.Context, userID int64, module string, key string, value string) (string, error)
	Confirm(ctx context.Context, userID int64, id string) (string, error)
}

type LogProvider interface {
	Recent(ctx context.Context, includeInfo bool) (string, error)
}

type UpdateSource interface {
	Updates(ctx context.Context) (<-chan tgbotapi.Update, error)
}

type MessageSender interface {
	SendText(ctx context.Context, chatID int64, text string) error
}

type Service struct {
	Bot              *tgbotapi.BotAPI
	UpdateSource     UpdateSource
	Sender           MessageSender
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
	if service.UpdateSource != nil {
		updates, err := service.UpdateSource.Updates(ctx)
		if err != nil {
			return err
		}
		return service.consumeUpdates(ctx, updates)
	}
	if service.Bot == nil {
		return errTelegramBotRequired
	}
	return service.pollBotUpdates(ctx)
}

func (service Service) consumeUpdates(ctx context.Context, updates <-chan tgbotapi.Update) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if err := service.handleUpdate(ctx, update); err != nil {
				return err
			}
		}
	}
}

func (service Service) SendAlert(ctx context.Context, msg interactions.AlertMessage) error {
	return service.sendText(ctx, service.Authorization.ChatID, msg.Text)
}

func (service Service) SendReport(ctx context.Context, msg interactions.ReportMessage) error {
	return service.sendText(ctx, service.Authorization.ChatID, msg.Text)
}

func (service Service) SendCommandResponse(ctx context.Context, msg interactions.CommandResponse) error {
	return service.sendText(ctx, msg.ChatID, msg.Text)
}

func (service Service) HandleCommand(ctx context.Context, chatID int64, userID int64, text string) (interactions.CommandResponse, error) {
	parsed := ParseCommand(text)
	if err := service.Authorization.Check(chatID, userID); err != nil {
		if recordErr := service.recordSecurityReject(ctx, parsed, chatID, userID); recordErr != nil {
			return interactions.CommandResponse{}, recordErr
		}
		return interactions.CommandResponse{}, err
	}
	response, err := service.dispatch(ctx, parsed, userID)
	if err != nil {
		return interactions.CommandResponse{}, err
	}
	return interactions.CommandResponse{ChatID: chatID, Text: service.boundResponse(response)}, nil
}

func (service Service) dispatch(ctx context.Context, command ParsedCommand, userID int64) (string, error) {
	switch command.Name {
	case string(core.CommandStats):
		return service.stats(ctx)
	case string(core.CommandWithdraw):
		return service.withdraw(ctx)
	case string(core.CommandConfirm):
		return service.confirm(ctx, command.Args, userID)
	case string(core.CommandThresholds):
		return service.thresholds(ctx)
	case string(core.CommandThresholdSet):
		return service.thresholdSet(ctx, command.Args, userID)
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

func (service Service) confirm(ctx context.Context, args []string, userID int64) (string, error) {
	if len(args) != 1 {
		return responseConfirmUsage, nil
	}
	if service.Thresholds == nil {
		return responseConfirmUnavailable, nil
	}
	return service.Thresholds.Confirm(ctx, userID, args[0])
}

func (service Service) thresholds(ctx context.Context) (string, error) {
	if service.Thresholds == nil {
		return responseThresholdsUnavailable, nil
	}
	return service.Thresholds.List(ctx)
}

func (service Service) thresholdSet(ctx context.Context, args []string, userID int64) (string, error) {
	if len(args) != 4 || args[0] != thresholdSetSubcommand {
		return responseThresholdSetUsage, nil
	}
	if service.Thresholds == nil {
		return responseThresholdSetUnavailable, nil
	}
	return service.Thresholds.BuildSetConfirmation(ctx, userID, args[1], args[2], args[3])
}

func (service Service) logs(ctx context.Context, args []string) (string, error) {
	if len(args) > 1 || len(args) == 1 && args[0] != logsInfoArg {
		return responseLogsUsage, nil
	}
	if service.Logs == nil {
		return responseLogsUnavailable, nil
	}
	includeInfo := len(args) == 1 && args[0] == logsInfoArg
	return service.Logs.Recent(ctx, includeInfo)
}

func (service Service) handleUpdate(ctx context.Context, update tgbotapi.Update) error {
	if update.Message == nil || update.Message.Chat == nil || update.Message.From == nil || !isCommandText(update.Message.Text) {
		return nil
	}
	response, err := service.HandleCommand(ctx, update.Message.Chat.ID, update.Message.From.ID, update.Message.Text)
	if err != nil {
		if errors.Is(err, errUnauthorizedTelegramCommand) {
			return nil
		}
		return err
	}
	if response.Text == "" {
		return nil
	}
	return service.SendCommandResponse(ctx, response)
}

func (service Service) pollBotUpdates(ctx context.Context) error {
	config := tgbotapi.NewUpdate(0)
	config.Timeout = defaultUpdateTimeoutSeconds
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		updates, err := service.Bot.GetUpdates(config)
		if err != nil {
			return sanitizeTelegramError(operationGetTelegramUpdates, service.Bot.Token, err)
		}
		for _, update := range updates {
			if update.UpdateID >= config.Offset {
				config.Offset = update.UpdateID + 1
			}
			if err := service.handleUpdate(ctx, update); err != nil {
				return err
			}
		}
	}
}

func (service Service) sendText(ctx context.Context, chatID int64, text string) error {
	boundedText := service.boundResponse(text)
	if service.Sender != nil {
		return service.Sender.SendText(ctx, chatID, boundedText)
	}
	if service.Bot == nil {
		return nil
	}
	message := tgbotapi.NewMessage(chatID, boundedText)
	_, err := service.Bot.Send(message)
	return sanitizeTelegramError(operationSendTelegramMessage, service.Bot.Token, err)
}

func (service Service) boundResponse(text string) string {
	maxChars := service.MaxResponseChars
	if maxChars <= 0 {
		maxChars = defaultMaxResponseChars
	}
	if len([]rune(text)) <= maxChars {
		return text
	}
	runes := []rune(text)
	if maxChars <= 3 {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-3]) + "..."
}

func (service Service) recordSecurityReject(ctx context.Context, command ParsedCommand, chatID int64, userID int64) error {
	if service.Events == nil {
		return nil
	}
	if err := service.Events.Record(ctx, core.EventSecurity, securityRejectMessage, map[string]string{
		eventFieldChatID:  fmt.Sprint(chatID),
		eventFieldUserID:  fmt.Sprint(userID),
		eventFieldCommand: sanitizedCommandName(command.Name),
	}, service.now()); err != nil {
		return fmt.Errorf("%s: %w", operationRecordSecurityReject, err)
	}
	return nil
}

func isCommandText(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), telegramCommandPrefix)
}

func sanitizedCommandName(name string) string {
	switch name {
	case string(core.CommandStats),
		string(core.CommandWithdraw),
		string(core.CommandConfirm),
		string(core.CommandThresholds),
		string(core.CommandThresholdSet),
		string(core.CommandLogs),
		string(core.CommandHelp):
		return name
	default:
		return sanitizedUnknownCommand
	}
}

func sanitizeTelegramError(operation string, token string, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if token != "" {
		message = strings.ReplaceAll(message, token, sanitizedSecretValue)
	}
	return fmt.Errorf("%s: %s", operation, message)
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
