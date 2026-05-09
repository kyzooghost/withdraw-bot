package withdraw

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"withdraw-bot/internal/core"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestDryRunFullExitReturnsNoopWhenSharesAreZero(t *testing.T) {
	// Arrange
	ctx := context.Background()
	adapter := &fakeWithdrawAdapter{
		position: core.PositionSnapshot{ShareBalance: big.NewInt(0)},
	}
	service := Service{
		Adapter:   adapter,
		Signer:    &fakeSigner{},
		Submitter: &fakeSubmitter{nonce: 7, gasPrice: big.NewInt(100), tipCap: big.NewInt(8)},
		Clock:     mutableClock{value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	result, err := service.DryRunFullExit(ctx)

	// Assert
	if err != nil {
		t.Fatalf("dry run full exit: %v", err)
	}
	if result.Status != WithdrawalStatusNoop {
		t.Fatalf("expected noop status, got %s", result.Status)
	}
	if !result.Noop {
		t.Fatal("expected noop result")
	}
	if adapter.simulateCalls != 0 {
		t.Fatalf("expected no simulation calls, got %d", adapter.simulateCalls)
	}
}

func TestExecuteFullExitDoesNotSignWhenSimulationFails(t *testing.T) {
	// Arrange
	ctx := context.Background()
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	adapter := &fakeWithdrawAdapter{
		position: core.PositionSnapshot{Vault: vault, Owner: owner, Receiver: receiver, ShareBalance: big.NewInt(789)},
		simulation: core.FullExitSimulation{
			Success:            false,
			ExpectedAssetUnits: big.NewInt(456),
			RevertReason:       simulationFailedReason,
		},
		candidate: core.TxCandidate{To: vault, Value: big.NewInt(0), Data: []byte{0x01}},
	}
	signer := &fakeSigner{address: owner}
	submitter := &fakeSubmitter{nonce: 7, gasPrice: big.NewInt(100), tipCap: big.NewInt(8)}
	repo := &fakeWithdrawalRepository{}
	service := Service{
		Adapter:    adapter,
		Signer:     signer,
		Submitter:  submitter,
		Repository: repo,
		ChainID:    big.NewInt(1),
		Clock:      mutableClock{value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	_, err := service.ExecuteFullExit(ctx, urgentTrigger())

	// Assert
	if !errors.Is(err, ErrSimulationFailed) {
		t.Fatalf("expected simulation failed error, got %v", err)
	}
	if signer.signCalls != 0 {
		t.Fatalf("expected no signing, got %d sign call(s)", signer.signCalls)
	}
	if len(submitter.sent) != 0 {
		t.Fatalf("expected no submitted transaction, got %d", len(submitter.sent))
	}
	if len(repo.attempts) != 1 {
		t.Fatalf("expected one stored attempt, got %d", len(repo.attempts))
	}
	if repo.attempts[0].Status != WithdrawalStatusSimulationFailed {
		t.Fatalf("expected simulation failed status, got %s", repo.attempts[0].Status)
	}
	if repo.attempts[0].SimulationSuccess {
		t.Fatal("expected failed simulation to be stored")
	}
}

func TestUrgentReplacementBumpsPendingTransactionFeesAfterTimeout(t *testing.T) {
	// Arrange
	ctx := context.Background()
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	adapter := &fakeWithdrawAdapter{
		position: core.PositionSnapshot{Vault: vault, Owner: owner, Receiver: receiver, ShareBalance: big.NewInt(789)},
		simulation: core.FullExitSimulation{
			Success:            true,
			ExpectedAssetUnits: big.NewInt(456),
			GasUnits:           100_000,
		},
		candidate: core.TxCandidate{To: vault, Value: big.NewInt(0), Data: []byte{0x01}},
	}
	clock := &mutableClock{value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)}
	submitter := &fakeSubmitter{nonce: 7, gasPrice: big.NewInt(100), tipCap: big.NewInt(8)}
	service := Service{
		Adapter:            adapter,
		Signer:             &fakeSigner{address: owner},
		Submitter:          submitter,
		Repository:         &fakeWithdrawalRepository{},
		ChainID:            big.NewInt(1),
		Clock:              clock,
		GasPolicy:          GasPolicy{BumpBPS: 1250, MaxFeeCap: big.NewInt(112), MaxTipCap: big.NewInt(10)},
		GasLimitBufferBPS:  2000,
		ReplacementTimeout: 2 * time.Minute,
	}

	// Act
	_, err := service.ExecuteFullExit(ctx, urgentTrigger())
	if err != nil {
		t.Fatalf("execute full exit: %v", err)
	}
	clock.value = clock.value.Add(3 * time.Minute)
	_, err = service.ExecuteFullExit(ctx, urgentTrigger())

	// Assert
	if err != nil {
		t.Fatalf("replace full exit: %v", err)
	}
	if len(submitter.sent) != 2 {
		t.Fatalf("expected two submitted transactions, got %d", len(submitter.sent))
	}
	first := submitter.sent[0]
	second := submitter.sent[1]
	if second.Nonce() != first.Nonce() {
		t.Fatalf("expected replacement nonce %d, got %d", first.Nonce(), second.Nonce())
	}
	if second.GasTipCap().String() != "9" {
		t.Fatalf("expected bumped priority fee 9, got %s", second.GasTipCap().String())
	}
	if second.GasFeeCap().Cmp(first.GasFeeCap()) <= 0 {
		t.Fatalf("expected bumped max fee above %s, got %s", first.GasFeeCap().String(), second.GasFeeCap().String())
	}
}

func TestUrgentReplacementDoesNotSubmitAgainBeforeTimeout(t *testing.T) {
	// Arrange
	ctx := context.Background()
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	adapter := &fakeWithdrawAdapter{
		position: core.PositionSnapshot{Vault: vault, Owner: owner, Receiver: receiver, ShareBalance: big.NewInt(789)},
		simulation: core.FullExitSimulation{
			Success:            true,
			ExpectedAssetUnits: big.NewInt(456),
			GasUnits:           100_000,
		},
		candidate: core.TxCandidate{To: vault, Value: big.NewInt(0), Data: []byte{0x01}},
	}
	clock := &mutableClock{value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)}
	submitter := &fakeSubmitter{nonce: 7, gasPrice: big.NewInt(100), tipCap: big.NewInt(8)}
	service := Service{
		Adapter:            adapter,
		Signer:             &fakeSigner{address: owner},
		Submitter:          submitter,
		Repository:         &fakeWithdrawalRepository{},
		ChainID:            big.NewInt(1),
		Clock:              clock,
		GasPolicy:          GasPolicy{BumpBPS: 1250, MaxFeeCap: big.NewInt(112), MaxTipCap: big.NewInt(10)},
		GasLimitBufferBPS:  2000,
		ReplacementTimeout: 2 * time.Minute,
	}

	// Act
	_, err := service.ExecuteFullExit(ctx, urgentTrigger())
	if err != nil {
		t.Fatalf("execute full exit: %v", err)
	}
	clock.value = clock.value.Add(time.Minute)
	_, err = service.ExecuteFullExit(ctx, urgentTrigger())

	// Assert
	if err != nil {
		t.Fatalf("handle pending full exit: %v", err)
	}
	if len(submitter.sent) != 1 {
		t.Fatalf("expected one submitted transaction before timeout, got %d", len(submitter.sent))
	}
}

func TestExecuteFullExitUsesSuggestedGasPriceForMaxFee(t *testing.T) {
	// Arrange
	ctx := context.Background()
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	adapter := &fakeWithdrawAdapter{
		position: core.PositionSnapshot{Vault: vault, Owner: owner, Receiver: receiver, ShareBalance: big.NewInt(789)},
		simulation: core.FullExitSimulation{
			Success:            true,
			ExpectedAssetUnits: big.NewInt(456),
			GasUnits:           100_000,
		},
		candidate: core.TxCandidate{To: vault, Value: big.NewInt(0), Data: []byte{0x01}},
	}
	submitter := &fakeSubmitter{nonce: 7, gasPrice: big.NewInt(100), tipCap: big.NewInt(8)}
	service := Service{
		Adapter:           adapter,
		Signer:            &fakeSigner{address: owner},
		Submitter:         submitter,
		Repository:        &fakeWithdrawalRepository{},
		ChainID:           big.NewInt(1),
		Clock:             mutableClock{value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
		GasLimitBufferBPS: 2000,
	}

	// Act
	_, err := service.ExecuteFullExit(ctx, urgentTrigger())

	// Assert
	if err != nil {
		t.Fatalf("execute full exit: %v", err)
	}
	if len(submitter.sent) != 1 {
		t.Fatalf("expected one submitted transaction, got %d", len(submitter.sent))
	}
	if submitter.sent[0].GasFeeCap().String() != "108" {
		t.Fatalf("expected max fee to include suggested gas price and tip, got %s", submitter.sent[0].GasFeeCap().String())
	}
}

func TestExecuteFullExitRejectsSignerOwnerMismatch(t *testing.T) {
	// Arrange
	ctx := context.Background()
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	adapter := &fakeWithdrawAdapter{
		position: core.PositionSnapshot{Vault: vault, Owner: owner, Receiver: receiver, ShareBalance: big.NewInt(789)},
		simulation: core.FullExitSimulation{
			Success:            true,
			ExpectedAssetUnits: big.NewInt(456),
			GasUnits:           100_000,
		},
		candidate: core.TxCandidate{To: vault, Value: big.NewInt(0), Data: []byte{0x01}},
	}
	signer := &fakeSigner{address: common.HexToAddress("0x0000000000000000000000000000000000000003")}
	service := Service{
		Adapter:    adapter,
		Signer:     signer,
		Submitter:  &fakeSubmitter{nonce: 7, gasPrice: big.NewInt(100), tipCap: big.NewInt(8)},
		Repository: &fakeWithdrawalRepository{},
		ChainID:    big.NewInt(1),
		Clock:      mutableClock{value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	_, err := service.ExecuteFullExit(ctx, urgentTrigger())

	// Assert
	if !errors.Is(err, ErrSignerOwnerMismatch) {
		t.Fatalf("expected signer owner mismatch, got %v", err)
	}
	if adapter.simulateCalls != 0 {
		t.Fatalf("expected no simulation calls, got %d", adapter.simulateCalls)
	}
	if signer.signCalls != 0 {
		t.Fatalf("expected no signing, got %d sign call(s)", signer.signCalls)
	}
}

func TestExecuteFullExitRejectsInvalidFeeCapOrdering(t *testing.T) {
	// Arrange
	ctx := context.Background()
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	adapter := &fakeWithdrawAdapter{
		position: core.PositionSnapshot{Vault: vault, Owner: owner, Receiver: receiver, ShareBalance: big.NewInt(789)},
		simulation: core.FullExitSimulation{
			Success:            true,
			ExpectedAssetUnits: big.NewInt(456),
			GasUnits:           100_000,
		},
		candidate: core.TxCandidate{To: vault, Value: big.NewInt(0), Data: []byte{0x01}},
	}
	signer := &fakeSigner{address: owner}
	submitter := &fakeSubmitter{nonce: 7, gasPrice: big.NewInt(100), tipCap: big.NewInt(8)}
	service := Service{
		Adapter:    adapter,
		Signer:     signer,
		Submitter:  submitter,
		Repository: &fakeWithdrawalRepository{},
		ChainID:    big.NewInt(1),
		Clock:      mutableClock{value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
		GasPolicy:  GasPolicy{MaxFeeCap: big.NewInt(5), MaxTipCap: big.NewInt(8)},
	}

	// Act
	_, err := service.ExecuteFullExit(ctx, urgentTrigger())

	// Assert
	if err == nil {
		t.Fatal("expected invalid fee cap ordering error")
	}
	if signer.signCalls != 0 {
		t.Fatalf("expected no signing, got %d sign call(s)", signer.signCalls)
	}
	if len(submitter.sent) != 0 {
		t.Fatalf("expected no submitted transaction, got %d", len(submitter.sent))
	}
}

func TestExecuteFullExitIsConcurrencySafeForPendingWithdrawal(t *testing.T) {
	// Arrange
	ctx := context.Background()
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	adapter := &fakeWithdrawAdapter{
		position: core.PositionSnapshot{Vault: vault, Owner: owner, Receiver: receiver, ShareBalance: big.NewInt(789)},
		simulation: core.FullExitSimulation{
			Success:            true,
			ExpectedAssetUnits: big.NewInt(456),
			GasUnits:           100_000,
		},
		candidate: core.TxCandidate{To: vault, Value: big.NewInt(0), Data: []byte{0x01}},
	}
	releaseSend := make(chan struct{})
	sendStarted := make(chan struct{}, 2)
	submitter := &fakeSubmitter{
		nonce:       7,
		gasPrice:    big.NewInt(100),
		tipCap:      big.NewInt(8),
		releaseSend: releaseSend,
		sendStarted: sendStarted,
	}
	service := Service{
		Adapter:            adapter,
		Signer:             &fakeSigner{address: owner},
		Submitter:          submitter,
		Repository:         &fakeWithdrawalRepository{},
		ChainID:            big.NewInt(1),
		Clock:              mutableClock{value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
		GasLimitBufferBPS:  2000,
		ReplacementTimeout: 2 * time.Minute,
	}
	errCh := make(chan error, 2)

	// Act
	go func() {
		_, err := service.ExecuteFullExit(ctx, urgentTrigger())
		errCh <- err
	}()
	<-sendStarted
	go func() {
		_, err := service.ExecuteFullExit(ctx, urgentTrigger())
		errCh <- err
	}()

	// Assert
	select {
	case <-sendStarted:
		t.Fatal("expected second call to wait for pending state")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseSend)
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("execute full exit: %v", err)
		}
	}
	if len(submitter.sent) != 1 {
		t.Fatalf("expected one submitted transaction, got %d", len(submitter.sent))
	}
}

func TestExecuteFullExitKeepsPendingNonceWhenRepositoryFailsAfterSubmit(t *testing.T) {
	// Arrange
	ctx := context.Background()
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	adapter := &fakeWithdrawAdapter{
		position: core.PositionSnapshot{Vault: vault, Owner: owner, Receiver: receiver, ShareBalance: big.NewInt(789)},
		simulation: core.FullExitSimulation{
			Success:            true,
			ExpectedAssetUnits: big.NewInt(456),
			GasUnits:           100_000,
		},
		candidate: core.TxCandidate{To: vault, Value: big.NewInt(0), Data: []byte{0x01}},
	}
	clock := &mutableClock{value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)}
	repo := &fakeWithdrawalRepository{err: errors.New("database temporarily unavailable")}
	submitter := &fakeSubmitter{nonce: 7, gasPrice: big.NewInt(100), tipCap: big.NewInt(8)}
	service := Service{
		Adapter:            adapter,
		Signer:             &fakeSigner{address: owner},
		Submitter:          submitter,
		Repository:         repo,
		ChainID:            big.NewInt(1),
		Clock:              clock,
		GasPolicy:          GasPolicy{BumpBPS: 1250, MaxFeeCap: big.NewInt(112), MaxTipCap: big.NewInt(10)},
		GasLimitBufferBPS:  2000,
		ReplacementTimeout: 2 * time.Minute,
	}

	// Act
	result, err := service.ExecuteFullExit(ctx, urgentTrigger())
	repo.err = nil
	clock.value = clock.value.Add(time.Minute)
	_, nextErr := service.ExecuteFullExit(ctx, urgentTrigger())

	// Assert
	if err == nil {
		t.Fatal("expected repository error")
	}
	if result.Status != WithdrawalStatusSubmitted {
		t.Fatalf("expected submitted result despite repository error, got %s", result.Status)
	}
	if result.TxHash == (common.Hash{}) {
		t.Fatal("expected submitted tx hash despite repository error")
	}
	if nextErr != nil {
		t.Fatalf("handle pending full exit: %v", nextErr)
	}
	if len(submitter.sent) != 1 {
		t.Fatalf("expected pending nonce to prevent duplicate submit, got %d submitted transactions", len(submitter.sent))
	}
}

func urgentTrigger() WithdrawalTrigger {
	return WithdrawalTrigger{
		Kind:       TriggerKindUrgent,
		ModuleID:   core.ModuleWithdrawLiquidity,
		FindingKey: core.FindingIdleLiquidity,
	}
}

type fakeWithdrawAdapter struct {
	position      core.PositionSnapshot
	simulation    core.FullExitSimulation
	candidate     core.TxCandidate
	positionErr   error
	buildErr      error
	simulationErr error
	simulateCalls int
}

func (adapter *fakeWithdrawAdapter) ID() string {
	return MorphoVaultV2AdapterID
}

func (adapter *fakeWithdrawAdapter) Position(ctx context.Context) (core.PositionSnapshot, error) {
	if adapter.positionErr != nil {
		return core.PositionSnapshot{}, adapter.positionErr
	}
	return adapter.position.Clone(), nil
}

func (adapter *fakeWithdrawAdapter) BuildFullExit(ctx context.Context, req core.FullExitRequest) (core.TxCandidate, error) {
	if adapter.buildErr != nil {
		return core.TxCandidate{}, adapter.buildErr
	}
	return adapter.candidate.Clone(), nil
}

func (adapter *fakeWithdrawAdapter) SimulateFullExit(ctx context.Context, req core.FullExitRequest) (core.FullExitSimulation, error) {
	adapter.simulateCalls++
	if adapter.simulationErr != nil {
		return core.FullExitSimulation{}, adapter.simulationErr
	}
	return adapter.simulation.Clone(), nil
}

type fakeSigner struct {
	address   common.Address
	signCalls int
	signed    []*types.Transaction
}

func (signer *fakeSigner) Address(ctx context.Context) (common.Address, error) {
	return signer.address, nil
}

func (signer *fakeSigner) SignTransaction(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	signer.signCalls++
	signer.signed = append(signer.signed, tx)
	return tx, nil
}

type fakeSubmitter struct {
	nonce       uint64
	gasPrice    *big.Int
	tipCap      *big.Int
	sent        []*types.Transaction
	releaseSend chan struct{}
	sendStarted chan struct{}
	mu          sync.Mutex
}

func (submitter *fakeSubmitter) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return submitter.nonce, nil
}

func (submitter *fakeSubmitter) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return new(big.Int).Set(submitter.tipCap), nil
}

func (submitter *fakeSubmitter) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return new(big.Int).Set(submitter.gasPrice), nil
}

func (submitter *fakeSubmitter) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	submitter.mu.Lock()
	submitter.sent = append(submitter.sent, tx)
	submitter.mu.Unlock()
	if submitter.sendStarted != nil {
		submitter.sendStarted <- struct{}{}
	}
	if submitter.releaseSend != nil {
		<-submitter.releaseSend
	}
	return nil
}

func (submitter *fakeSubmitter) EstimateGas(ctx context.Context, call geth.CallMsg) (uint64, error) {
	return 0, nil
}

type fakeWithdrawalRepository struct {
	attempts []WithdrawalAttempt
	err      error
}

func (repo *fakeWithdrawalRepository) InsertWithdrawalAttempt(ctx context.Context, attempt WithdrawalAttempt) error {
	if repo.err != nil {
		return repo.err
	}
	repo.attempts = append(repo.attempts, attempt)
	return nil
}

type mutableClock struct {
	value time.Time
}

func (clock mutableClock) Now() time.Time {
	return clock.value
}
