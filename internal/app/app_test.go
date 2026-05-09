package app

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"withdraw-bot/internal/config"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestRunConfigCheckReturnsChainIDMismatch(t *testing.T) {
	// Arrange
	runtime := Runtime{
		Config: config.Config{Ethereum: config.EthereumConfig{ChainID: 1, ReceiverAddress: "0x0000000000000000000000000000000000000001"}},
		Ethereum: fakeChainClient{
			chainID: big.NewInt(2),
		},
		Signer:   fakeRuntimeSigner{address: common.HexToAddress("0x0000000000000000000000000000000000000002")},
		Receiver: common.HexToAddress("0x0000000000000000000000000000000000000001"),
	}

	// Act
	err := runConfigCheck(context.Background(), runtime)

	// Assert
	if err == nil {
		t.Fatalf("expected chain ID mismatch")
	}
	if !strings.Contains(err.Error(), "chain ID mismatch") {
		t.Fatalf("expected chain ID mismatch, got %v", err)
	}
}

func TestRunBootstrapWritesYAMLFragments(t *testing.T) {
	// Arrange
	var output bytes.Buffer
	runtime := Runtime{
		Modules: []BootstrapModule{
			fakeBootstrapModule{id: "share_price_loss", fragment: map[string]any{"baseline_share_price_asset_units": "1000000"}},
		},
		Output: &output,
	}

	// Act
	err := runBootstrap(context.Background(), runtime)

	// Assert
	if err != nil {
		t.Fatalf("run bootstrap: %v", err)
	}
	if !strings.Contains(output.String(), "baseline_share_price_asset_units: \"1000000\"") {
		t.Fatalf("expected bootstrap YAML fragment, got %q", output.String())
	}
}

func TestRunMonitorStartsMonitorAndTelegram(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	monitor := &fakeMonitorRunner{cancel: cancel}
	telegram := &fakeTelegramRunner{}
	runtime := Runtime{
		Config:   config.Config{App: config.AppConfig{MonitorInterval: "1ms"}},
		Monitor:  monitor,
		Telegram: telegram,
	}

	// Act
	err := runMonitor(ctx, runtime)

	// Assert
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("run monitor: %v", err)
	}
	if monitor.calls != 1 {
		t.Fatalf("expected monitor to start once, got %d", monitor.calls)
	}
	if telegram.calls != 1 {
		t.Fatalf("expected telegram to start once, got %d", telegram.calls)
	}
}

type fakeChainClient struct {
	chainID *big.Int
}

func (client fakeChainClient) ChainID(ctx context.Context) (*big.Int, error) {
	return new(big.Int).Set(client.chainID), nil
}

func (client fakeChainClient) Close() {}

type fakeRuntimeSigner struct {
	address common.Address
}

func (signer fakeRuntimeSigner) Address(ctx context.Context) (common.Address, error) {
	return signer.address, nil
}

func (signer fakeRuntimeSigner) SignTransaction(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	return tx, nil
}

type fakeMonitorRunner struct {
	calls  int
	cancel context.CancelFunc
}

func (runner *fakeMonitorRunner) RunLoop(ctx context.Context, interval time.Duration) error {
	runner.calls++
	runner.cancel()
	<-ctx.Done()
	return ctx.Err()
}

type fakeTelegramRunner struct {
	calls int
}

func (runner *fakeTelegramRunner) Start(ctx context.Context) error {
	runner.calls++
	<-ctx.Done()
	return ctx.Err()
}
