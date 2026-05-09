package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"withdraw-bot/internal/config"
	"withdraw-bot/internal/core"
	"withdraw-bot/internal/ethereum"
	"withdraw-bot/internal/monitor"
	morphomod "withdraw-bot/internal/monitor/modules/morpho"
	morphovault "withdraw-bot/internal/morpho"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"gopkg.in/yaml.v3"
)

const (
	moduleConfigKeyEnabled                      = "enabled"
	moduleConfigKeyBaselineSharePriceAssetUnits = "baseline_share_price_asset_units"
	moduleConfigKeyLossWarnBPS                  = "loss_warn_bps"
	moduleConfigKeyLossUrgentBPS                = "loss_urgent_bps"
	moduleConfigKeyIdleWarnThresholdUSDC        = "idle_warn_threshold_usdc"
	moduleConfigKeyIdleUrgentThresholdUSDC      = "idle_urgent_threshold_usdc"
	moduleConfigKeyChangeSeverity               = "change_severity"
	moduleConfigKeyStaleUrgentAfter             = "stale_urgent_after"
	moduleConfigKeyBaseline                     = "baseline"
	vaultMethodAsset                            = "asset"
	vaultMethodTotalAssets                      = "totalAssets"
	vaultMethodTotalSupply                      = "totalSupply"
	vaultMethodOwner                            = "owner"
	vaultMethodCurator                          = "curator"
	vaultMethodReceiveSharesGate                = "receiveSharesGate"
	vaultMethodSendSharesGate                   = "sendSharesGate"
	vaultMethodReceiveAssetsGate                = "receiveAssetsGate"
	vaultMethodSendAssetsGate                   = "sendAssetsGate"
	vaultMethodAdapterRegistry                  = "adapterRegistry"
	vaultMethodLiquidityAdapter                 = "liquidityAdapter"
	vaultMethodLiquidityData                    = "liquidityData"
	vaultMethodPerformanceFee                   = "performanceFee"
	vaultMethodPerformanceFeeRecipient          = "performanceFeeRecipient"
	vaultMethodManagementFee                    = "managementFee"
	vaultMethodManagementFeeRecipient           = "managementFeeRecipient"
	vaultMethodMaxRate                          = "maxRate"
	vaultMethodAdaptersLength                   = "adaptersLength"
	vaultMethodAdapters                         = "adapters"
	erc20MethodBalanceOf                        = "balanceOf"
	errMissingModuleConfig                      = "%s module config is required"
	errUnknownEnabledModule                     = "unknown enabled module %q"
	errMissingModuleEnabled                     = "%s.enabled is required"
	errInvalidModuleEnabled                     = "%s.enabled must be a bool"
	errMissingModuleField                       = "%s.%s is required"
	errInvalidModuleInteger                     = "%s.%s must be an integer"
	errInvalidModuleString                      = "%s.%s must be a string"
	errInvalidVaultOutput                       = "call %s: expected %s"
)

var knownModuleIDs = map[core.MonitorModuleID]bool{
	core.ModuleSharePriceLoss:    true,
	core.ModuleWithdrawLiquidity: true,
	core.ModuleVaultState:        true,
}

type vaultReader struct {
	Ethereum      ethereum.MultiClient
	Vault         common.Address
	AssetDecimals uint8
}

func buildModules(cfg config.Config, ethClient ethereum.MultiClient, vault common.Address, owner common.Address, receiver common.Address, adapter withdrawAdapter) ([]monitor.Module, error) {
	reader := vaultReader{Ethereum: ethClient, Vault: vault, AssetDecimals: cfg.Ethereum.AssetDecimals}
	modules := make([]monitor.Module, 0, len(cfg.Modules))

	if err := validateModuleConfigs(cfg); err != nil {
		return nil, err
	}

	shareConfig, ok, err := enabledModuleConfig(cfg, core.ModuleSharePriceLoss)
	if err != nil {
		return nil, err
	}
	if ok {
		module, err := buildSharePriceModule(shareConfig, reader)
		if err != nil {
			return nil, err
		}
		modules = append(modules, module)
	}

	liquidityConfig, ok, err := enabledModuleConfig(cfg, core.ModuleWithdrawLiquidity)
	if err != nil {
		return nil, err
	}
	if ok {
		module, err := buildWithdrawLiquidityModule(cfg, liquidityConfig, reader, adapter, vault, owner, receiver)
		if err != nil {
			return nil, err
		}
		modules = append(modules, module)
	}

	vaultStateConfig, ok, err := enabledModuleConfig(cfg, core.ModuleVaultState)
	if err != nil {
		return nil, err
	}
	if ok {
		module, err := buildVaultStateModule(vaultStateConfig, reader)
		if err != nil {
			return nil, err
		}
		modules = append(modules, module)
	}

	return modules, nil
}

type withdrawAdapter interface {
	Position(ctx context.Context) (core.PositionSnapshot, error)
	SimulateFullExit(ctx context.Context, req core.FullExitRequest) (core.FullExitSimulation, error)
}

func buildSharePriceModule(moduleConfig config.ModuleConfig, reader vaultReader) (morphomod.SharePriceModule, error) {
	baseline, err := moduleBigInt(moduleConfig, core.ModuleSharePriceLoss, moduleConfigKeyBaselineSharePriceAssetUnits)
	if err != nil {
		return morphomod.SharePriceModule{}, err
	}
	warn, err := moduleInt64(moduleConfig, core.ModuleSharePriceLoss, moduleConfigKeyLossWarnBPS)
	if err != nil {
		return morphomod.SharePriceModule{}, err
	}
	urgent, err := moduleInt64(moduleConfig, core.ModuleSharePriceLoss, moduleConfigKeyLossUrgentBPS)
	if err != nil {
		return morphomod.SharePriceModule{}, err
	}
	return morphomod.SharePriceModule{
		BaselineSharePrice: baseline,
		WarnBPS:            warn,
		UrgentBPS:          urgent,
		Reader:             reader,
		Clock:              core.SystemClock{},
	}, nil
}

func buildWithdrawLiquidityModule(cfg config.Config, moduleConfig config.ModuleConfig, reader vaultReader, adapter withdrawAdapter, vault common.Address, owner common.Address, receiver common.Address) (morphomod.WithdrawLiquidityModule, error) {
	warn, err := moduleDecimalUnits(moduleConfig, core.ModuleWithdrawLiquidity, moduleConfigKeyIdleWarnThresholdUSDC, cfg.Ethereum.AssetDecimals)
	if err != nil {
		return morphomod.WithdrawLiquidityModule{}, err
	}
	urgent, err := moduleDecimalUnits(moduleConfig, core.ModuleWithdrawLiquidity, moduleConfigKeyIdleUrgentThresholdUSDC, cfg.Ethereum.AssetDecimals)
	if err != nil {
		return morphomod.WithdrawLiquidityModule{}, err
	}
	return morphomod.WithdrawLiquidityModule{
		IdleAssetReader: reader,
		ExitSimulator:   adapter,
		Owner:           owner,
		Receiver:        receiver,
		Vault:           vault,
		IdleWarn:        warn,
		IdleUrgent:      urgent,
		Clock:           core.SystemClock{},
	}, nil
}

func buildVaultStateModule(moduleConfig config.ModuleConfig, reader vaultReader) (morphomod.VaultStateModule, error) {
	var baseline morphomod.VaultStateSnapshot
	if err := moduleStruct(moduleConfig, core.ModuleVaultState, moduleConfigKeyBaseline, &baseline); err != nil {
		return morphomod.VaultStateModule{}, err
	}
	severity, err := moduleString(moduleConfig, core.ModuleVaultState, moduleConfigKeyChangeSeverity)
	if err != nil {
		return morphomod.VaultStateModule{}, err
	}
	return morphomod.VaultStateModule{
		Reader:         reader,
		Baseline:       baseline,
		ChangeSeverity: core.Severity(severity),
		Clock:          core.SystemClock{},
	}, nil
}

func validateModuleConfigs(cfg config.Config) error {
	for rawID, moduleConfig := range cfg.Modules {
		moduleID := core.MonitorModuleID(rawID)
		enabled, err := moduleEnabled(moduleConfig, moduleID)
		if err != nil {
			return err
		}
		if enabled && !knownModuleIDs[moduleID] {
			return fmt.Errorf(errUnknownEnabledModule, rawID)
		}
	}
	return nil
}

func enabledModuleConfig(cfg config.Config, moduleID core.MonitorModuleID) (config.ModuleConfig, bool, error) {
	moduleConfig, ok := cfg.Modules[string(moduleID)]
	if !ok {
		return nil, false, nil
	}
	enabled, err := moduleEnabled(moduleConfig, moduleID)
	if err != nil {
		return nil, false, err
	}
	if !enabled {
		return nil, false, nil
	}
	return moduleConfig, true, nil
}

func moduleEnabled(moduleConfig config.ModuleConfig, moduleID core.MonitorModuleID) (bool, error) {
	value, ok := moduleConfig[moduleConfigKeyEnabled]
	if !ok {
		return false, fmt.Errorf(errMissingModuleEnabled, moduleID)
	}
	enabled, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf(errInvalidModuleEnabled, moduleID)
	}
	return enabled, nil
}

func moduleString(moduleConfig config.ModuleConfig, moduleID core.MonitorModuleID, key string) (string, error) {
	value, ok := moduleConfig[key]
	if !ok {
		return "", fmt.Errorf(errMissingModuleField, moduleID, key)
	}
	text, ok := value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf(errInvalidModuleString, moduleID, key)
	}
	return text, nil
}

func moduleBigInt(moduleConfig config.ModuleConfig, moduleID core.MonitorModuleID, key string) (*big.Int, error) {
	text, err := moduleString(moduleConfig, moduleID, key)
	if err != nil {
		return nil, err
	}
	value, ok := new(big.Int).SetString(text, 10)
	if !ok {
		return nil, fmt.Errorf(errInvalidModuleString, moduleID, key)
	}
	return value, nil
}

func moduleInt64(moduleConfig config.ModuleConfig, moduleID core.MonitorModuleID, key string) (int64, error) {
	value, ok := moduleConfig[key]
	if !ok {
		return 0, fmt.Errorf(errMissingModuleField, moduleID, key)
	}
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	default:
		return 0, fmt.Errorf(errInvalidModuleInteger, moduleID, key)
	}
}

func moduleDecimalUnits(moduleConfig config.ModuleConfig, moduleID core.MonitorModuleID, key string, decimals uint8) (*big.Int, error) {
	text, err := moduleString(moduleConfig, moduleID, key)
	if err != nil {
		return nil, err
	}
	return config.ParseDecimalUnits(fmt.Sprintf("%s.%s", moduleID, key), text, decimals)
}

func moduleStruct(moduleConfig config.ModuleConfig, moduleID core.MonitorModuleID, key string, target any) error {
	value, ok := moduleConfig[key]
	if !ok {
		return fmt.Errorf(errMissingModuleField, moduleID, key)
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	var intermediate any
	if err := yaml.Unmarshal(data, &intermediate); err != nil {
		return err
	}
	normalized, err := json.Marshal(intermediate)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(normalized, target); err != nil {
		return err
	}
	return nil
}

func (reader vaultReader) CurrentSharePrice(ctx context.Context) (*big.Int, error) {
	totalAssets, err := reader.vaultUint256(ctx, vaultMethodTotalAssets)
	if err != nil {
		return nil, err
	}
	totalSupply, err := reader.vaultUint256(ctx, vaultMethodTotalSupply)
	if err != nil {
		return nil, err
	}
	if totalSupply.Sign() <= 0 {
		return nil, fmt.Errorf(errInvalidVaultOutput, vaultMethodTotalSupply, "positive total supply")
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(reader.AssetDecimals)), nil)
	return new(big.Int).Div(new(big.Int).Mul(totalAssets, scale), totalSupply), nil
}

func (reader vaultReader) IdleAssets(ctx context.Context, vault common.Address) (*big.Int, error) {
	asset, err := reader.vaultAddress(ctx, vaultMethodAsset)
	if err != nil {
		return nil, err
	}
	data, err := morphovault.ERC20ABI.Pack(erc20MethodBalanceOf, vault)
	if err != nil {
		return nil, err
	}
	raw, err := reader.Ethereum.CallContract(ctx, geth.CallMsg{To: &asset, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	out, err := morphovault.ERC20ABI.Unpack(erc20MethodBalanceOf, raw)
	if err != nil {
		return nil, err
	}
	return uint256Output(erc20MethodBalanceOf, out)
}

func (reader vaultReader) CurrentVaultState(ctx context.Context) (morphomod.VaultStateSnapshot, error) {
	owner, err := reader.vaultAddressString(ctx, vaultMethodOwner)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	curator, err := reader.vaultAddressString(ctx, vaultMethodCurator)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	receiveSharesGate, err := reader.vaultAddressString(ctx, vaultMethodReceiveSharesGate)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	sendSharesGate, err := reader.vaultAddressString(ctx, vaultMethodSendSharesGate)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	receiveAssetsGate, err := reader.vaultAddressString(ctx, vaultMethodReceiveAssetsGate)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	sendAssetsGate, err := reader.vaultAddressString(ctx, vaultMethodSendAssetsGate)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	adapterRegistry, err := reader.vaultAddressString(ctx, vaultMethodAdapterRegistry)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	liquidityAdapter, err := reader.vaultAddressString(ctx, vaultMethodLiquidityAdapter)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	liquidityData, err := reader.vaultBytes(ctx, vaultMethodLiquidityData)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	performanceFee, err := reader.vaultValueString(ctx, vaultMethodPerformanceFee)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	performanceFeeRecipient, err := reader.vaultAddressString(ctx, vaultMethodPerformanceFeeRecipient)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	managementFee, err := reader.vaultValueString(ctx, vaultMethodManagementFee)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	managementFeeRecipient, err := reader.vaultAddressString(ctx, vaultMethodManagementFeeRecipient)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	maxRate, err := reader.vaultValueString(ctx, vaultMethodMaxRate)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	adapters, err := reader.vaultAdapters(ctx)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	return morphomod.VaultStateSnapshot{
		Owner:                    owner,
		Curator:                  curator,
		ReceiveSharesGate:        receiveSharesGate,
		SendSharesGate:           sendSharesGate,
		ReceiveAssetsGate:        receiveAssetsGate,
		SendAssetsGate:           sendAssetsGate,
		AdapterRegistry:          adapterRegistry,
		LiquidityAdapter:         liquidityAdapter,
		LiquidityDataHex:         hexutil.Encode(liquidityData),
		PerformanceFee:           performanceFee,
		PerformanceFeeRecipient:  performanceFeeRecipient,
		ManagementFee:            managementFee,
		ManagementFeeRecipient:   managementFeeRecipient,
		MaxRate:                  maxRate,
		Adapters:                 adapters,
		AllocatorRoles:           map[string]bool{},
		SentinelRoles:            map[string]bool{},
		Timelocks:                map[string]string{},
		Abdicated:                map[string]bool{},
		ForceDeallocatePenalties: map[string]string{},
	}, nil
}

func (reader vaultReader) vaultAddressString(ctx context.Context, method string) (string, error) {
	address, err := reader.vaultAddress(ctx, method)
	if err != nil {
		return "", err
	}
	return address.Hex(), nil
}

func (reader vaultReader) vaultValueString(ctx context.Context, method string) (string, error) {
	out, err := reader.vaultCall(ctx, method)
	if err != nil {
		return "", err
	}
	if len(out) != 1 {
		return "", fmt.Errorf(errInvalidVaultOutput, method, "one value")
	}
	switch typed := out[0].(type) {
	case *big.Int:
		return typed.String(), nil
	case uint64:
		return fmt.Sprint(typed), nil
	default:
		return fmt.Sprint(typed), nil
	}
}

func (reader vaultReader) vaultAdapters(ctx context.Context) ([]string, error) {
	length, err := reader.vaultUint256(ctx, vaultMethodAdaptersLength)
	if err != nil {
		return nil, err
	}
	adapters := make([]string, 0, int(length.Int64()))
	for index := int64(0); index < length.Int64(); index++ {
		address, err := reader.vaultAddress(ctx, vaultMethodAdapters, big.NewInt(index))
		if err != nil {
			return nil, err
		}
		adapters = append(adapters, address.Hex())
	}
	return adapters, nil
}

func (reader vaultReader) vaultUint256(ctx context.Context, method string, args ...any) (*big.Int, error) {
	out, err := reader.vaultCall(ctx, method, args...)
	if err != nil {
		return nil, err
	}
	return uint256Output(method, out)
}

func (reader vaultReader) vaultAddress(ctx context.Context, method string, args ...any) (common.Address, error) {
	out, err := reader.vaultCall(ctx, method, args...)
	if err != nil {
		return common.Address{}, err
	}
	if len(out) != 1 {
		return common.Address{}, fmt.Errorf(errInvalidVaultOutput, method, "one address")
	}
	address, ok := out[0].(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf(errInvalidVaultOutput, method, "address")
	}
	return address, nil
}

func (reader vaultReader) vaultBytes(ctx context.Context, method string, args ...any) ([]byte, error) {
	out, err := reader.vaultCall(ctx, method, args...)
	if err != nil {
		return nil, err
	}
	if len(out) != 1 {
		return nil, fmt.Errorf(errInvalidVaultOutput, method, "one bytes value")
	}
	data, ok := out[0].([]byte)
	if !ok {
		return nil, fmt.Errorf(errInvalidVaultOutput, method, "bytes")
	}
	return data, nil
}

func (reader vaultReader) vaultCall(ctx context.Context, method string, args ...any) ([]any, error) {
	data, err := morphovault.VaultABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	raw, err := reader.Ethereum.CallContract(ctx, geth.CallMsg{To: &reader.Vault, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	return morphovault.VaultABI.Unpack(method, raw)
}

func uint256Output(method string, out []any) (*big.Int, error) {
	if len(out) != 1 {
		return nil, fmt.Errorf(errInvalidVaultOutput, method, "one uint256")
	}
	value, ok := out[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(errInvalidVaultOutput, method, "uint256")
	}
	return new(big.Int).Set(value), nil
}
