package app

import (
	"context"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/monitor"
	"withdraw-bot/internal/storage"
)

const (
	moduleConfigKeyEnabled                 = "enabled"
	moduleConfigKeyLossWarnBPS             = "loss_warn_bps"
	moduleConfigKeyLossUrgentBPS           = "loss_urgent_bps"
	moduleConfigKeyIdleWarnThresholdUSDC   = "idle_warn_threshold_usdc"
	moduleConfigKeyIdleUrgentThresholdUSDC = "idle_urgent_threshold_usdc"
	moduleConfigKeyChangeSeverity          = "change_severity"
	moduleConfigKeyStaleUrgentAfter        = "stale_urgent_after"
)

type thresholdOverrideModule struct {
	module        monitor.Module
	repos         storage.Repositories
	assetDecimals uint8
	factory       ModuleFactory
}

func withThresholdOverrides(modules []monitor.Module, repos storage.Repositories, assetDecimals uint8, factory ModuleFactory) []monitor.Module {
	result := make([]monitor.Module, len(modules))
	for index, module := range modules {
		if module == nil {
			continue
		}
		result[index] = thresholdOverrideModule{module: module, repos: repos, assetDecimals: assetDecimals, factory: factory}
	}
	return result
}

func (module thresholdOverrideModule) ID() core.MonitorModuleID {
	return module.module.ID()
}

func (module thresholdOverrideModule) ValidateConfig(ctx context.Context) error {
	effective, err := module.effective(ctx)
	if err != nil {
		return err
	}
	return effective.ValidateConfig(ctx)
}

func (module thresholdOverrideModule) Bootstrap(ctx context.Context) (map[string]any, error) {
	return module.module.Bootstrap(ctx)
}

func (module thresholdOverrideModule) Monitor(ctx context.Context) (core.MonitorResult, error) {
	effective, err := module.effective(ctx)
	if err != nil {
		return core.MonitorResult{}, err
	}
	return effective.Monitor(ctx)
}

func (module thresholdOverrideModule) effective(ctx context.Context) (monitor.Module, error) {
	overrides, err := module.repos.ListThresholdOverrides(ctx)
	if err != nil {
		return nil, err
	}
	return module.factory.ApplyOverrides(module.module, toThresholdOverrides(overrides), module.assetDecimals)
}

func toThresholdOverrides(overrides []storage.ThresholdOverride) []core.ThresholdOverride {
	result := make([]core.ThresholdOverride, len(overrides))
	for i, o := range overrides {
		result[i] = core.ThresholdOverride{ModuleID: o.ModuleID, Key: o.Key, Value: o.Value}
	}
	return result
}

type telegramThresholdRequest struct {
	ModuleID string
	Key      string
	Value    string
}

func upsertEffectiveOverride(overrides []storage.ThresholdOverride, request telegramThresholdRequest) []storage.ThresholdOverride {
	result := append([]storage.ThresholdOverride{}, overrides...)
	for index, override := range result {
		if override.ModuleID == request.ModuleID && override.Key == request.Key {
			result[index].Value = request.Value
			return result
		}
	}
	return append(result, storage.ThresholdOverride{ModuleID: request.ModuleID, Key: request.Key, Value: request.Value})
}
