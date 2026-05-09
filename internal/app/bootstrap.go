package app

import (
	"context"

	"withdraw-bot/internal/core"
)

type BootstrapModule interface {
	ID() core.MonitorModuleID
	Bootstrap(ctx context.Context) (map[string]any, error)
}

func CollectBootstrapFragments(ctx context.Context, modules []BootstrapModule) (map[string]any, error) {
	result := make(map[string]any, len(modules))
	for _, module := range modules {
		fragment, err := module.Bootstrap(ctx)
		if err != nil {
			return nil, err
		}
		result[string(module.ID())] = fragment
	}
	return result, nil
}
