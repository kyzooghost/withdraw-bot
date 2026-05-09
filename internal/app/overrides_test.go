package app

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"withdraw-bot/internal/config"
	"withdraw-bot/internal/core"
	"withdraw-bot/internal/monitor"
	morphomod "withdraw-bot/internal/monitor/modules/morpho"
	"withdraw-bot/internal/storage"
)

const (
	testStaticThresholdValue  = "50"
	testInvalidThresholdValue = "abc"
	testOverrideUserID        = int64(1)
	testDifferentOverrideUser = int64(2)
)

func TestThresholdOverridesAffectSharePriceMonitorDecision(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := storage.NewRepositories(db)
	if err := repos.UpsertThresholdOverride(ctx, string(core.ModuleSharePriceLoss), moduleConfigKeyLossWarnBPS, testStaticThresholdValue, testOverrideUserID, time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("upsert threshold override: %v", err)
	}
	base := morphomod.SharePriceModule{
		BaselineSharePrice: big.NewInt(1000),
		WarnBPS:            200,
		UrgentBPS:          300,
		Reader:             fixedSharePriceReader{price: big.NewInt(990)},
		Clock:              core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}
	wrapped := withThresholdOverrides([]monitor.Module{base}, repos, 6)[0]

	// Act
	result, err := wrapped.Monitor(ctx)

	// Assert
	if err != nil {
		t.Fatalf("monitor with threshold override: %v", err)
	}
	if result.Status != core.MonitorStatusWarn {
		t.Fatalf("expected override to change status to warn, got %s", result.Status)
	}
}

func TestThresholdProviderRejectsInvalidValueBeforeConfirmation(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	provider := thresholdProvider{repos: storage.NewRepositories(db), assetDecimals: 6}

	// Act
	_, err = provider.BuildSetConfirmation(ctx, testOverrideUserID, string(core.ModuleWithdrawLiquidity), moduleConfigKeyIdleWarnThresholdUSDC, testInvalidThresholdValue)

	// Assert
	if err == nil {
		t.Fatal("expected invalid threshold value to be rejected")
	}
}

func TestThresholdProviderRejectsConfirmationFromDifferentUser(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := storage.NewRepositories(db)
	provider := thresholdProvider{repos: repos, assetDecimals: 6, clock: core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)}}
	if _, err := provider.BuildSetConfirmation(ctx, testOverrideUserID, string(core.ModuleSharePriceLoss), moduleConfigKeyLossWarnBPS, testStaticThresholdValue); err != nil {
		t.Fatalf("build threshold confirmation: %v", err)
	}

	// Act
	_, err = provider.Confirm(ctx, testDifferentOverrideUser, testThresholdConfirmID)

	// Assert
	if err == nil {
		t.Fatal("expected different user confirmation to be rejected")
	}
	overrides, err := repos.ListThresholdOverrides(ctx)
	if err != nil {
		t.Fatalf("list threshold overrides: %v", err)
	}
	if len(overrides) != 0 {
		t.Fatalf("expected no override after rejected confirmation, got %+v", overrides)
	}
	if _, err := provider.Confirm(ctx, testOverrideUserID, testThresholdConfirmID); err != nil {
		t.Fatalf("expected requester to still be able to confirm: %v", err)
	}
}

func TestThresholdProviderListIncludesStaticThresholdsAndOverrides(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := storage.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := storage.NewRepositories(db)
	cfg := config.Config{Modules: map[string]config.ModuleConfig{
		string(core.ModuleSharePriceLoss): {
			moduleConfigKeyEnabled:     true,
			moduleConfigKeyLossWarnBPS: testStaticThresholdValue,
		},
	}}
	if err := repos.UpsertThresholdOverride(ctx, string(core.ModuleSharePriceLoss), moduleConfigKeyLossWarnBPS, testThresholdValue, testOverrideUserID, time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("upsert threshold override: %v", err)
	}
	provider := thresholdProvider{repos: repos, config: cfg}

	// Act
	result, err := provider.List(ctx)

	// Assert
	if err != nil {
		t.Fatalf("list thresholds: %v", err)
	}
	if !strings.Contains(result, "share_price_loss loss_warn_bps=50") {
		t.Fatalf("expected static threshold, got %q", result)
	}
	if !strings.Contains(result, "share_price_loss loss_warn_bps=75") {
		t.Fatalf("expected active override, got %q", result)
	}
}

type fixedSharePriceReader struct {
	price *big.Int
}

func (reader fixedSharePriceReader) CurrentSharePrice(ctx context.Context) (*big.Int, error) {
	return new(big.Int).Set(reader.price), nil
}
