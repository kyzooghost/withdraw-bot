package withdraw

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/ethereum"
	"withdraw-bot/internal/morpho"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

const MorphoVaultV2AdapterID = "morpho_vault_v2"

const simulationFailedReason = "simulation failed"

var (
	errZeroVault           = errors.New("vault address is required")
	errVaultClientMismatch = errors.New("vault client mismatch")
	errZeroOwner           = errors.New("owner address is required")
	errZeroReceiver        = errors.New("receiver address is required")
	errInvalidShares       = errors.New("shares must be positive")
)

type MorphoAdapter struct {
	Ethereum      ethereum.MultiClient
	VaultClient   morpho.VaultClient
	Vault         common.Address
	Owner         common.Address
	Receiver      common.Address
	AssetSymbol   string
	AssetDecimals uint8
	Clock         core.Clock
}

func (adapter MorphoAdapter) ID() string {
	return MorphoVaultV2AdapterID
}

func (adapter MorphoAdapter) Position(ctx context.Context) (core.PositionSnapshot, error) {
	if err := adapter.validateVaultClient(); err != nil {
		return core.PositionSnapshot{}, err
	}
	shares, err := adapter.VaultClient.BalanceOf(ctx, adapter.Owner)
	if err != nil {
		return core.PositionSnapshot{}, err
	}
	clock := adapter.Clock
	if clock == nil {
		clock = core.SystemClock{}
	}
	return core.PositionSnapshot{
		Vault:         adapter.Vault,
		Owner:         adapter.Owner,
		Receiver:      adapter.Receiver,
		ShareBalance:  new(big.Int).Set(shares),
		AssetSymbol:   adapter.AssetSymbol,
		AssetDecimals: adapter.AssetDecimals,
		ObservedAt:    clock.Now(),
	}, nil
}

func (adapter MorphoAdapter) BuildFullExit(ctx context.Context, req core.FullExitRequest) (core.TxCandidate, error) {
	if err := adapter.validateFullExitRequest(req); err != nil {
		return core.TxCandidate{}, err
	}
	data, err := morpho.PackRedeem(req.Shares, req.Receiver, req.Owner)
	if err != nil {
		return core.TxCandidate{}, err
	}
	return core.TxCandidate{To: req.Vault, Data: data, Value: big.NewInt(0)}, nil
}

func (adapter MorphoAdapter) SimulateFullExit(ctx context.Context, req core.FullExitRequest) (core.FullExitSimulation, error) {
	if err := adapter.validateFullExitRequest(req); err != nil {
		return core.FullExitSimulation{}, err
	}
	expected, err := adapter.VaultClient.PreviewRedeem(ctx, req.Shares)
	if err != nil {
		return core.FullExitSimulation{}, err
	}
	candidate, err := adapter.BuildFullExit(ctx, req)
	if err != nil {
		return core.FullExitSimulation{}, err
	}
	call := geth.CallMsg{From: req.Owner, To: &candidate.To, Value: candidate.Value, Data: candidate.Data}
	gas, err := adapter.Ethereum.EstimateGas(ctx, call)
	if err != nil {
		return core.FullExitSimulation{Success: false, ExpectedAssetUnits: new(big.Int).Set(expected), RevertReason: simulationFailedReason}, nil
	}
	return core.FullExitSimulation{Success: true, ExpectedAssetUnits: new(big.Int).Set(expected), GasUnits: gas}, nil
}

func (adapter MorphoAdapter) validateVaultClient() error {
	if adapter.Vault == (common.Address{}) {
		return errZeroVault
	}
	if adapter.VaultClient.Vault != adapter.Vault {
		return errVaultClientMismatch
	}
	return nil
}

func (adapter MorphoAdapter) validateFullExitRequest(req core.FullExitRequest) error {
	if err := adapter.validateVaultClient(); err != nil {
		return err
	}
	if req.Vault == (common.Address{}) {
		return errZeroVault
	}
	if req.Vault != adapter.Vault {
		return fmt.Errorf("request vault mismatch: %w", errVaultClientMismatch)
	}
	if req.Owner == (common.Address{}) {
		return errZeroOwner
	}
	if req.Receiver == (common.Address{}) {
		return errZeroReceiver
	}
	if req.Shares == nil || req.Shares.Sign() <= 0 {
		return errInvalidShares
	}
	return nil
}
