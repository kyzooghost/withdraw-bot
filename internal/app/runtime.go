package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"withdraw-bot/internal/config"
	"withdraw-bot/internal/core"
	"withdraw-bot/internal/ethereum"
	"withdraw-bot/internal/interactions/telegram"
	"withdraw-bot/internal/logging"
	"withdraw-bot/internal/monitor"
	morphovault "withdraw-bot/internal/morpho"
	"withdraw-bot/internal/reports"
	"withdraw-bot/internal/signer"
	"withdraw-bot/internal/storage"
	"withdraw-bot/internal/withdraw"

	"github.com/ethereum/go-ethereum/common"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	databaseFileName                 = "withdraw-bot.sqlite"
	errCreateLogDir                  = "create log dir"
	errInvalidAddress                = "%s must be a valid Ethereum address"
	runtimeMonitorServicesReadyEvent = "monitor services initialized"
)

type runtimeDependencies struct {
	loadConfig     func(path string) (config.Config, error)
	loadSecrets    func() (config.Secrets, error)
	dialEthereum   func(ctx context.Context, primaryURL string, fallbackURLs []string) (ethereum.MultiClient, error)
	newSigner      func(privateKeyHex string) (signer.Service, error)
	openStorage    func(ctx context.Context, path string) (*sql.DB, error)
	newTelegramBot func(token string) (*tgbotapi.BotAPI, error)
}

var runtimeDeps = runtimeDependencies{
	loadConfig:     config.Load,
	loadSecrets:    config.LoadSecretsFromEnv,
	dialEthereum:   ethereum.DialMulti,
	newSigner:      newPrivateKeySigner,
	openStorage:    storage.Open,
	newTelegramBot: tgbotapi.NewBotAPI,
}

func buildRuntime(ctx context.Context, configPath string) (Runtime, error) {
	cfg, err := runtimeDeps.loadConfig(configPath)
	if err != nil {
		return Runtime{}, err
	}
	secrets, err := runtimeDeps.loadSecrets()
	if err != nil {
		return Runtime{}, err
	}
	vault, err := parseAddress("ethereum.vault_address", cfg.Ethereum.VaultAddress)
	if err != nil {
		return Runtime{}, err
	}
	receiver, err := parseAddress("ethereum.receiver_address", cfg.Ethereum.ReceiverAddress)
	if err != nil {
		return Runtime{}, err
	}
	ethClient, err := runtimeDeps.dialEthereum(ctx, secrets.PrimaryRPCURL, secrets.FallbackRPCURLs)
	if err != nil {
		return Runtime{}, sanitizeRuntimeError(secrets, err)
	}
	closeRuntime := func() {
		ethClient.Close()
	}
	signerService, err := runtimeDeps.newSigner(secrets.PrivateKey)
	if err != nil {
		closeRuntime()
		return Runtime{}, err
	}
	signerAddress, err := signerService.Address(ctx)
	if err != nil {
		closeRuntime()
		return Runtime{}, err
	}
	vaultClient := morphovault.VaultClient{Ethereum: ethClient, Vault: vault}
	adapter := withdraw.MorphoAdapter{
		Ethereum:      ethClient,
		VaultClient:   vaultClient,
		Vault:         vault,
		Owner:         signerAddress,
		Receiver:      receiver,
		AssetSymbol:   cfg.Ethereum.AssetSymbol,
		AssetDecimals: cfg.Ethereum.AssetDecimals,
	}
	modules, err := buildModules(cfg, ethClient, vault, signerAddress, receiver, adapter)
	if err != nil {
		closeRuntime()
		return Runtime{}, err
	}
	runtime := Runtime{
		Config:         cfg,
		Secrets:        secrets,
		Ethereum:       ethClient,
		Submitter:      ethClient,
		Signer:         signerService,
		Receiver:       receiver,
		Modules:        bootstrapModules(modules),
		MonitorModules: modules,
		Adapter:        adapter,
		Output:         os.Stdout,
		Close:          closeRuntime,
	}
	return runtime, nil
}

func buildMonitorServices(ctx context.Context, runtime *Runtime) (func(), error) {
	if err := os.MkdirAll(runtime.Config.App.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	logCleanup, err := setupRuntimeLogger(runtime.Config.Logs)
	if err != nil {
		return nil, err
	}
	db, err := runtimeDeps.openStorage(ctx, filepath.Join(runtime.Config.App.DataDir, databaseFileName))
	if err != nil {
		logCleanup()
		return nil, err
	}
	cleanup := func() {
		logCleanup()
		db.Close()
	}
	repos := storage.NewRepositories(db)
	monitorModules := withThresholdOverrides(runtime.MonitorModules, repos, runtime.Config.Ethereum.AssetDecimals)
	monitorService := monitor.NewService(monitorModules, repos, nil)
	bot, err := runtimeDeps.newTelegramBot(runtime.Secrets.TelegramToken)
	if err != nil {
		cleanup()
		return nil, sanitizeRuntimeError(runtime.Secrets, err)
	}
	replacementTimeout, err := time.ParseDuration(runtime.Config.Gas.ReplacementTimeout)
	if err != nil {
		cleanup()
		return nil, err
	}
	maxFeeCap, err := config.ParseGwei("gas.max_fee_per_gas_gwei", runtime.Config.Gas.MaxFeePerGasGwei)
	if err != nil {
		cleanup()
		return nil, err
	}
	maxTipCap, err := config.ParseGwei("gas.max_priority_fee_per_gas_gwei", runtime.Config.Gas.MaxPriorityFeePerGasGwei)
	if err != nil {
		cleanup()
		return nil, err
	}
	withdrawService := &withdraw.Service{
		Adapter:            runtime.Adapter,
		Signer:             runtime.Signer,
		ChainID:            big.NewInt(runtime.Config.Ethereum.ChainID),
		Submitter:          runtime.Submitter,
		Repository:         repos,
		Clock:              core.SystemClock{},
		GasPolicy:          withdraw.GasPolicy{BumpBPS: runtime.Config.Gas.FeeBumpBPS, MaxFeeCap: maxFeeCap, MaxTipCap: maxTipCap},
		GasLimitBufferBPS:  runtime.Config.Gas.GasLimitBufferBPS,
		ReplacementTimeout: replacementTimeout,
	}
	telegramService := &telegram.Service{
		Bot:           bot,
		Authorization: telegram.Authorization{ChatID: runtime.Config.Telegram.ChatID, AllowedUserIDs: allowedUserIDs(runtime.Config.Telegram.AllowedUserIDs)},
		Reports:       reportProvider{monitor: monitorService},
		Withdraw:      withdrawService,
		Thresholds:    thresholdProvider{repos: repos, config: runtime.Config, assetDecimals: runtime.Config.Ethereum.AssetDecimals},
		Logs:          eventLogProvider{repos: repos},
		Events:        repos,
	}
	monitorService.ResultHandler = AlertService{
		Withdrawer: urgentWithdrawer{executor: withdrawService},
		Notifier:   telegramAlertNotifier{sender: telegramService},
		Clock:      core.SystemClock{},
	}
	runtime.Monitor = monitorService
	runtime.MonitorModules = monitorModules
	runtime.Telegram = telegramService
	slog.InfoContext(ctx, runtimeMonitorServicesReadyEvent)
	return cleanup, nil
}

func setupRuntimeLogger(logConfig config.LogConfig) (func(), error) {
	if logConfig.FilePath == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(logConfig.FilePath), 0o755); err != nil {
		return nil, fmt.Errorf("%s: %w", errCreateLogDir, err)
	}
	logger, closer := logging.New(logging.Config{
		FilePath:   logConfig.FilePath,
		MaxSizeMB:  logConfig.MaxSizeMB,
		MaxBackups: logConfig.MaxBackups,
		MaxAgeDays: logConfig.MaxAgeDays,
	})
	previous := slog.Default()
	slog.SetDefault(logger)
	return func() {
		slog.SetDefault(previous)
		_ = closer.Close()
	}, nil
}

func parseAddress(name string, value string) (common.Address, error) {
	if !common.IsHexAddress(value) {
		return common.Address{}, fmt.Errorf(errInvalidAddress, name)
	}
	address := common.HexToAddress(value)
	if address == (common.Address{}) {
		return common.Address{}, fmt.Errorf(errInvalidAddress, name)
	}
	return address, nil
}

func sanitizeRuntimeError(secrets config.Secrets, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, secret := range []string{secrets.PrivateKey, secrets.TelegramToken, secrets.PrimaryRPCURL} {
		message = redactValue(message, secret)
	}
	for _, secret := range secrets.FallbackRPCURLs {
		message = redactValue(message, secret)
	}
	return fmt.Errorf("%s", message)
}

func redactValue(message string, value string) string {
	if value == "" {
		return message
	}
	return strings.ReplaceAll(message, value, "[REDACTED]")
}

func newPrivateKeySigner(privateKeyHex string) (signer.Service, error) {
	return signer.NewPrivateKeyService(privateKeyHex)
}

func allowedUserIDs(ids []int64) map[int64]bool {
	result := make(map[int64]bool, len(ids))
	for _, id := range ids {
		result[id] = true
	}
	return result
}

func bootstrapModules(modules []monitor.Module) []BootstrapModule {
	result := make([]BootstrapModule, 0, len(modules))
	for _, module := range modules {
		result = append(result, module)
	}
	return result
}

type reportProvider struct {
	monitor *monitor.Service
}

func (provider reportProvider) Stats(ctx context.Context) (string, error) {
	return reports.RenderStats(provider.monitor.Snapshot()), nil
}
