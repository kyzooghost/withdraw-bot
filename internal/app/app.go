package app

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"os"
	"time"

	"withdraw-bot/internal/config"
	"withdraw-bot/internal/signer"

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
	Config   config.Config
	Secrets  config.Secrets
	Ethereum chainIDClient
	Signer   signer.Service
	Receiver common.Address
	Modules  []BootstrapModule
	Monitor  monitorRunner
	Telegram telegramRunner
	Output   io.Writer
	Close    func()
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
	switch mode {
	case ModeMonitor:
		return runMonitor(ctx, runtime)
	case ModeBootstrap:
		return runBootstrap(ctx, runtime)
	case ModeConfigCheck:
		return runConfigCheck(ctx, runtime)
	default:
		return fmt.Errorf(errUnsupportedMode, mode)
	}
}

func runMonitor(ctx context.Context, runtime Runtime) error {
	if runtime.Monitor == nil {
		return fmt.Errorf(errMonitorRequired)
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
	secondErr := <-errCh
	if firstErr != nil {
		return firstErr
	}
	return secondErr
}

func runBootstrap(ctx context.Context, runtime Runtime) error {
	fragments, err := CollectBootstrapFragments(ctx, runtime.Modules)
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
	return encoder.Encode(fragments)
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
