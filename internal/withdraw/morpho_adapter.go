package withdraw

import (
	"context"
	"math/big"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/ethereum"
	"withdraw-bot/internal/morpho"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

const MorphoVaultV2AdapterID = "morpho_vault_v2"

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
	shares, err := adapter.VaultClient.BalanceOf(ctx, adapter.Owner)
	if err != nil {
		return core.PositionSnapshot{}, err
	}
	return core.PositionSnapshot{
		Vault:         adapter.Vault,
		Owner:         adapter.Owner,
		Receiver:      adapter.Receiver,
		ShareBalance:  shares,
		AssetSymbol:   adapter.AssetSymbol,
		AssetDecimals: adapter.AssetDecimals,
		ObservedAt:    adapter.Clock.Now(),
	}, nil
}

func (adapter MorphoAdapter) BuildFullExit(ctx context.Context, req core.FullExitRequest) (core.TxCandidate, error) {
	data, err := morpho.PackRedeem(req.Shares, req.Receiver, req.Owner)
	if err != nil {
		return core.TxCandidate{}, err
	}
	return core.TxCandidate{To: req.Vault, Data: data, Value: big.NewInt(0)}, nil
}

func (adapter MorphoAdapter) SimulateFullExit(ctx context.Context, req core.FullExitRequest) (core.FullExitSimulation, error) {
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
		return core.FullExitSimulation{Success: false, ExpectedAssetUnits: expected, RevertReason: err.Error()}, nil
	}
	return core.FullExitSimulation{Success: true, ExpectedAssetUnits: expected, GasUnits: gas}, nil
}
