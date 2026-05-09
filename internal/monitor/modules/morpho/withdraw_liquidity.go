package morpho

import (
	"context"
	"fmt"
	"math/big"

	"withdraw-bot/internal/core"

	"github.com/ethereum/go-ethereum/common"
)

const (
	withdrawLiquidityIdleWarnConfigKey       = "idle_warn_threshold_usdc"
	withdrawLiquidityIdleUrgentConfigKey     = "idle_urgent_threshold_usdc"
	withdrawLiquidityIdleAssetsMetricKey     = "idle_assets"
	withdrawLiquidityExpectedExitMetricKey   = "expected_exit_assets"
	withdrawLiquidityMetricUnit              = "asset_units"
	withdrawLiquiditySimulationSuccessKey    = "simulation_success"
	withdrawLiquidityIdleLiquidityMessage    = "idle liquidity crossed threshold"
	withdrawLiquidityExitSimulationMessage   = "full-exit simulation failed"
	withdrawLiquidityShareBalanceConfigKey   = "share_balance"
	withdrawLiquidityExpectedExitConfigKey   = withdrawLiquidityExpectedExitMetricKey
	withdrawLiquidityIdleAssetsEvidenceKey   = withdrawLiquidityIdleAssetsMetricKey
	withdrawLiquidityExpectedExitEvidenceKey = withdrawLiquidityExpectedExitMetricKey
)

type IdleAssetReader interface {
	IdleAssets(ctx context.Context, vault common.Address) (*big.Int, error)
}

type ExitSimulator interface {
	Position(ctx context.Context) (core.PositionSnapshot, error)
	SimulateFullExit(ctx context.Context, req core.FullExitRequest) (core.FullExitSimulation, error)
}

type WithdrawLiquidityModule struct {
	IdleAssetReader IdleAssetReader
	ExitSimulator   ExitSimulator
	Owner           common.Address
	Receiver        common.Address
	Vault           common.Address
	IdleWarn        *big.Int
	IdleUrgent      *big.Int
	Clock           core.Clock
}

func (module WithdrawLiquidityModule) ID() core.MonitorModuleID {
	return core.ModuleWithdrawLiquidity
}

func (module WithdrawLiquidityModule) ValidateConfig(ctx context.Context) error {
	if module.IdleAssetReader == nil {
		return fmt.Errorf("%s idle asset reader is required", core.ModuleWithdrawLiquidity)
	}
	if module.ExitSimulator == nil {
		return fmt.Errorf("%s exit simulator is required", core.ModuleWithdrawLiquidity)
	}
	if module.Owner == (common.Address{}) {
		return fmt.Errorf("%s owner address is required", core.ModuleWithdrawLiquidity)
	}
	if module.Receiver == (common.Address{}) {
		return fmt.Errorf("%s receiver address is required", core.ModuleWithdrawLiquidity)
	}
	if module.Vault == (common.Address{}) {
		return fmt.Errorf("%s vault address is required", core.ModuleWithdrawLiquidity)
	}
	if module.IdleWarn == nil || module.IdleWarn.Sign() <= 0 {
		return fmt.Errorf("%s %s must be positive", core.ModuleWithdrawLiquidity, withdrawLiquidityIdleWarnConfigKey)
	}
	if module.IdleUrgent == nil || module.IdleUrgent.Sign() <= 0 {
		return fmt.Errorf("%s %s must be positive", core.ModuleWithdrawLiquidity, withdrawLiquidityIdleUrgentConfigKey)
	}
	if module.IdleWarn.Cmp(module.IdleUrgent) < 0 {
		return fmt.Errorf("%s warn threshold must be greater than or equal to urgent threshold", core.ModuleWithdrawLiquidity)
	}
	return nil
}

func (module WithdrawLiquidityModule) Bootstrap(ctx context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}

func (module WithdrawLiquidityModule) Monitor(ctx context.Context) (core.MonitorResult, error) {
	if err := module.ValidateConfig(ctx); err != nil {
		return core.MonitorResult{}, err
	}
	idleAssets, err := module.IdleAssetReader.IdleAssets(ctx, module.Vault)
	if err != nil {
		return core.MonitorResult{}, err
	}
	if idleAssets == nil || idleAssets.Sign() < 0 {
		return core.MonitorResult{}, fmt.Errorf("%s idle assets must be non-negative", core.ModuleWithdrawLiquidity)
	}
	position, err := module.ExitSimulator.Position(ctx)
	if err != nil {
		return core.MonitorResult{}, err
	}
	if position.ShareBalance == nil || position.ShareBalance.Sign() <= 0 {
		return core.MonitorResult{}, fmt.Errorf("%s %s must be positive", core.ModuleWithdrawLiquidity, withdrawLiquidityShareBalanceConfigKey)
	}
	if err := module.validatePosition(position); err != nil {
		return core.MonitorResult{}, err
	}
	simulation, err := module.ExitSimulator.SimulateFullExit(ctx, core.FullExitRequest{
		Vault:    module.Vault,
		Owner:    module.Owner,
		Receiver: module.Receiver,
		Shares:   new(big.Int).Set(position.ShareBalance),
	})
	if err != nil {
		return core.MonitorResult{}, err
	}
	expectedExitAssets := big.NewInt(0)
	if simulation.ExpectedAssetUnits != nil {
		if simulation.ExpectedAssetUnits.Sign() < 0 {
			return core.MonitorResult{}, fmt.Errorf("%s expected exit assets must be non-negative", core.ModuleWithdrawLiquidity)
		}
		expectedExitAssets = new(big.Int).Set(simulation.ExpectedAssetUnits)
	} else if simulation.Success {
		return core.MonitorResult{}, fmt.Errorf("%s %s is required for successful simulation", core.ModuleWithdrawLiquidity, withdrawLiquidityExpectedExitConfigKey)
	}
	clock := module.Clock
	if clock == nil {
		clock = core.SystemClock{}
	}
	status := core.MonitorStatusOK
	findings := []core.Finding{}
	if idleAssets.Cmp(module.IdleUrgent) < 0 {
		status = core.MonitorStatusUrgent
		findings = append(findings, module.idleLiquidityFinding(core.SeverityUrgent, idleAssets))
	} else if idleAssets.Cmp(module.IdleWarn) < 0 {
		status = core.MonitorStatusWarn
		findings = append(findings, module.idleLiquidityFinding(core.SeverityWarn, idleAssets))
	}
	if !simulation.Success {
		status = core.MonitorStatusUrgent
		findings = append(findings, core.Finding{
			Key:      core.FindingExitSimulation,
			Severity: core.SeverityUrgent,
			Message:  withdrawLiquidityExitSimulationMessage,
			Evidence: map[string]string{
				withdrawLiquidityExpectedExitEvidenceKey: expectedExitAssets.String(),
				withdrawLiquiditySimulationSuccessKey:    fmt.Sprint(simulation.Success),
			},
		})
	}
	return core.MonitorResult{
		ModuleID:   module.ID(),
		Status:     status,
		ObservedAt: clock.Now(),
		Metrics: []core.Metric{
			{Key: withdrawLiquidityIdleAssetsMetricKey, Value: idleAssets.String(), Unit: withdrawLiquidityMetricUnit},
			{Key: withdrawLiquidityExpectedExitMetricKey, Value: expectedExitAssets.String(), Unit: withdrawLiquidityMetricUnit},
		},
		Findings: findings,
	}, nil
}

func (module WithdrawLiquidityModule) validatePosition(position core.PositionSnapshot) error {
	if position.Vault != module.Vault {
		return fmt.Errorf("%s position vault mismatch", core.ModuleWithdrawLiquidity)
	}
	if position.Owner != module.Owner {
		return fmt.Errorf("%s position owner mismatch", core.ModuleWithdrawLiquidity)
	}
	if position.Receiver != module.Receiver {
		return fmt.Errorf("%s position receiver mismatch", core.ModuleWithdrawLiquidity)
	}
	return nil
}

func (module WithdrawLiquidityModule) idleLiquidityFinding(severity core.Severity, idleAssets *big.Int) core.Finding {
	return core.Finding{
		Key:      core.FindingIdleLiquidity,
		Severity: severity,
		Message:  withdrawLiquidityIdleLiquidityMessage,
		Evidence: map[string]string{
			withdrawLiquidityIdleAssetsEvidenceKey: idleAssets.String(),
			withdrawLiquidityIdleWarnConfigKey:     module.IdleWarn.String(),
			withdrawLiquidityIdleUrgentConfigKey:   module.IdleUrgent.String(),
		},
	}
}
