package app

import (
	"context"
	"fmt"
	"math/big"
	"strconv"

	"withdraw-bot/internal/config"
	"withdraw-bot/internal/core"
	"withdraw-bot/internal/monitor"
	morphomod "withdraw-bot/internal/monitor/modules/morpho"
	"withdraw-bot/internal/storage"

	"github.com/ethereum/go-ethereum/common"
)

const (
	errThresholdBPSInteger    = "%s.%s must be an integer"
	errThresholdBPSRange      = "%s.%s must be between 1 and 10000 bps"
	errThresholdDuration      = "%s.%s must be a positive duration"
	errThresholdSeverity      = "%s.%s must be warn or urgent"
	errThresholdAssetUnits    = "%s.%s must be positive"
	validationOwnerAddress    = "0x0000000000000000000000000000000000000001"
	validationReceiverAddress = "0x0000000000000000000000000000000000000002"
	validationVaultAddress    = "0x0000000000000000000000000000000000000003"
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
	}
	return nil
}

func (provider thresholdProvider) validateEffectiveThreshold(ctx context.Context, request telegramThresholdRequest) error {
	moduleID := core.MonitorModuleID(request.ModuleID)
	moduleConfig, ok := provider.config.Modules[request.ModuleID]
	if !ok {
		return nil
	}
	overrides, err := provider.repos.ListThresholdOverrides(ctx)
	if err != nil {
		return err
	}
	overrides = upsertEffectiveOverride(overrides, request)
	switch moduleID {
	case core.ModuleSharePriceLoss:
		module, err := sharePriceThresholdConfig(moduleConfig)
		if err != nil {
			return err
		}
		effective, err := applySharePriceOverrides(module, overrides)
		if err != nil {
			return err
		}
		return effective.(morphomod.SharePriceModule).ValidateConfig(ctx)
	case core.ModuleWithdrawLiquidity:
		module, err := withdrawLiquidityThresholdConfig(moduleConfig, provider.assetDecimals)
		if err != nil {
			return err
		}
		effective, err := applyWithdrawLiquidityOverrides(module, overrides, provider.assetDecimals)
		if err != nil {
			return err
		}
		return effective.(morphomod.WithdrawLiquidityModule).ValidateConfig(ctx)
	case core.ModuleVaultState:
		module, err := vaultStateThresholdConfig(moduleConfig)
		if err != nil {
			return err
		}
		effective, err := applyVaultStateOverrides(module, overrides)
		if err != nil {
			return err
		}
		return effective.(morphomod.VaultStateModule).ValidateConfig(ctx)
	default:
		return nil
	}
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

func sharePriceThresholdConfig(moduleConfig config.ModuleConfig) (morphomod.SharePriceModule, error) {
	warn, err := moduleInt64(moduleConfig, core.ModuleSharePriceLoss, moduleConfigKeyLossWarnBPS)
	if err != nil {
		return morphomod.SharePriceModule{}, err
	}
	urgent, err := moduleInt64(moduleConfig, core.ModuleSharePriceLoss, moduleConfigKeyLossUrgentBPS)
	if err != nil {
		return morphomod.SharePriceModule{}, err
	}
	return morphomod.SharePriceModule{BaselineSharePrice: big.NewInt(1), WarnBPS: warn, UrgentBPS: urgent, Reader: noopSharePriceReader{}}, nil
}

func withdrawLiquidityThresholdConfig(moduleConfig config.ModuleConfig, assetDecimals uint8) (morphomod.WithdrawLiquidityModule, error) {
	warn, err := moduleDecimalUnits(moduleConfig, core.ModuleWithdrawLiquidity, moduleConfigKeyIdleWarnThresholdUSDC, assetDecimals)
	if err != nil {
		return morphomod.WithdrawLiquidityModule{}, err
	}
	urgent, err := moduleDecimalUnits(moduleConfig, core.ModuleWithdrawLiquidity, moduleConfigKeyIdleUrgentThresholdUSDC, assetDecimals)
	if err != nil {
		return morphomod.WithdrawLiquidityModule{}, err
	}
	return morphomod.WithdrawLiquidityModule{
		IdleAssetReader: noopIdleAssetReader{},
		ExitSimulator:   noopExitSimulator{},
		Owner:           common.HexToAddress(validationOwnerAddress),
		Receiver:        common.HexToAddress(validationReceiverAddress),
		Vault:           common.HexToAddress(validationVaultAddress),
		IdleWarn:        warn,
		IdleUrgent:      urgent,
	}, nil
}

func vaultStateThresholdConfig(moduleConfig config.ModuleConfig) (morphomod.VaultStateModule, error) {
	severity, err := moduleString(moduleConfig, core.ModuleVaultState, moduleConfigKeyChangeSeverity)
	if err != nil {
		return morphomod.VaultStateModule{}, err
	}
	return morphomod.VaultStateModule{Reader: noopVaultStateReader{}, ChangeSeverity: core.Severity(severity)}, nil
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

type noopSharePriceReader struct{}

func (reader noopSharePriceReader) CurrentSharePrice(ctx context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}

type noopIdleAssetReader struct{}

func (reader noopIdleAssetReader) IdleAssets(ctx context.Context, vault common.Address) (*big.Int, error) {
	return big.NewInt(1), nil
}

type noopExitSimulator struct{}

func (simulator noopExitSimulator) Position(ctx context.Context) (core.PositionSnapshot, error) {
	return core.PositionSnapshot{}, nil
}

func (simulator noopExitSimulator) SimulateFullExit(ctx context.Context, req core.FullExitRequest) (core.FullExitSimulation, error) {
	return core.FullExitSimulation{}, nil
}

type noopVaultStateReader struct{}

func (reader noopVaultStateReader) CurrentVaultState(ctx context.Context) (morphomod.VaultStateSnapshot, error) {
	return morphomod.VaultStateSnapshot{}, nil
}
