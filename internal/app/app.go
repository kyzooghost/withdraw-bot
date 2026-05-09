package app

import (
	"context"
	"fmt"
)

func Run(ctx context.Context, mode Mode, configPath string) error {
	if configPath == "" {
		return fmt.Errorf("config path is required")
	}
	switch mode {
	case ModeMonitor, ModeBootstrap, ModeConfigCheck:
		return nil
	default:
		return fmt.Errorf("unsupported mode %q", mode)
	}
}
