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
	"withdraw-bot/internal/core"
	"withdraw-bot/internal/ethereum"
	telegramcmd "withdraw-bot/internal/interactions/telegram"
	"withdraw-bot/internal/monitor"
	"withdraw-bot/internal/monitor/modules/morpho"
	"withdraw-bot/internal/signer"
	"withdraw-bot/internal/storage"
	"withdraw-bot/internal/withdraw"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	testPrimaryRPCURL       = "https://rpc.example/private-token"
	testTelegramChatID      = 100
	testTelegramAllowedUser = 1
	testTelegramDeniedUser  = 2
	testRuntimeLogMessage   = "warning event from runtime"
	testThresholdValue      = "75"
	testThresholdConfirmID  = "threshold:share_price_loss:loss_warn_bps:1"
	testRuntimeAdapterID    = "fake"
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

func TestBuildMonitorServicesWiresLogsProvider(t *testing.T) {
	// Arrange
	ctx := context.Background()
	runtime, db := buildTestRuntimeWithMonitorServices(t, ctx)
	telegramService := runtimeTelegramService(t, runtime)
	repos := storage.NewRepositories(db)
	if err := repos.InsertEvent(ctx, core.EventWarning, testRuntimeLogMessage, nil, time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	// Act
	result, err := telegramService.HandleCommand(ctx, testTelegramChatID, testTelegramAllowedUser, string(core.CommandLogs))

	// Assert
	if err != nil {
		t.Fatalf("handle logs command: %v", err)
	}
	if !strings.Contains(result.Text, testRuntimeLogMessage) {
		t.Fatalf("expected logs response to include event message, got %q", result.Text)
	}
}

func TestBuildMonitorServicesWiresThresholdProvider(t *testing.T) {
	// Arrange
	ctx := context.Background()
	runtime, db := buildTestRuntimeWithMonitorServices(t, ctx)
	telegramService := runtimeTelegramService(t, runtime)
	repos := storage.NewRepositories(db)

	// Act
	result, err := telegramService.HandleCommand(ctx, testTelegramChatID, testTelegramAllowedUser, string(core.CommandThresholdSet)+" set share_price_loss loss_warn_bps "+testThresholdValue)

	// Assert
	if err != nil {
		t.Fatalf("handle threshold set command: %v", err)
	}
	if !strings.Contains(result.Text, testThresholdConfirmID) {
		t.Fatalf("expected threshold confirmation id, got %q", result.Text)
	}

	// Act
	result, err = telegramService.HandleCommand(ctx, testTelegramChatID, testTelegramAllowedUser, string(core.CommandConfirm)+" "+testThresholdConfirmID)

	// Assert
	if err != nil {
		t.Fatalf("handle threshold confirm command: %v", err)
	}
	if !strings.Contains(result.Text, testThresholdValue) {
		t.Fatalf("expected threshold confirmation response, got %q", result.Text)
	}
	overrides, err := repos.ListThresholdOverrides(ctx)
	if err != nil {
		t.Fatalf("list threshold overrides: %v", err)
	}
	if len(overrides) != 1 || overrides[0].Value != testThresholdValue {
		t.Fatalf("expected threshold override value %q, got %+v", testThresholdValue, overrides)
	}
}

func TestBuildMonitorServicesWiresSecurityEventRecorder(t *testing.T) {
	// Arrange
	ctx := context.Background()
	runtime, db := buildTestRuntimeWithMonitorServices(t, ctx)
	telegramService := runtimeTelegramService(t, runtime)

	// Act
	_, err := telegramService.HandleCommand(ctx, testTelegramChatID, testTelegramDeniedUser, string(core.CommandStats))

	// Assert
	if err == nil {
		t.Fatal("expected authorization error")
	}
	events, err := storage.NewRepositories(db).ListRecentEvents(ctx, false, 10)
	if err != nil {
		t.Fatalf("list recent events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != core.EventSecurity {
		t.Fatalf("expected security event, got %+v", events)
	}
}

func TestBuildMonitorServicesWiresUrgentResultsToAutoWithdraw(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000002")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000001")
	vault := common.HexToAddress("0x0000000000000000000000000000000000000003")
	sender := &fakeRuntimeMessageSender{}
	rpc := &fakeAutoWithdrawRPCClient{
		chainID:  big.NewInt(1),
		gasPrice: big.NewInt(1_000_000_000),
		tipCap:   big.NewInt(100_000_000),
	}
	runtime, db := buildTestRuntimeWithMonitorServicesForAutoWithdraw(t, ctx, owner, receiver, vault, rpc)
	telegramService := runtimeTelegramService(t, runtime)
	telegramService.Sender = sender
	monitorService := runtimeMonitorService(t, runtime)

	// Act
	_, err := monitorService.RunOnce(ctx)

	// Assert
	if err != nil {
		t.Fatalf("run monitor once: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one Telegram alert, got %d", sender.calls)
	}
	if rpc.sent != 1 {
		t.Fatalf("expected one submitted withdrawal transaction, got %d", rpc.sent)
	}
	var triggerKind string
	var triggerModuleID string
	var triggerFindingKey string
	var status string
	if err := db.QueryRowContext(ctx, `SELECT trigger_kind, trigger_module_id, trigger_finding_key, status FROM withdrawal_attempts`).Scan(&triggerKind, &triggerModuleID, &triggerFindingKey, &status); err != nil {
		t.Fatalf("query withdrawal attempt: %v", err)
	}
	if triggerKind != string(withdraw.TriggerKindUrgent) {
		t.Fatalf("expected urgent trigger kind, got %q", triggerKind)
	}
	if triggerModuleID != string(core.ModuleWithdrawLiquidity) {
		t.Fatalf("expected trigger module %q, got %q", core.ModuleWithdrawLiquidity, triggerModuleID)
	}
	if triggerFindingKey != string(core.FindingIdleLiquidity) {
		t.Fatalf("expected trigger finding %q, got %q", core.FindingIdleLiquidity, triggerFindingKey)
	}
	if status != string(withdraw.WithdrawalStatusSubmitted) {
		t.Fatalf("expected withdrawal status %q, got %q", withdraw.WithdrawalStatusSubmitted, status)
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

func buildTestRuntimeWithMonitorServices(t *testing.T, ctx context.Context) (Runtime, *sql.DB) {
	t.Helper()
	var db *sql.DB
	withRuntimeDependencies(t, runtimeDependencies{
		openStorage: func(ctx context.Context, path string) (*sql.DB, error) {
			var err error
			db, err = storage.Open(ctx, ":memory:")
			return db, err
		},
		newTelegramBot: func(token string) (*tgbotapi.BotAPI, error) {
			return &tgbotapi.BotAPI{Token: token}, nil
		},
	})
	cfg := testRuntimeConfig()
	cfg.App.DataDir = t.TempDir()
	cfg.Telegram = config.TelegramConfig{ChatID: testTelegramChatID, AllowedUserIDs: []int64{testTelegramAllowedUser}}
	cfg.Logs = config.LogConfig{FilePath: cfg.App.DataDir + "/withdraw-bot.log", MaxSizeMB: 1, MaxBackups: 1, MaxAgeDays: 1}
	runtime := Runtime{
		Config:  cfg,
		Secrets: testRuntimeSecrets(),
		Signer:  fakeRuntimeSigner{address: common.HexToAddress("0x0000000000000000000000000000000000000002")},
		Submitter: ethereum.NewMultiClient(fakeRPCClient{
			chainID: big.NewInt(1),
		}, nil),
	}
	cleanup, err := buildMonitorServices(ctx, &runtime)
	if err != nil {
		t.Fatalf("build monitor services: %v", err)
	}
	t.Cleanup(cleanup)
	return runtime, db
}

func buildTestRuntimeWithMonitorServicesForAutoWithdraw(t *testing.T, ctx context.Context, owner common.Address, receiver common.Address, vault common.Address, rpc *fakeAutoWithdrawRPCClient) (Runtime, *sql.DB) {
	t.Helper()
	var db *sql.DB
	withRuntimeDependencies(t, runtimeDependencies{
		openStorage: func(ctx context.Context, path string) (*sql.DB, error) {
			var err error
			db, err = storage.Open(ctx, ":memory:")
			return db, err
		},
		newTelegramBot: func(token string) (*tgbotapi.BotAPI, error) {
			return &tgbotapi.BotAPI{Token: token}, nil
		},
	})
	cfg := testRuntimeConfig()
	cfg.App.DataDir = t.TempDir()
	cfg.Telegram = config.TelegramConfig{ChatID: testTelegramChatID, AllowedUserIDs: []int64{testTelegramAllowedUser}}
	cfg.Logs = config.LogConfig{FilePath: cfg.App.DataDir + "/withdraw-bot.log", MaxSizeMB: 1, MaxBackups: 1, MaxAgeDays: 1}
	runtime := Runtime{
		Config:    cfg,
		Secrets:   testRuntimeSecrets(),
		Signer:    fakeRuntimeSigner{address: owner},
		Submitter: ethereum.NewMultiClient(rpc, nil),
		Adapter: fakeRuntimeWithdrawAdapter{
			position: core.PositionSnapshot{
				Vault:        vault,
				Owner:        owner,
				Receiver:     receiver,
				ShareBalance: big.NewInt(100),
			},
			simulation: core.FullExitSimulation{Success: true, ExpectedAssetUnits: big.NewInt(100_000_000), GasUnits: 21_000},
			candidate:  core.TxCandidate{To: vault, Data: []byte{0x01}, Value: big.NewInt(0)},
		},
		MonitorModules: []monitor.Module{fakeRuntimeMonitorModule{
			id: core.ModuleWithdrawLiquidity,
			result: core.MonitorResult{
				ModuleID: core.ModuleWithdrawLiquidity,
				Status:   core.MonitorStatusUrgent,
				Findings: []core.Finding{{
					Key:      core.FindingIdleLiquidity,
					Severity: core.SeverityUrgent,
					Message:  "idle liquidity urgent",
				}},
			},
		}},
	}
	cleanup, err := buildMonitorServices(ctx, &runtime)
	if err != nil {
		t.Fatalf("build monitor services: %v", err)
	}
	t.Cleanup(cleanup)
	return runtime, db
}

func runtimeTelegramService(t *testing.T, runtime Runtime) *telegramcmd.Service {
	t.Helper()
	switch telegramService := runtime.Telegram.(type) {
	case *telegramcmd.Service:
		return telegramService
	case telegramcmd.Service:
		return &telegramService
	default:
		t.Fatalf("expected telegram service, got %T", runtime.Telegram)
		return nil
	}
}

func runtimeMonitorService(t *testing.T, runtime Runtime) *monitor.Service {
	t.Helper()
	switch monitorService := runtime.Monitor.(type) {
	case *monitor.Service:
		return monitorService
	default:
		t.Fatalf("expected monitor service, got %T", runtime.Monitor)
		return nil
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

type fakeRuntimeMessageSender struct {
	calls int
}

func (sender *fakeRuntimeMessageSender) SendText(ctx context.Context, chatID int64, text string) error {
	sender.calls++
	return nil
}

type fakeRuntimeMonitorModule struct {
	id     core.MonitorModuleID
	result core.MonitorResult
}

func (module fakeRuntimeMonitorModule) ID() core.MonitorModuleID {
	return module.id
}

func (module fakeRuntimeMonitorModule) ValidateConfig(ctx context.Context) error {
	return nil
}

func (module fakeRuntimeMonitorModule) Bootstrap(ctx context.Context) (map[string]any, error) {
	return nil, nil
}

func (module fakeRuntimeMonitorModule) Monitor(ctx context.Context) (core.MonitorResult, error) {
	return module.result, nil
}

type fakeRuntimeWithdrawAdapter struct {
	position   core.PositionSnapshot
	simulation core.FullExitSimulation
	candidate  core.TxCandidate
}

func (adapter fakeRuntimeWithdrawAdapter) ID() string {
	return testRuntimeAdapterID
}

func (adapter fakeRuntimeWithdrawAdapter) Position(ctx context.Context) (core.PositionSnapshot, error) {
	return adapter.position.Clone(), nil
}

func (adapter fakeRuntimeWithdrawAdapter) BuildFullExit(ctx context.Context, req core.FullExitRequest) (core.TxCandidate, error) {
	return adapter.candidate.Clone(), nil
}

func (adapter fakeRuntimeWithdrawAdapter) SimulateFullExit(ctx context.Context, req core.FullExitRequest) (core.FullExitSimulation, error) {
	return adapter.simulation.Clone(), nil
}

type fakeAutoWithdrawRPCClient struct {
	chainID  *big.Int
	gasPrice *big.Int
	tipCap   *big.Int
	sent     int
}

func (client *fakeAutoWithdrawRPCClient) CallContract(ctx context.Context, call geth.CallMsg, blockNumber *big.Int) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (client *fakeAutoWithdrawRPCClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return 7, nil
}

func (client *fakeAutoWithdrawRPCClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return new(big.Int).Set(client.gasPrice), nil
}

func (client *fakeAutoWithdrawRPCClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return new(big.Int).Set(client.tipCap), nil
}

func (client *fakeAutoWithdrawRPCClient) EstimateGas(ctx context.Context, call geth.CallMsg) (uint64, error) {
	return 0, errors.New("not implemented")
}

func (client *fakeAutoWithdrawRPCClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	client.sent++
	return nil
}

func (client *fakeAutoWithdrawRPCClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return nil, errors.New("not implemented")
}

func (client *fakeAutoWithdrawRPCClient) ChainID(ctx context.Context) (*big.Int, error) {
	return new(big.Int).Set(client.chainID), nil
}

func (client *fakeAutoWithdrawRPCClient) Close() {}
