package morpho

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"withdraw-bot/internal/core"

	"github.com/ethereum/go-ethereum/common"
)

func TestWithdrawLiquidityModuleReturnsUrgentWhenIdleLiquidityIsBelowThreshold(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	shares := big.NewInt(789)
	expectedExitAssets := big.NewInt(456)
	observedAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	simulator := &fakeExitSimulator{
		position: core.PositionSnapshot{
			Vault:        vault,
			Owner:        owner,
			Receiver:     receiver,
			ShareBalance: shares,
		},
		simulation: core.FullExitSimulation{
			Success:            true,
			ExpectedAssetUnits: expectedExitAssets,
			GasUnits:           88000,
		},
	}
	module := WithdrawLiquidityModule{
		IdleAssetReader: fakeIdleAssetReader{assets: big.NewInt(499_999)},
		ExitSimulator:   simulator,
		Owner:           owner,
		Receiver:        receiver,
		Vault:           vault,
		IdleWarn:        big.NewInt(1_000_000),
		IdleUrgent:      big.NewInt(500_000),
		Clock:           core.FixedClock{Value: observedAt},
	}

	// Act
	result, err := module.Monitor(ctx)

	// Assert
	if err != nil {
		t.Fatalf("monitor withdraw liquidity: %v", err)
	}
	if result.Status != core.MonitorStatusUrgent {
		t.Fatalf("expected urgent status, got %s", result.Status)
	}
	if result.ModuleID != core.ModuleWithdrawLiquidity {
		t.Fatalf("expected module %s, got %s", core.ModuleWithdrawLiquidity, result.ModuleID)
	}
	if !result.ObservedAt.Equal(observedAt) {
		t.Fatalf("expected observed time %s, got %s", observedAt, result.ObservedAt)
	}
	if simulator.request.Vault != vault {
		t.Fatalf("expected simulation vault %s, got %s", vault.Hex(), simulator.request.Vault.Hex())
	}
	if simulator.request.Owner != owner {
		t.Fatalf("expected simulation owner %s, got %s", owner.Hex(), simulator.request.Owner.Hex())
	}
	if simulator.request.Receiver != receiver {
		t.Fatalf("expected simulation receiver %s, got %s", receiver.Hex(), simulator.request.Receiver.Hex())
	}
	if simulator.request.Shares.Cmp(shares) != 0 {
		t.Fatalf("expected simulation shares %s, got %s", shares.String(), simulator.request.Shares.String())
	}
	assertMetricValue(t, result, withdrawLiquidityIdleAssetsMetricKey, "499999", withdrawLiquidityMetricUnit)
	assertMetricValue(t, result, withdrawLiquidityExpectedExitMetricKey, expectedExitAssets.String(), withdrawLiquidityMetricUnit)
	assertFinding(t, result, core.FindingIdleLiquidity, core.SeverityUrgent)
}

func TestWithdrawLiquidityModuleReturnsErrorWhenPositionOwnerDoesNotMatch(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	otherOwner := common.HexToAddress("0x0000000000000000000000000000000000000003")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	module := WithdrawLiquidityModule{
		IdleAssetReader: fakeIdleAssetReader{assets: big.NewInt(750_000)},
		ExitSimulator: &fakeExitSimulator{
			position: core.PositionSnapshot{
				Vault:        vault,
				Owner:        otherOwner,
				Receiver:     receiver,
				ShareBalance: big.NewInt(789),
			},
			simulation: core.FullExitSimulation{Success: true, ExpectedAssetUnits: big.NewInt(456)},
		},
		Owner:      owner,
		Receiver:   receiver,
		Vault:      vault,
		IdleWarn:   big.NewInt(1_000_000),
		IdleUrgent: big.NewInt(500_000),
	}

	// Act
	_, err := module.Monitor(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected position owner mismatch error")
	}
}

func TestWithdrawLiquidityModuleReturnsErrorWhenSuccessfulSimulationHasNilExpectedAssets(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	module := WithdrawLiquidityModule{
		IdleAssetReader: fakeIdleAssetReader{assets: big.NewInt(750_000)},
		ExitSimulator: &fakeExitSimulator{
			position: core.PositionSnapshot{
				Vault:        vault,
				Owner:        owner,
				Receiver:     receiver,
				ShareBalance: big.NewInt(789),
			},
			simulation: core.FullExitSimulation{Success: true},
		},
		Owner:      owner,
		Receiver:   receiver,
		Vault:      vault,
		IdleWarn:   big.NewInt(1_000_000),
		IdleUrgent: big.NewInt(500_000),
	}

	// Act
	_, err := module.Monitor(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected missing expected exit assets error")
	}
}

func TestWithdrawLiquidityModuleReturnsWarnWhenIdleLiquidityIsBelowWarnThreshold(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	module := WithdrawLiquidityModule{
		IdleAssetReader: fakeIdleAssetReader{assets: big.NewInt(750_000)},
		ExitSimulator: &fakeExitSimulator{
			position: core.PositionSnapshot{
				Vault:        vault,
				Owner:        owner,
				Receiver:     receiver,
				ShareBalance: big.NewInt(789),
			},
			simulation: core.FullExitSimulation{Success: true, ExpectedAssetUnits: big.NewInt(456)},
		},
		Owner:      owner,
		Receiver:   receiver,
		Vault:      vault,
		IdleWarn:   big.NewInt(1_000_000),
		IdleUrgent: big.NewInt(500_000),
	}

	// Act
	result, err := module.Monitor(ctx)

	// Assert
	if err != nil {
		t.Fatalf("monitor withdraw liquidity: %v", err)
	}
	if result.Status != core.MonitorStatusWarn {
		t.Fatalf("expected warn status, got %s", result.Status)
	}
	assertFinding(t, result, core.FindingIdleLiquidity, core.SeverityWarn)
}

func TestWithdrawLiquidityModuleReturnsOKWhenIdleLiquidityEqualsWarnThreshold(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	module := WithdrawLiquidityModule{
		IdleAssetReader: fakeIdleAssetReader{assets: big.NewInt(1_000_000)},
		ExitSimulator: &fakeExitSimulator{
			position: core.PositionSnapshot{
				Vault:        vault,
				Owner:        owner,
				Receiver:     receiver,
				ShareBalance: big.NewInt(789),
			},
			simulation: core.FullExitSimulation{Success: true, ExpectedAssetUnits: big.NewInt(456)},
		},
		Owner:      owner,
		Receiver:   receiver,
		Vault:      vault,
		IdleWarn:   big.NewInt(1_000_000),
		IdleUrgent: big.NewInt(500_000),
	}

	// Act
	result, err := module.Monitor(ctx)

	// Assert
	if err != nil {
		t.Fatalf("monitor withdraw liquidity: %v", err)
	}
	if result.Status != core.MonitorStatusOK {
		t.Fatalf("expected ok status, got %s", result.Status)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected no findings, got %d", len(result.Findings))
	}
}

func TestWithdrawLiquidityModuleReturnsWarnWhenIdleLiquidityEqualsUrgentThreshold(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	module := WithdrawLiquidityModule{
		IdleAssetReader: fakeIdleAssetReader{assets: big.NewInt(500_000)},
		ExitSimulator: &fakeExitSimulator{
			position: core.PositionSnapshot{
				Vault:        vault,
				Owner:        owner,
				Receiver:     receiver,
				ShareBalance: big.NewInt(789),
			},
			simulation: core.FullExitSimulation{Success: true, ExpectedAssetUnits: big.NewInt(456)},
		},
		Owner:      owner,
		Receiver:   receiver,
		Vault:      vault,
		IdleWarn:   big.NewInt(1_000_000),
		IdleUrgent: big.NewInt(500_000),
	}

	// Act
	result, err := module.Monitor(ctx)

	// Assert
	if err != nil {
		t.Fatalf("monitor withdraw liquidity: %v", err)
	}
	if result.Status != core.MonitorStatusWarn {
		t.Fatalf("expected warn status, got %s", result.Status)
	}
	assertFinding(t, result, core.FindingIdleLiquidity, core.SeverityWarn)
}

func TestWithdrawLiquidityModuleReturnsUrgentWhenExitSimulationFails(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	module := WithdrawLiquidityModule{
		IdleAssetReader: fakeIdleAssetReader{assets: big.NewInt(1_500_000)},
		ExitSimulator: &fakeExitSimulator{
			position: core.PositionSnapshot{
				Vault:        vault,
				Owner:        owner,
				Receiver:     receiver,
				ShareBalance: big.NewInt(789),
			},
			simulation: core.FullExitSimulation{Success: false, ExpectedAssetUnits: big.NewInt(456)},
		},
		Owner:      owner,
		Receiver:   receiver,
		Vault:      vault,
		IdleWarn:   big.NewInt(1_000_000),
		IdleUrgent: big.NewInt(500_000),
	}

	// Act
	result, err := module.Monitor(ctx)

	// Assert
	if err != nil {
		t.Fatalf("monitor withdraw liquidity: %v", err)
	}
	if result.Status != core.MonitorStatusUrgent {
		t.Fatalf("expected urgent status, got %s", result.Status)
	}
	assertFinding(t, result, core.FindingExitSimulation, core.SeverityUrgent)
}

func TestWithdrawLiquidityModuleReturnsUrgentWhenExitSimulatorReturnsError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	module := WithdrawLiquidityModule{
		IdleAssetReader: fakeIdleAssetReader{assets: big.NewInt(1_500_000)},
		ExitSimulator: &fakeExitSimulator{
			position: core.PositionSnapshot{
				Vault:        vault,
				Owner:        owner,
				Receiver:     receiver,
				ShareBalance: big.NewInt(789),
			},
			simulationErr: errors.New("simulation reverted with provider detail"),
		},
		Owner:      owner,
		Receiver:   receiver,
		Vault:      vault,
		IdleWarn:   big.NewInt(1_000_000),
		IdleUrgent: big.NewInt(500_000),
	}

	// Act
	result, err := module.Monitor(ctx)

	// Assert
	if err != nil {
		t.Fatalf("monitor withdraw liquidity: %v", err)
	}
	if result.Status != core.MonitorStatusUrgent {
		t.Fatalf("expected urgent status, got %s", result.Status)
	}
	assertFinding(t, result, core.FindingExitSimulation, core.SeverityUrgent)
	for _, finding := range result.Findings {
		if finding.Key != core.FindingExitSimulation {
			continue
		}
		for _, value := range finding.Evidence {
			if value == "simulation reverted with provider detail" {
				t.Fatalf("expected simulation error detail to stay out of evidence")
			}
		}
	}
}

func assertMetricValue(t *testing.T, result core.MonitorResult, key string, value string, unit string) {
	t.Helper()
	for _, metric := range result.Metrics {
		if metric.Key != key {
			continue
		}
		if metric.Value != value {
			t.Fatalf("expected metric %s value %s, got %s", key, value, metric.Value)
		}
		if metric.Unit != unit {
			t.Fatalf("expected metric %s unit %s, got %s", key, unit, metric.Unit)
		}
		return
	}
	t.Fatalf("expected metric %s", key)
}

func assertFinding(t *testing.T, result core.MonitorResult, key core.FindingKey, severity core.Severity) {
	t.Helper()
	for _, finding := range result.Findings {
		if finding.Key == key && finding.Severity == severity {
			return
		}
	}
	t.Fatalf("expected finding %s with severity %s", key, severity)
}

type fakeIdleAssetReader struct {
	assets *big.Int
	err    error
}

func (reader fakeIdleAssetReader) IdleAssets(ctx context.Context, vault common.Address) (*big.Int, error) {
	if reader.err != nil {
		return nil, reader.err
	}
	if reader.assets == nil {
		return nil, nil
	}
	return new(big.Int).Set(reader.assets), nil
}

type fakeExitSimulator struct {
	position      core.PositionSnapshot
	simulation    core.FullExitSimulation
	request       core.FullExitRequest
	positionErr   error
	simulationErr error
}

func (simulator *fakeExitSimulator) Position(ctx context.Context) (core.PositionSnapshot, error) {
	if simulator.positionErr != nil {
		return core.PositionSnapshot{}, simulator.positionErr
	}
	result := simulator.position
	if result.ShareBalance != nil {
		result.ShareBalance = new(big.Int).Set(result.ShareBalance)
	}
	return result, nil
}

func (simulator *fakeExitSimulator) SimulateFullExit(ctx context.Context, req core.FullExitRequest) (core.FullExitSimulation, error) {
	simulator.request = req
	if simulator.simulationErr != nil {
		return core.FullExitSimulation{}, simulator.simulationErr
	}
	result := simulator.simulation
	if result.ExpectedAssetUnits != nil {
		result.ExpectedAssetUnits = new(big.Int).Set(result.ExpectedAssetUnits)
	}
	return result, nil
}
