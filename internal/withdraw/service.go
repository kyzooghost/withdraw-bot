package withdraw

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/signer"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	TriggerKindUrgent TriggerKind = "urgent"

	WithdrawalStatusNoop             WithdrawalStatus = "noop"
	WithdrawalStatusSimulationFailed WithdrawalStatus = "simulation_failed"
	WithdrawalStatusSimulated        WithdrawalStatus = "simulated"
	WithdrawalStatusSubmitted        WithdrawalStatus = "submitted"

	withdrawalAttemptIDTimeFormat = "20060102T150405.000000000Z0700"
)

var ErrSimulationFailed = errors.New("full-exit simulation failed")

type Adapter interface {
	ID() string
	Position(ctx context.Context) (core.PositionSnapshot, error)
	BuildFullExit(ctx context.Context, req core.FullExitRequest) (core.TxCandidate, error)
	SimulateFullExit(ctx context.Context, req core.FullExitRequest) (core.FullExitSimulation, error)
}

type TransactionSubmitter interface {
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
}

type AttemptRepository interface {
	InsertWithdrawalAttempt(ctx context.Context, attempt WithdrawalAttempt) error
}

type TriggerKind string
type WithdrawalStatus string

type WithdrawalTrigger struct {
	Kind       TriggerKind
	ModuleID   core.MonitorModuleID
	FindingKey core.FindingKey
}

type WithdrawalResult struct {
	Status             WithdrawalStatus
	Noop               bool
	TxHash             common.Hash
	Nonce              uint64
	GasUnits           uint64
	FeeCaps            FeeCaps
	ExpectedAssetUnits *big.Int
}

type WithdrawalAttempt struct {
	ID                 string
	Trigger            WithdrawalTrigger
	Status             WithdrawalStatus
	TxHash             common.Hash
	Nonce              uint64
	GasUnits           uint64
	FeeCaps            FeeCaps
	ExpectedAssetUnits *big.Int
	SimulationSuccess  bool
	FailureReason      string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Service struct {
	Adapter            Adapter
	Signer             signer.Service
	ChainID            *big.Int
	Clock              core.Clock
	Submitter          TransactionSubmitter
	Repository         AttemptRepository
	GasPolicy          GasPolicy
	GasLimitBufferBPS  int64
	ReplacementTimeout time.Duration

	pending *pendingWithdrawal
}

type pendingWithdrawal struct {
	Nonce       uint64
	FeeCaps     FeeCaps
	SubmittedAt time.Time
	TxHash      common.Hash
}

func (service *Service) DryRunFullExit(ctx context.Context) (WithdrawalResult, error) {
	if service.Adapter == nil {
		return WithdrawalResult{}, errors.New("withdraw adapter is required")
	}
	position, err := service.Adapter.Position(ctx)
	if err != nil {
		return WithdrawalResult{}, err
	}
	if position.ShareBalance == nil || position.ShareBalance.Sign() == 0 {
		return WithdrawalResult{Status: WithdrawalStatusNoop, Noop: true}, nil
	}
	simulation, err := service.Adapter.SimulateFullExit(ctx, fullExitRequest(position))
	if err != nil {
		return WithdrawalResult{}, err
	}
	return WithdrawalResult{
		Status:             statusForSimulation(simulation),
		ExpectedAssetUnits: cloneBigInt(simulation.ExpectedAssetUnits),
		GasUnits:           simulation.GasUnits,
	}, nil
}

func (service *Service) ExecuteFullExit(ctx context.Context, trigger WithdrawalTrigger) (WithdrawalResult, error) {
	if err := service.validateExecuteConfig(); err != nil {
		return WithdrawalResult{}, err
	}
	now := service.now()
	position, err := service.Adapter.Position(ctx)
	if err != nil {
		return WithdrawalResult{}, err
	}
	if position.ShareBalance == nil || position.ShareBalance.Sign() == 0 {
		result := WithdrawalResult{Status: WithdrawalStatusNoop, Noop: true}
		if err := service.recordAttempt(ctx, trigger, result, core.FullExitSimulation{}, "", now); err != nil {
			return WithdrawalResult{}, err
		}
		service.pending = nil
		return result, nil
	}
	if position.ShareBalance.Sign() < 0 {
		return WithdrawalResult{}, errors.New("share balance must be non-negative")
	}

	request := fullExitRequest(position)
	simulation, err := service.Adapter.SimulateFullExit(ctx, request)
	if err != nil || !simulation.Success {
		result := WithdrawalResult{
			Status:             WithdrawalStatusSimulationFailed,
			ExpectedAssetUnits: cloneBigInt(simulation.ExpectedAssetUnits),
			GasUnits:           simulation.GasUnits,
		}
		if err := service.recordAttempt(ctx, trigger, result, simulation, ErrSimulationFailed.Error(), now); err != nil {
			return WithdrawalResult{}, err
		}
		return result, ErrSimulationFailed
	}
	if service.pending != nil && service.ReplacementTimeout > 0 && now.Sub(service.pending.SubmittedAt) < service.ReplacementTimeout {
		return WithdrawalResult{
			Status:             WithdrawalStatusSubmitted,
			TxHash:             service.pending.TxHash,
			Nonce:              service.pending.Nonce,
			GasUnits:           simulation.GasUnits,
			FeeCaps:            service.pending.FeeCaps.Clone(),
			ExpectedAssetUnits: cloneBigInt(simulation.ExpectedAssetUnits),
		}, nil
	}

	candidate, err := service.Adapter.BuildFullExit(ctx, request)
	if err != nil {
		return WithdrawalResult{}, err
	}
	nonce, feeCaps, err := service.nonceAndFeeCaps(ctx, now)
	if err != nil {
		return WithdrawalResult{}, err
	}
	tx := buildDynamicFeeTx(candidate, service.ChainID, nonce, bufferedGas(simulation.GasUnits, service.GasLimitBufferBPS), feeCaps)
	signed, err := service.Signer.SignTransaction(ctx, tx, service.ChainID)
	if err != nil {
		return WithdrawalResult{}, fmt.Errorf("sign full-exit transaction: %w", err)
	}
	if err := service.Submitter.SendTransaction(ctx, signed); err != nil {
		return WithdrawalResult{}, fmt.Errorf("submit full-exit transaction: %w", err)
	}
	result := WithdrawalResult{
		Status:             WithdrawalStatusSubmitted,
		TxHash:             signed.Hash(),
		Nonce:              nonce,
		GasUnits:           tx.Gas(),
		FeeCaps:            feeCaps.Clone(),
		ExpectedAssetUnits: cloneBigInt(simulation.ExpectedAssetUnits),
	}
	service.pending = &pendingWithdrawal{Nonce: nonce, FeeCaps: feeCaps.Clone(), SubmittedAt: now, TxHash: signed.Hash()}
	if err := service.recordAttempt(ctx, trigger, result, simulation, "", now); err != nil {
		return WithdrawalResult{}, err
	}
	return result, nil
}

func (service *Service) validateExecuteConfig() error {
	if service.Adapter == nil {
		return errors.New("withdraw adapter is required")
	}
	if service.Signer == nil {
		return errors.New("withdraw signer is required")
	}
	if service.Submitter == nil {
		return errors.New("withdraw submitter is required")
	}
	if service.Repository == nil {
		return errors.New("withdraw repository is required")
	}
	if service.ChainID == nil {
		return errors.New("chain ID is required")
	}
	return nil
}

func (service *Service) nonceAndFeeCaps(ctx context.Context, now time.Time) (uint64, FeeCaps, error) {
	if service.pending != nil {
		if service.ReplacementTimeout <= 0 || now.Sub(service.pending.SubmittedAt) >= service.ReplacementTimeout {
			return service.pending.Nonce, service.GasPolicy.Bump(service.pending.FeeCaps), nil
		}
		return service.pending.Nonce, service.pending.FeeCaps.Clone(), nil
	}
	address, err := service.Signer.Address(ctx)
	if err != nil {
		return 0, FeeCaps{}, fmt.Errorf("read signer address: %w", err)
	}
	nonce, err := service.Submitter.PendingNonceAt(ctx, address)
	if err != nil {
		return 0, FeeCaps{}, fmt.Errorf("read pending nonce: %w", err)
	}
	feeCaps, err := service.initialFeeCaps(ctx)
	if err != nil {
		return 0, FeeCaps{}, err
	}
	return nonce, feeCaps, nil
}

func (service *Service) initialFeeCaps(ctx context.Context) (FeeCaps, error) {
	tipCap, err := service.Submitter.SuggestGasTipCap(ctx)
	if err != nil {
		return FeeCaps{}, fmt.Errorf("suggest gas tip cap: %w", err)
	}
	if tipCap == nil || tipCap.Sign() <= 0 {
		return FeeCaps{}, errors.New("suggested gas tip cap must be positive")
	}
	maxFee := new(big.Int).Mul(tipCap, big.NewInt(2))
	return FeeCaps{
		MaxFeePerGas:         capBig(maxFee, service.GasPolicy.MaxFeeCap),
		MaxPriorityFeePerGas: capBig(tipCap, service.GasPolicy.MaxTipCap),
	}, nil
}

func (service *Service) recordAttempt(ctx context.Context, trigger WithdrawalTrigger, result WithdrawalResult, simulation core.FullExitSimulation, failureReason string, now time.Time) error {
	return service.Repository.InsertWithdrawalAttempt(ctx, WithdrawalAttempt{
		ID:                 withdrawalAttemptID(now, trigger),
		Trigger:            trigger,
		Status:             result.Status,
		TxHash:             result.TxHash,
		Nonce:              result.Nonce,
		GasUnits:           result.GasUnits,
		FeeCaps:            result.FeeCaps.Clone(),
		ExpectedAssetUnits: cloneBigInt(result.ExpectedAssetUnits),
		SimulationSuccess:  simulation.Success,
		FailureReason:      failureReason,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
}

func (service *Service) now() time.Time {
	if service.Clock == nil {
		return core.SystemClock{}.Now()
	}
	return service.Clock.Now()
}

func fullExitRequest(position core.PositionSnapshot) core.FullExitRequest {
	return core.FullExitRequest{
		Vault:    position.Vault,
		Owner:    position.Owner,
		Receiver: position.Receiver,
		Shares:   cloneBigInt(position.ShareBalance),
	}
}

func buildDynamicFeeTx(candidate core.TxCandidate, chainID *big.Int, nonce uint64, gas uint64, fees FeeCaps) *types.Transaction {
	value := candidate.Value
	if value == nil {
		value = big.NewInt(0)
	}
	return types.NewTx(&types.DynamicFeeTx{
		ChainID:   new(big.Int).Set(chainID),
		Nonce:     nonce,
		GasTipCap: cloneBigInt(fees.MaxPriorityFeePerGas),
		GasFeeCap: cloneBigInt(fees.MaxFeePerGas),
		Gas:       gas,
		To:        &candidate.To,
		Value:     cloneBigInt(value),
		Data:      cloneBytes(candidate.Data),
	})
}

func bufferedGas(gas uint64, bufferBPS int64) uint64 {
	if bufferBPS <= 0 {
		return gas
	}
	buffered := new(big.Int).Mul(new(big.Int).SetUint64(gas), big.NewInt(10_000+bufferBPS))
	buffered.Div(buffered, big.NewInt(10_000))
	return buffered.Uint64()
}

func statusForSimulation(simulation core.FullExitSimulation) WithdrawalStatus {
	if simulation.Success {
		return WithdrawalStatusSimulated
	}
	return WithdrawalStatusSimulationFailed
}

func withdrawalAttemptID(now time.Time, trigger WithdrawalTrigger) string {
	return fmt.Sprintf("%s-%s-%s-%s", now.UTC().Format(withdrawalAttemptIDTimeFormat), trigger.Kind, trigger.ModuleID, trigger.FindingKey)
}

func cloneBigInt(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	clone := make([]byte, len(value))
	copy(clone, value)
	return clone
}

func (fees FeeCaps) Clone() FeeCaps {
	return FeeCaps{
		MaxFeePerGas:         cloneBigInt(fees.MaxFeePerGas),
		MaxPriorityFeePerGas: cloneBigInt(fees.MaxPriorityFeePerGas),
	}
}
