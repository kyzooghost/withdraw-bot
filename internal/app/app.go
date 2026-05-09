package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"time"

	"withdraw-bot/internal/config"
	"withdraw-bot/internal/ethereum"
	"withdraw-bot/internal/monitor"
	"withdraw-bot/internal/signer"
	"withdraw-bot/internal/withdraw"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

const (
	defaultOutputIndent       = 2
	errConfigPathRequired     = "config path is required"
	errMonitorRequired        = "monitor service is required"
	errTelegramRequired       = "telegram service is required"
	errRuntimeEthereumMissing = "ethereum client is required"
	errRuntimeSignerMissing   = "signer is required"
	errReceiverRequired       = "receiver address is required"
	errReceiverMismatch       = "receiver address mismatch"
	errChainIDMismatch        = "chain ID mismatch: config=%d rpc=%s"
	errUnsupportedMode        = "unsupported mode %q"
)

type Runtime struct {
	Config         config.Config
	Secrets        config.Secrets
	Ethereum       chainIDClient
	Submitter      ethereum.MultiClient
	Signer         signer.Service
	Receiver       common.Address
	Modules        []BootstrapModule
	MonitorModules []monitor.Module
	Adapter        withdraw.MorphoAdapter
	Monitor        monitorRunner
	Telegram       telegramRunner
	Output         io.Writer
	Close          func()
}

type chainIDClient interface {
	ChainID(ctx context.Context) (*big.Int, error)
	Close()
}

type monitorRunner interface {
	RunLoop(ctx context.Context, interval time.Duration) error
}

type telegramRunner interface {
	Start(ctx context.Context) error
}

func Run(ctx context.Context, mode Mode, configPath string) error {
	if configPath == "" {
		return fmt.Errorf(errConfigPathRequired)
	}
	runtime, err := buildRuntime(ctx, configPath)
	if err != nil {
		return err
	}
	if runtime.Close != nil {
		defer runtime.Close()
	}
	var modeErr error
	switch mode {
	case ModeMonitor:
		modeErr = runMonitor(ctx, runtime)
	case ModeBootstrap:
		modeErr = runBootstrap(ctx, runtime)
	case ModeConfigCheck:
		modeErr = runConfigCheck(ctx, runtime)
	default:
		return fmt.Errorf(errUnsupportedMode, mode)
	}
	if modeErr != nil {
		return sanitizeRuntimeError(runtime.Secrets, modeErr)
	}
	return nil
}

func runMonitor(ctx context.Context, runtime Runtime) error {
	if runtime.Monitor == nil {
		cleanup, err := buildMonitorServices(ctx, &runtime)
		if err != nil {
			return err
		}
		defer cleanup()
	}
	if runtime.Telegram == nil {
		return fmt.Errorf(errTelegramRequired)
	}
	interval, err := time.ParseDuration(runtime.Config.App.MonitorInterval)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)
	go func() {
		errCh <- runtime.Telegram.Start(ctx)
	}()
	go func() {
		errCh <- runtime.Monitor.RunLoop(ctx, interval)
	}()
	firstErr := <-errCh
	cancel()
	return firstErr
}

func runBootstrap(ctx context.Context, runtime Runtime) error {
	fragments, err := CollectBootstrapFragments(ctx, runtime.Modules)
	if err != nil {
		return sanitizeRuntimeError(runtime.Secrets, err)
	}
	normalized, err := normalizeYAMLValue(fragments)
	if err != nil {
		return err
	}
	output := runtime.Output
	if output == nil {
		output = os.Stdout
	}
	encoder := yaml.NewEncoder(output)
	encoder.SetIndent(defaultOutputIndent)
	defer encoder.Close()
	return encoder.Encode(normalized)
}

func normalizeYAMLValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func runConfigCheck(ctx context.Context, runtime Runtime) error {
	return RunChecks(ctx, []Check{
		func(ctx context.Context) error {
			if runtime.Ethereum == nil {
				return fmt.Errorf(errRuntimeEthereumMissing)
			}
			chainID, err := runtime.Ethereum.ChainID(ctx)
			if err != nil {
				return err
			}
			expected := big.NewInt(runtime.Config.Ethereum.ChainID)
			if chainID == nil || chainID.Cmp(expected) != 0 {
				return fmt.Errorf(errChainIDMismatch, runtime.Config.Ethereum.ChainID, chainID)
			}
			return nil
		},
		func(ctx context.Context) error {
			if runtime.Signer == nil {
				return fmt.Errorf(errRuntimeSignerMissing)
			}
			_, err := runtime.Signer.Address(ctx)
			return err
		},
		func(ctx context.Context) error {
			if runtime.Receiver == (common.Address{}) {
				return fmt.Errorf(errReceiverRequired)
			}
			if configured := common.HexToAddress(runtime.Config.Ethereum.ReceiverAddress); configured != runtime.Receiver {
				return fmt.Errorf(errReceiverMismatch)
			}
			return nil
		},
	})
}
