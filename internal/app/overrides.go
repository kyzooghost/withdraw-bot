package app

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"withdraw-bot/internal/config"
	"withdraw-bot/internal/core"
	"withdraw-bot/internal/monitor"
	morphomod "withdraw-bot/internal/monitor/modules/morpho"
	"withdraw-bot/internal/storage"
)

const (
	errThresholdBPSInteger = "%s.%s must be an integer"
	errThresholdBPSRange   = "%s.%s must be between 1 and 10000 bps"
	errThresholdDuration   = "%s.%s must be a positive duration"
	errThresholdSeverity   = "%s.%s must be warn or urgent"
	errThresholdAssetUnits = "%s.%s must be positive"
)

type thresholdOverrideModule struct {
	module        monitor.Module
	repos         storage.Repositories
	assetDecimals uint8
}

func withThresholdOverrides(modules []monitor.Module, repos storage.Repositories, assetDecimals uint8) []monitor.Module {
	result := make([]monitor.Module, len(modules))
	for index, module := range modules {
		if module == nil {
			continue
		}
		result[index] = thresholdOverrideModule{module: module, repos: repos, assetDecimals: assetDecimals}
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
	return applyThresholdOverrides(module.module, overrides, module.assetDecimals)
}

func applyThresholdOverrides(module monitor.Module, overrides []storage.ThresholdOverride, assetDecimals uint8) (monitor.Module, error) {
	switch typed := module.(type) {
	case morphomod.SharePriceModule:
		return applySharePriceOverrides(typed, overrides)
	case *morphomod.SharePriceModule:
		if typed == nil {
			return module, nil
		}
		copy := *typed
		return applySharePriceOverrides(copy, overrides)
	case morphomod.WithdrawLiquidityModule:
		return applyWithdrawLiquidityOverrides(typed, overrides, assetDecimals)
	case *morphomod.WithdrawLiquidityModule:
		if typed == nil {
			return module, nil
		}
		copy := *typed
		return applyWithdrawLiquidityOverrides(copy, overrides, assetDecimals)
	case morphomod.VaultStateModule:
		return applyVaultStateOverrides(typed, overrides)
	case *morphomod.VaultStateModule:
		if typed == nil {
			return module, nil
		}
		copy := *typed
		return applyVaultStateOverrides(copy, overrides)
	default:
		return module, nil
	}
}

func applySharePriceOverrides(module morphomod.SharePriceModule, overrides []storage.ThresholdOverride) (monitor.Module, error) {
	for _, override := range overridesForModule(overrides, core.ModuleSharePriceLoss) {
		if err := validateThresholdValue(override.ModuleID, override.Key, override.Value, 0); err != nil {
			return nil, err
		}
		switch override.Key {
		case moduleConfigKeyLossWarnBPS:
			value, err := parseThresholdBPS(core.ModuleSharePriceLoss, override.Key, override.Value)
			if err != nil {
				return nil, err
			}
			module.WarnBPS = value
		case moduleConfigKeyLossUrgentBPS:
			value, err := parseThresholdBPS(core.ModuleSharePriceLoss, override.Key, override.Value)
			if err != nil {
				return nil, err
			}
			module.UrgentBPS = value
		}
	}
	return module, nil
}

func applyWithdrawLiquidityOverrides(module morphomod.WithdrawLiquidityModule, overrides []storage.ThresholdOverride, assetDecimals uint8) (monitor.Module, error) {
	for _, override := range overridesForModule(overrides, core.ModuleWithdrawLiquidity) {
		if err := validateThresholdValue(override.ModuleID, override.Key, override.Value, assetDecimals); err != nil {
			return nil, err
		}
		switch override.Key {
		case moduleConfigKeyIdleWarnThresholdUSDC:
			value, err := parseThresholdAssetUnits(core.ModuleWithdrawLiquidity, override.Key, override.Value, assetDecimals)
			if err != nil {
				return nil, err
			}
			module.IdleWarn = value
		case moduleConfigKeyIdleUrgentThresholdUSDC:
			value, err := parseThresholdAssetUnits(core.ModuleWithdrawLiquidity, override.Key, override.Value, assetDecimals)
			if err != nil {
				return nil, err
			}
			module.IdleUrgent = value
		}
	}
	return module, nil
}

func applyVaultStateOverrides(module morphomod.VaultStateModule, overrides []storage.ThresholdOverride) (monitor.Module, error) {
	for _, override := range overridesForModule(overrides, core.ModuleVaultState) {
		if err := validateThresholdValue(override.ModuleID, override.Key, override.Value, 0); err != nil {
			return nil, err
		}
		if override.Key == moduleConfigKeyChangeSeverity {
			module.ChangeSeverity = core.Severity(override.Value)
		}
	}
	return module, nil
}

func overridesForModule(overrides []storage.ThresholdOverride, moduleID core.MonitorModuleID) []storage.ThresholdOverride {
	result := make([]storage.ThresholdOverride, 0, len(overrides))
	for _, override := range overrides {
		if override.ModuleID == string(moduleID) {
			result = append(result, override)
		}
	}
	return result
}

func validateThresholdValue(moduleID string, key string, value string, assetDecimals uint8) error {
	switch key {
	case moduleConfigKeyLossWarnBPS, moduleConfigKeyLossUrgentBPS:
		_, err := parseThresholdBPS(core.MonitorModuleID(moduleID), key, value)
		return err
	case moduleConfigKeyIdleWarnThresholdUSDC, moduleConfigKeyIdleUrgentThresholdUSDC:
		_, err := parseThresholdAssetUnits(core.MonitorModuleID(moduleID), key, value, assetDecimals)
		return err
	case moduleConfigKeyChangeSeverity:
		if value != string(core.SeverityWarn) && value != string(core.SeverityUrgent) {
			return fmt.Errorf(errThresholdSeverity, moduleID, key)
		}
	case moduleConfigKeyStaleUrgentAfter:
		duration, err := time.ParseDuration(value)
		if err != nil || duration <= 0 {
			return fmt.Errorf(errThresholdDuration, moduleID, key)
		}
	}
	return nil
}

func parseThresholdBPS(moduleID core.MonitorModuleID, key string, value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf(errThresholdBPSInteger, moduleID, key)
	}
	if err := config.ValidateBPS(fmt.Sprintf("%s.%s", moduleID, key), parsed); err != nil || parsed <= 0 {
		return 0, fmt.Errorf(errThresholdBPSRange, moduleID, key)
	}
	return parsed, nil
}

func parseThresholdAssetUnits(moduleID core.MonitorModuleID, key string, value string, assetDecimals uint8) (*big.Int, error) {
	parsed, err := config.ParseDecimalUnits(fmt.Sprintf("%s.%s", moduleID, key), value, assetDecimals)
	if err != nil {
		return nil, err
	}
	if parsed.Sign() <= 0 {
		return nil, fmt.Errorf(errThresholdAssetUnits, moduleID, key)
	}
	return parsed, nil
}
