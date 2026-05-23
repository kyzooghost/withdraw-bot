package morpho

import (
	"context"
	"fmt"
	"math/big"

	"withdraw-bot/internal/config"
	"withdraw-bot/internal/core"
	"withdraw-bot/internal/ethereum"
	morphomod "withdraw-bot/internal/monitor/modules/morpho"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const (
	moduleConfigKeyBaselineSharePriceAssetUnits = "baseline_share_price_asset_units"
	moduleConfigKeyLossWarnBPS                  = "loss_warn_bps"
	moduleConfigKeyLossUrgentBPS                = "loss_urgent_bps"
	moduleConfigKeyIdleWarnThresholdUSDC        = "idle_warn_threshold_usdc"
	moduleConfigKeyIdleUrgentThresholdUSDC      = "idle_urgent_threshold_usdc"
	moduleConfigKeyChangeSeverity               = "change_severity"
	moduleConfigKeyBaseline                     = "baseline"
	factoryMethodAsset                          = "asset"
	factoryMethodTotalAssets                    = "totalAssets"
	factoryMethodTotalSupply                    = "totalSupply"
	factoryMethodOwner                          = "owner"
	factoryMethodCurator                        = "curator"
	factoryMethodReceiveSharesGate              = "receiveSharesGate"
	factoryMethodSendSharesGate                 = "sendSharesGate"
	factoryMethodReceiveAssetsGate              = "receiveAssetsGate"
	factoryMethodSendAssetsGate                 = "sendAssetsGate"
	factoryMethodAdapterRegistry                = "adapterRegistry"
	factoryMethodLiquidityAdapter               = "liquidityAdapter"
	factoryMethodLiquidityData                  = "liquidityData"
	factoryMethodPerformanceFee                 = "performanceFee"
	factoryMethodPerformanceFeeRecipient        = "performanceFeeRecipient"
	factoryMethodManagementFee                  = "managementFee"
	factoryMethodManagementFeeRecipient         = "managementFeeRecipient"
	factoryMethodMaxRate                        = "maxRate"
	factoryMethodAdaptersLength                 = "adaptersLength"
	factoryMethodAdapters                       = "adapters"
	erc20MethodBalanceOf                        = "balanceOf"
	errUnknownEnabledModule                     = "unknown enabled module %q"
	errInvalidVaultOutput                       = "call %s: expected %s"
)

var knownModuleIDs = map[core.MonitorModuleID]bool{
	core.ModuleSharePriceLoss:    true,
	core.ModuleWithdrawLiquidity: true,
	core.ModuleVaultState:        true,
}

// Factory builds Morpho Vault V2 monitor modules.
type Factory struct{}

// NewFactory returns a new Morpho protocol factory.
func NewFactory() *Factory { return &Factory{} }

// BuildModules constructs all enabled Morpho monitor modules from config.
func (f *Factory) BuildModules(cfg config.Config, ethClient ethereum.MultiClient, vault, owner, receiver common.Address, exitSim core.ExitSimulator) ([]core.MonitorModule, error) {
	reader := vaultReader{Ethereum: ethClient, Vault: vault, AssetDecimals: cfg.Ethereum.AssetDecimals}
	modules := make([]core.MonitorModule, 0, len(cfg.Modules))

	if err := validateModuleConfigs(cfg); err != nil {
		return nil, err
	}

	shareConfig, ok, err := config.EnabledModuleConfig(cfg, core.ModuleSharePriceLoss)
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

	liquidityConfig, ok, err := config.EnabledModuleConfig(cfg, core.ModuleWithdrawLiquidity)
	if err != nil {
		return nil, err
	}
	if ok {
		module, err := buildWithdrawLiquidityModule(cfg, liquidityConfig, reader, exitSim, vault, owner, receiver)
		if err != nil {
			return nil, err
		}
		modules = append(modules, module)
	}

	vaultStateConfig, ok, err := config.EnabledModuleConfig(cfg, core.ModuleVaultState)
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

func validateModuleConfigs(cfg config.Config) error {
	for rawID, moduleConfig := range cfg.Modules {
		moduleID := core.MonitorModuleID(rawID)
		enabled, err := config.ModuleEnabled(moduleConfig, moduleID)
		if err != nil {
			return err
		}
		if enabled && !knownModuleIDs[moduleID] {
			return fmt.Errorf(errUnknownEnabledModule, rawID)
		}
	}
	return nil
}

func buildSharePriceModule(moduleConfig config.ModuleConfig, reader vaultReader) (morphomod.SharePriceModule, error) {
	baseline, err := config.ModuleBigInt(moduleConfig, core.ModuleSharePriceLoss, moduleConfigKeyBaselineSharePriceAssetUnits)
	if err != nil {
		return morphomod.SharePriceModule{}, err
	}
	warn, err := config.ModuleInt64(moduleConfig, core.ModuleSharePriceLoss, moduleConfigKeyLossWarnBPS)
	if err != nil {
		return morphomod.SharePriceModule{}, err
	}
	urgent, err := config.ModuleInt64(moduleConfig, core.ModuleSharePriceLoss, moduleConfigKeyLossUrgentBPS)
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

func buildWithdrawLiquidityModule(cfg config.Config, moduleConfig config.ModuleConfig, reader vaultReader, exitSim core.ExitSimulator, vault, owner, receiver common.Address) (morphomod.WithdrawLiquidityModule, error) {
	warn, err := config.ModuleDecimalUnits(moduleConfig, core.ModuleWithdrawLiquidity, moduleConfigKeyIdleWarnThresholdUSDC, cfg.Ethereum.AssetDecimals)
	if err != nil {
		return morphomod.WithdrawLiquidityModule{}, err
	}
	urgent, err := config.ModuleDecimalUnits(moduleConfig, core.ModuleWithdrawLiquidity, moduleConfigKeyIdleUrgentThresholdUSDC, cfg.Ethereum.AssetDecimals)
	if err != nil {
		return morphomod.WithdrawLiquidityModule{}, err
	}
	return morphomod.WithdrawLiquidityModule{
		IdleAssetReader: reader,
		ExitSimulator:   exitSim,
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
	if err := config.ModuleStruct(moduleConfig, core.ModuleVaultState, moduleConfigKeyBaseline, &baseline); err != nil {
		return morphomod.VaultStateModule{}, err
	}
	severity, err := config.ModuleString(moduleConfig, core.ModuleVaultState, moduleConfigKeyChangeSeverity)
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

// vaultReader reads on-chain state from a Morpho Vault V2 contract.
type vaultReader struct {
	Ethereum      ethereum.MultiClient
	Vault         common.Address
	AssetDecimals uint8
}

func (reader vaultReader) CurrentSharePrice(ctx context.Context) (*big.Int, error) {
	totalAssets, err := reader.vaultUint256(ctx, factoryMethodTotalAssets)
	if err != nil {
		return nil, err
	}
	totalSupply, err := reader.vaultUint256(ctx, factoryMethodTotalSupply)
	if err != nil {
		return nil, err
	}
	if totalSupply.Sign() <= 0 {
		return nil, fmt.Errorf(errInvalidVaultOutput, factoryMethodTotalSupply, "positive total supply")
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(reader.AssetDecimals)), nil)
	return new(big.Int).Div(new(big.Int).Mul(totalAssets, scale), totalSupply), nil
}

func (reader vaultReader) IdleAssets(ctx context.Context, vault common.Address) (*big.Int, error) {
	asset, err := reader.vaultAddress(ctx, factoryMethodAsset)
	if err != nil {
		return nil, err
	}
	data, err := ERC20ABI.Pack(erc20MethodBalanceOf, vault)
	if err != nil {
		return nil, err
	}
	raw, err := reader.Ethereum.CallContract(ctx, geth.CallMsg{To: &asset, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	out, err := ERC20ABI.Unpack(erc20MethodBalanceOf, raw)
	if err != nil {
		return nil, err
	}
	return parseUint256Output(erc20MethodBalanceOf, out)
}

func (reader vaultReader) CurrentVaultState(ctx context.Context) (morphomod.VaultStateSnapshot, error) {
	owner, err := reader.vaultAddressString(ctx, factoryMethodOwner)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	curator, err := reader.vaultAddressString(ctx, factoryMethodCurator)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	receiveSharesGate, err := reader.vaultAddressString(ctx, factoryMethodReceiveSharesGate)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	sendSharesGate, err := reader.vaultAddressString(ctx, factoryMethodSendSharesGate)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	receiveAssetsGate, err := reader.vaultAddressString(ctx, factoryMethodReceiveAssetsGate)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	sendAssetsGate, err := reader.vaultAddressString(ctx, factoryMethodSendAssetsGate)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	adapterRegistry, err := reader.vaultAddressString(ctx, factoryMethodAdapterRegistry)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	liquidityAdapter, err := reader.vaultAddressString(ctx, factoryMethodLiquidityAdapter)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	liquidityData, err := reader.vaultBytes(ctx, factoryMethodLiquidityData)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	performanceFee, err := reader.vaultValueString(ctx, factoryMethodPerformanceFee)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	performanceFeeRecipient, err := reader.vaultAddressString(ctx, factoryMethodPerformanceFeeRecipient)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	managementFee, err := reader.vaultValueString(ctx, factoryMethodManagementFee)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	managementFeeRecipient, err := reader.vaultAddressString(ctx, factoryMethodManagementFeeRecipient)
	if err != nil {
		return morphomod.VaultStateSnapshot{}, err
	}
	maxRate, err := reader.vaultValueString(ctx, factoryMethodMaxRate)
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
	length, err := reader.vaultUint256(ctx, factoryMethodAdaptersLength)
	if err != nil {
		return nil, err
	}
	adapters := make([]string, 0, int(length.Int64()))
	for index := int64(0); index < length.Int64(); index++ {
		address, err := reader.vaultAddress(ctx, factoryMethodAdapters, big.NewInt(index))
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
	return parseUint256Output(method, out)
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
	data, err := VaultABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	raw, err := reader.Ethereum.CallContract(ctx, geth.CallMsg{To: &reader.Vault, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	return VaultABI.Unpack(method, raw)
}

func parseUint256Output(method string, out []any) (*big.Int, error) {
	if len(out) != 1 {
		return nil, fmt.Errorf(errInvalidVaultOutput, method, "one uint256")
	}
	value, ok := out[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf(errInvalidVaultOutput, method, "uint256")
	}
	return new(big.Int).Set(value), nil
}
