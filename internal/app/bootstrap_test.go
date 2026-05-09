package app

import (
	"context"
	"testing"

	"withdraw-bot/internal/core"
)

func TestCollectBootstrapFragmentsIncludesModuleOutput(t *testing.T) {
	// Arrange
	module := fakeBootstrapModule{id: "share_price_loss", fragment: map[string]any{"baseline_share_price_asset_units": "1000000"}}

	// Act
	result, err := CollectBootstrapFragments(context.Background(), []BootstrapModule{module})

	// Assert
	if err != nil {
		t.Fatalf("collect bootstrap fragments: %v", err)
	}
	if result["share_price_loss"].(map[string]any)["baseline_share_price_asset_units"] != "1000000" {
		t.Fatalf("expected share price baseline fragment")
	}
}

type fakeBootstrapModule struct {
	id       core.MonitorModuleID
	fragment map[string]any
}

func (module fakeBootstrapModule) ID() core.MonitorModuleID {
	return module.id
}

func (module fakeBootstrapModule) Bootstrap(ctx context.Context) (map[string]any, error) {
	return module.fragment, nil
}
