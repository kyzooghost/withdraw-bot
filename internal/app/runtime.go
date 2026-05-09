package app

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"withdraw-bot/internal/config"
	"withdraw-bot/internal/ethereum"
	"withdraw-bot/internal/interactions/telegram"
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
	databaseFileName  = "withdraw-bot.sqlite"
	errInvalidAddress = "%s must be a valid Ethereum address"
)

func buildRuntime(ctx context.Context, configPath string) (Runtime, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return Runtime{}, err
	}
	secrets, err := config.LoadSecretsFromEnv()
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
	ethClient, err := ethereum.DialMulti(ctx, secrets.PrimaryRPCURL, secrets.FallbackRPCURLs)
	if err != nil {
		return Runtime{}, sanitizeRuntimeError(secrets, err)
	}
	closeRuntime := func() {
		ethClient.Close()
	}
	signerService, err := signer.NewPrivateKeyService(secrets.PrivateKey)
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
	if err := os.MkdirAll(cfg.App.DataDir, 0o755); err != nil {
		closeRuntime()
		return Runtime{}, fmt.Errorf("create data dir: %w", err)
	}
	db, err := storage.Open(ctx, filepath.Join(cfg.App.DataDir, databaseFileName))
	if err != nil {
		closeRuntime()
		return Runtime{}, err
	}
	closeRuntime = func() {
		db.Close()
		ethClient.Close()
	}
	repos := storage.NewRepositories(db)
	monitorService := monitor.NewService(modules, repos, nil)
	bot, err := tgbotapi.NewBotAPI(secrets.TelegramToken)
	if err != nil {
		closeRuntime()
		return Runtime{}, sanitizeRuntimeError(secrets, err)
	}
	replacementTimeout, err := time.ParseDuration(cfg.Gas.ReplacementTimeout)
	if err != nil {
		closeRuntime()
		return Runtime{}, err
	}
	maxFeeCap, err := config.ParseGwei("gas.max_fee_per_gas_gwei", cfg.Gas.MaxFeePerGasGwei)
	if err != nil {
		closeRuntime()
		return Runtime{}, err
	}
	maxTipCap, err := config.ParseGwei("gas.max_priority_fee_per_gas_gwei", cfg.Gas.MaxPriorityFeePerGasGwei)
	if err != nil {
		closeRuntime()
		return Runtime{}, err
	}
	withdrawService := &withdraw.Service{
		Adapter:            adapter,
		Signer:             signerService,
		ChainID:            big.NewInt(cfg.Ethereum.ChainID),
		Submitter:          ethClient,
		GasPolicy:          withdraw.GasPolicy{BumpBPS: cfg.Gas.FeeBumpBPS, MaxFeeCap: maxFeeCap, MaxTipCap: maxTipCap},
		GasLimitBufferBPS:  cfg.Gas.GasLimitBufferBPS,
		ReplacementTimeout: replacementTimeout,
	}
	runtime := Runtime{
		Config:   cfg,
		Secrets:  secrets,
		Ethereum: ethClient,
		Signer:   signerService,
		Receiver: receiver,
		Modules:  bootstrapModules(modules),
		Monitor:  monitorService,
		Telegram: telegram.Service{
			Bot:           bot,
			Authorization: telegram.Authorization{ChatID: cfg.Telegram.ChatID, AllowedUserIDs: allowedUserIDs(cfg.Telegram.AllowedUserIDs)},
			Reports:       reportProvider{monitor: monitorService},
			Withdraw:      withdrawService,
		},
		Output: os.Stdout,
		Close:  closeRuntime,
	}
	return runtime, nil
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
