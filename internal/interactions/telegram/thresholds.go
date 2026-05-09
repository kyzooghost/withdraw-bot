package telegram

import (
	"fmt"

	"withdraw-bot/internal/core"
)

const (
	thresholdKeyLossWarnBPS             = "loss_warn_bps"
	thresholdKeyLossUrgentBPS           = "loss_urgent_bps"
	thresholdKeyStaleUrgentAfter        = "stale_urgent_after"
	thresholdKeyIdleWarnThresholdUSDC   = "idle_warn_threshold_usdc"
	thresholdKeyIdleUrgentThresholdUSDC = "idle_urgent_threshold_usdc"
	thresholdKeyChangeSeverity          = "change_severity"
	thresholdConfirmationIDFormat       = "threshold:%s:%s:%d"
	thresholdConfirmationMessageFormat  = "Confirm threshold override %s %s=%s"
	errUnknownThresholdModule           = "unknown module %q"
	errUnknownThresholdKey              = "threshold key %q is not allowed for module %q"
)

type ThresholdSetRequest struct {
	ModuleID string
	Key      string
	Value    string
	UserID   int64
}

type ThresholdConfirmation struct {
	ID      string
	Request ThresholdSetRequest
	Message string
}

var allowedThresholdKeys = map[core.MonitorModuleID]map[string]bool{
	core.ModuleSharePriceLoss: {
		thresholdKeyLossWarnBPS:      true,
		thresholdKeyLossUrgentBPS:    true,
		thresholdKeyStaleUrgentAfter: true,
	},
	core.ModuleWithdrawLiquidity: {
		thresholdKeyIdleWarnThresholdUSDC:   true,
		thresholdKeyIdleUrgentThresholdUSDC: true,
		thresholdKeyStaleUrgentAfter:        true,
	},
	core.ModuleVaultState: {
		thresholdKeyChangeSeverity:   true,
		thresholdKeyStaleUrgentAfter: true,
	},
}

func BuildThresholdConfirmation(request ThresholdSetRequest) (ThresholdConfirmation, error) {
	moduleID := core.MonitorModuleID(request.ModuleID)
	keys, ok := allowedThresholdKeys[moduleID]
	if !ok {
		return ThresholdConfirmation{}, fmt.Errorf(errUnknownThresholdModule, request.ModuleID)
	}
	if !keys[request.Key] {
		return ThresholdConfirmation{}, fmt.Errorf(errUnknownThresholdKey, request.Key, request.ModuleID)
	}
	id := fmt.Sprintf(thresholdConfirmationIDFormat, request.ModuleID, request.Key, request.UserID)
	return ThresholdConfirmation{
		ID:      id,
		Request: request,
		Message: fmt.Sprintf(thresholdConfirmationMessageFormat, request.ModuleID, request.Key, request.Value),
	}, nil
}
