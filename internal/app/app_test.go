package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"withdraw-bot/internal/config"
	"withdraw-bot/internal/ethereum"
	"withdraw-bot/internal/monitor/modules/morpho"
	"withdraw-bot/internal/signer"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const testPrimaryRPCURL = "https://rpc.example/private-token"

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
	telegramStarted := make(chan struct{})
	monitor := &fakeMonitorRunner{cancel: cancel, waitFor: telegramStarted}
	telegram := &fakeTelegramRunner{started: telegramStarted}
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

func TestRunConfigCheckDoesNotInitializeMonitorOnlyServices(t *testing.T) {
	// Arrange
	storageCalls := 0
	telegramCalls := 0
	withRuntimeDependencies(t, runtimeDependencies{
		loadConfig: func(path string) (config.Config, error) {
			return testRuntimeConfig(), nil
		},
		loadSecrets: func() (config.Secrets, error) {
			return testRuntimeSecrets(), nil
		},
		dialEthereum: func(ctx context.Context, primaryURL string, fallbackURLs []string) (ethereum.MultiClient, error) {
			return ethereum.NewMultiClient(fakeRPCClient{chainID: big.NewInt(1)}, nil), nil
		},
		newSigner: func(privateKeyHex string) (signer.Service, error) {
			return fakeRuntimeSigner{address: common.HexToAddress("0x0000000000000000000000000000000000000002")}, nil
		},
		openStorage: func(ctx context.Context, path string) (*sql.DB, error) {
			storageCalls++
			return nil, errors.New("storage should not open")
		},
		newTelegramBot: func(token string) (*tgbotapi.BotAPI, error) {
			telegramCalls++
			return nil, errors.New("telegram should not initialize")
		},
	})

	// Act
	err := Run(context.Background(), ModeConfigCheck, "config.yaml")

	// Assert
	if err != nil {
		t.Fatalf("run config-check: %v", err)
	}
	if storageCalls != 0 {
		t.Fatalf("expected storage not to open, got %d calls", storageCalls)
	}
	if telegramCalls != 0 {
		t.Fatalf("expected telegram not to initialize, got %d calls", telegramCalls)
	}
}

func TestRunRedactsRuntimeModeErrors(t *testing.T) {
	// Arrange
	withRuntimeDependencies(t, runtimeDependencies{
		loadConfig: func(path string) (config.Config, error) {
			return testRuntimeConfig(), nil
		},
		loadSecrets: func() (config.Secrets, error) {
			return testRuntimeSecrets(), nil
		},
		dialEthereum: func(ctx context.Context, primaryURL string, fallbackURLs []string) (ethereum.MultiClient, error) {
			return ethereum.NewMultiClient(fakeRPCClient{chainIDErr: errors.New("dial " + testPrimaryRPCURL)}, nil), nil
		},
		newSigner: func(privateKeyHex string) (signer.Service, error) {
			return fakeRuntimeSigner{address: common.HexToAddress("0x0000000000000000000000000000000000000002")}, nil
		},
	})

	// Act
	err := Run(context.Background(), ModeConfigCheck, "config.yaml")

	// Assert
	if err == nil {
		t.Fatal("expected config-check error")
	}
	if strings.Contains(err.Error(), testPrimaryRPCURL) {
		t.Fatalf("expected RPC URL to be redacted, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("expected redacted marker, got %q", err.Error())
	}
}

func TestRunMonitorReturnsFirstServiceError(t *testing.T) {
	// Arrange
	expectedErr := errors.New("monitor failed")
	release := make(chan struct{})
	defer close(release)
	runtime := Runtime{
		Config:   config.Config{App: config.AppConfig{MonitorInterval: "1ms"}},
		Monitor:  fakeFailingMonitorRunner{err: expectedErr},
		Telegram: blockingTelegramRunner{release: release},
	}
	done := make(chan error, 1)

	// Act
	go func() {
		done <- runMonitor(context.Background(), runtime)
	}()

	// Assert
	select {
	case err := <-done:
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected monitor error, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected runMonitor to return first service error")
	}
}

func TestBuildVaultStateModuleUsesSnakeCaseBaselineKeys(t *testing.T) {
	// Arrange
	moduleConfig := config.ModuleConfig{
		"enabled":         true,
		"change_severity": "urgent",
		"baseline": map[string]any{
			"receive_shares_gate": "0x0000000000000000000000000000000000000001",
			"liquidity_data_hex":  "0x1234",
		},
	}

	// Act
	module, err := buildVaultStateModule(moduleConfig, vaultReader{})

	// Assert
	if err != nil {
		t.Fatalf("build vault state module: %v", err)
	}
	if module.Baseline.ReceiveSharesGate != "0x0000000000000000000000000000000000000001" {
		t.Fatalf("expected receive_shares_gate baseline, got %q", module.Baseline.ReceiveSharesGate)
	}
	if module.Baseline.LiquidityDataHex != "0x1234" {
		t.Fatalf("expected liquidity_data_hex baseline, got %q", module.Baseline.LiquidityDataHex)
	}
}

func TestRunBootstrapWritesVaultStateSnakeCaseKeys(t *testing.T) {
	// Arrange
	var output bytes.Buffer
	runtime := Runtime{
		Modules: []BootstrapModule{
			fakeBootstrapModule{id: "vault_state_baseline", fragment: map[string]any{
				"baseline": morpho.VaultStateSnapshot{
					ReceiveSharesGate: "0x0000000000000000000000000000000000000001",
					LiquidityDataHex:  "0x1234",
				},
			}},
		},
		Output: &output,
	}

	// Act
	err := runBootstrap(context.Background(), runtime)

	// Assert
	if err != nil {
		t.Fatalf("run bootstrap: %v", err)
	}
	if !strings.Contains(output.String(), "receive_shares_gate:") {
		t.Fatalf("expected snake_case vault state key, got %q", output.String())
	}
	if !strings.Contains(output.String(), "liquidity_data_hex:") {
		t.Fatalf("expected snake_case liquidity data key, got %q", output.String())
	}
}

func TestBuildModulesRejectsUnknownEnabledModule(t *testing.T) {
	// Arrange
	cfg := config.Config{Modules: map[string]config.ModuleConfig{
		"typo": {"enabled": true},
	}}

	// Act
	_, err := buildModules(cfg, ethereum.MultiClient{}, common.Address{}, common.Address{}, common.Address{}, nil)

	// Assert
	if err == nil {
		t.Fatal("expected unknown enabled module to be rejected")
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
	calls   int
	cancel  context.CancelFunc
	waitFor <-chan struct{}
}

func (runner *fakeMonitorRunner) RunLoop(ctx context.Context, interval time.Duration) error {
	runner.calls++
	if runner.waitFor != nil {
		<-runner.waitFor
	}
	runner.cancel()
	<-ctx.Done()
	return ctx.Err()
}

type fakeTelegramRunner struct {
	calls   int
	started chan<- struct{}
}

func (runner *fakeTelegramRunner) Start(ctx context.Context) error {
	runner.calls++
	if runner.started != nil {
		close(runner.started)
	}
	<-ctx.Done()
	return ctx.Err()
}

type fakeFailingMonitorRunner struct {
	err error
}

func (runner fakeFailingMonitorRunner) RunLoop(ctx context.Context, interval time.Duration) error {
	return runner.err
}

type blockingTelegramRunner struct {
	release <-chan struct{}
}

func (runner blockingTelegramRunner) Start(ctx context.Context) error {
	<-runner.release
	return nil
}

type fakeRPCClient struct {
	chainID    *big.Int
	chainIDErr error
}

func (client fakeRPCClient) CallContract(ctx context.Context, call geth.CallMsg, blockNumber *big.Int) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (client fakeRPCClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return 0, errors.New("not implemented")
}

func (client fakeRPCClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return nil, errors.New("not implemented")
}

func (client fakeRPCClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return nil, errors.New("not implemented")
}

func (client fakeRPCClient) EstimateGas(ctx context.Context, call geth.CallMsg) (uint64, error) {
	return 0, errors.New("not implemented")
}

func (client fakeRPCClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return errors.New("not implemented")
}

func (client fakeRPCClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return nil, errors.New("not implemented")
}

func (client fakeRPCClient) ChainID(ctx context.Context) (*big.Int, error) {
	if client.chainIDErr != nil {
		return nil, client.chainIDErr
	}
	return new(big.Int).Set(client.chainID), nil
}

func (client fakeRPCClient) Close() {}

func testRuntimeConfig() config.Config {
	return config.Config{
		App: config.AppConfig{MonitorInterval: "5m", DataDir: "./data"},
		Ethereum: config.EthereumConfig{
			ChainID:         1,
			VaultAddress:    "0x0000000000000000000000000000000000000003",
			ReceiverAddress: "0x0000000000000000000000000000000000000001",
			AssetDecimals:   6,
		},
		Gas: config.GasConfig{
			ReplacementTimeout:       "2m",
			MaxFeePerGasGwei:         "200",
			MaxPriorityFeePerGasGwei: "5",
		},
	}
}

func testRuntimeSecrets() config.Secrets {
	return config.Secrets{
		PrivateKey:    "private",
		TelegramToken: "telegram",
		PrimaryRPCURL: testPrimaryRPCURL,
	}
}

func withRuntimeDependencies(t *testing.T, deps runtimeDependencies) {
	t.Helper()
	previous := runtimeDeps
	if deps.loadConfig == nil {
		deps.loadConfig = previous.loadConfig
	}
	if deps.loadSecrets == nil {
		deps.loadSecrets = previous.loadSecrets
	}
	if deps.dialEthereum == nil {
		deps.dialEthereum = previous.dialEthereum
	}
	if deps.newSigner == nil {
		deps.newSigner = previous.newSigner
	}
	if deps.openStorage == nil {
		deps.openStorage = previous.openStorage
	}
	if deps.newTelegramBot == nil {
		deps.newTelegramBot = previous.newTelegramBot
	}
	runtimeDeps = deps
	t.Cleanup(func() {
		runtimeDeps = previous
	})
}
