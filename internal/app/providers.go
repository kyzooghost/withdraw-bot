package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"withdraw-bot/internal/config"
	"withdraw-bot/internal/core"
	telegramcmd "withdraw-bot/internal/interactions/telegram"
	"withdraw-bot/internal/storage"
)

const (
	defaultRecentEventLimit             = 10
	defaultThresholdConfirmationTTL     = 10 * time.Minute
	thresholdConfirmationKind           = "threshold"
	thresholdConfirmationResponseFormat = "%s\nconfirm with %s %s"
	thresholdAppliedResponseFormat      = "threshold override applied %s %s=%s"
	thresholdOverridesEmptyResponse     = "no threshold overrides"
	thresholdOverridesHeader            = "threshold overrides:"
	thresholdOverrideLineFormat         = "%s %s=%s by %d at %s"
	thresholdStaticEmptyResponse        = "no static thresholds"
	thresholdStaticHeader               = "static thresholds:"
	thresholdStaticLineFormat           = "%s %s=%v"
	errPendingConfirmationNotThreshold  = "pending confirmation %q is not a threshold change"
	eventLogsEmptyResponse              = "no recent events"
	eventLogLineFormat                  = "%s %s %s"
	eventLogLineWithFieldsFormat        = "%s %s"
	eventLogLineSeparator               = "\n"
	eventFieldSeparator                 = " "
	eventFieldKeyValueSeparator         = "="
	eventMessageThresholdOverride       = "threshold override applied"
	eventFieldModuleID                  = "module_id"
	eventFieldThresholdKey              = "key"
	eventFieldThresholdValue            = "value"
	eventFieldUserID                    = "user_id"
)

type eventLogProvider struct {
	repos storage.Repositories
	limit int
}

func (provider eventLogProvider) Recent(ctx context.Context, includeInfo bool) (string, error) {
	limit := provider.limit
	if limit <= 0 {
		limit = defaultRecentEventLimit
	}
	events, err := provider.repos.ListRecentEvents(ctx, includeInfo, limit)
	if err != nil {
		return "", err
	}
	if len(events) == 0 {
		return eventLogsEmptyResponse, nil
	}
	lines := make([]string, 0, len(events))
	for _, event := range events {
		lines = append(lines, formatEventRecord(event))
	}
	return strings.Join(lines, eventLogLineSeparator), nil
}

type thresholdProvider struct {
	repos         storage.Repositories
	config        config.Config
	clock         core.Clock
	ttl           time.Duration
	assetDecimals uint8
}

func (provider thresholdProvider) List(ctx context.Context) (string, error) {
	overrides, err := provider.repos.ListThresholdOverrides(ctx)
	if err != nil {
		return "", err
	}
	lines := []string{thresholdStaticHeader}
	staticLines := staticThresholdLines(provider.config)
	if len(staticLines) == 0 {
		lines = append(lines, thresholdStaticEmptyResponse)
	} else {
		lines = append(lines, staticLines...)
	}
	lines = append(lines, thresholdOverridesHeader)
	if len(overrides) == 0 {
		lines = append(lines, thresholdOverridesEmptyResponse)
	} else {
		for _, override := range overrides {
			lines = append(lines, fmt.Sprintf(
				thresholdOverrideLineFormat,
				override.ModuleID,
				override.Key,
				override.Value,
				override.UpdatedByUserID,
				override.UpdatedAt.UTC().Format(time.RFC3339Nano),
			))
		}
	}
	return strings.Join(lines, eventLogLineSeparator), nil
}

func (provider thresholdProvider) BuildSetConfirmation(ctx context.Context, userID int64, module string, key string, value string) (string, error) {
	confirmation, err := telegramcmd.BuildThresholdConfirmation(telegramcmd.ThresholdSetRequest{
		ModuleID: module,
		Key:      key,
		Value:    value,
		UserID:   userID,
	})
	if err != nil {
		return "", err
	}
	if err := validateThresholdValue(confirmation.Request.ModuleID, confirmation.Request.Key, confirmation.Request.Value, provider.assetDecimals); err != nil {
		return "", err
	}
	payload, err := json.Marshal(confirmation.Request)
	if err != nil {
		return "", err
	}
	now := provider.now()
	ttl := provider.ttl
	if ttl <= 0 {
		ttl = defaultThresholdConfirmationTTL
	}
	if err := provider.repos.InsertPendingConfirmation(ctx, storage.PendingConfirmation{
		ID:                confirmation.ID,
		Kind:              thresholdConfirmationKind,
		PayloadJSON:       string(payload),
		RequestedByUserID: userID,
		ExpiresAt:         now.Add(ttl),
		CreatedAt:         now,
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf(thresholdConfirmationResponseFormat, confirmation.Message, core.CommandConfirm, confirmation.ID), nil
}

func (provider thresholdProvider) Confirm(ctx context.Context, userID int64, id string) (string, error) {
	now := provider.now()
	confirmation, err := provider.repos.ConsumePendingConfirmationForUser(ctx, id, userID, now)
	if err != nil {
		return "", err
	}
	if confirmation.Kind != thresholdConfirmationKind {
		return "", fmt.Errorf(errPendingConfirmationNotThreshold, id)
	}
	var request telegramcmd.ThresholdSetRequest
	if err := json.Unmarshal([]byte(confirmation.PayloadJSON), &request); err != nil {
		return "", err
	}
	if _, err := telegramcmd.BuildThresholdConfirmation(request); err != nil {
		return "", err
	}
	if err := validateThresholdValue(request.ModuleID, request.Key, request.Value, provider.assetDecimals); err != nil {
		return "", err
	}
	if err := provider.repos.UpsertThresholdOverride(ctx, request.ModuleID, request.Key, request.Value, confirmation.RequestedByUserID, now); err != nil {
		return "", err
	}
	if err := provider.repos.Record(ctx, core.EventSecurity, eventMessageThresholdOverride, map[string]string{
		eventFieldModuleID:       request.ModuleID,
		eventFieldThresholdKey:   request.Key,
		eventFieldThresholdValue: request.Value,
		eventFieldUserID:         fmt.Sprint(userID),
	}, now); err != nil {
		return "", err
	}
	return fmt.Sprintf(thresholdAppliedResponseFormat, request.ModuleID, request.Key, request.Value), nil
}

func (provider thresholdProvider) now() time.Time {
	if provider.clock == nil {
		return core.SystemClock{}.Now()
	}
	return provider.clock.Now()
}

func staticThresholdLines(cfg config.Config) []string {
	keysByModule := map[core.MonitorModuleID][]string{
		core.ModuleSharePriceLoss: {
			moduleConfigKeyLossWarnBPS,
			moduleConfigKeyLossUrgentBPS,
			moduleConfigKeyStaleUrgentAfter,
		},
		core.ModuleWithdrawLiquidity: {
			moduleConfigKeyIdleWarnThresholdUSDC,
			moduleConfigKeyIdleUrgentThresholdUSDC,
			moduleConfigKeyStaleUrgentAfter,
		},
		core.ModuleVaultState: {
			moduleConfigKeyChangeSeverity,
			moduleConfigKeyStaleUrgentAfter,
		},
	}
	moduleIDs := []core.MonitorModuleID{
		core.ModuleSharePriceLoss,
		core.ModuleWithdrawLiquidity,
		core.ModuleVaultState,
	}
	lines := []string{}
	for _, moduleID := range moduleIDs {
		moduleConfig, ok := cfg.Modules[string(moduleID)]
		if !ok {
			continue
		}
		for _, key := range keysByModule[moduleID] {
			value, ok := moduleConfig[key]
			if !ok {
				continue
			}
			lines = append(lines, fmt.Sprintf(thresholdStaticLineFormat, moduleID, key, value))
		}
	}
	return lines
}

func formatEventRecord(event storage.EventRecord) string {
	line := fmt.Sprintf(eventLogLineFormat, event.CreatedAt.UTC().Format(time.RFC3339Nano), event.EventType, event.Message)
	fields := formatEventFields(event.Fields)
	if fields == "" {
		return line
	}
	return fmt.Sprintf(eventLogLineWithFieldsFormat, line, fields)
}

func formatEventFields(fields map[string]string) string {
	if len(fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+eventFieldKeyValueSeparator+fields[key])
	}
	return strings.Join(parts, eventFieldSeparator)
}
