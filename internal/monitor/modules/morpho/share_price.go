package morpho

import (
	"context"
	"fmt"
	"math/big"

	"withdraw-bot/internal/core"
)

const (
	sharePriceBaselineConfigKey = "baseline_share_price_asset_units"
	sharePriceMetricKey         = "share_price_asset_units"
	sharePriceMetricUnit        = "asset_units"
	sharePriceBaselineLossKey   = "baseline_loss_bps"
	sharePricePreviousLossKey   = "previous_loss_bps"
)

type SharePriceReader interface {
	CurrentSharePrice(ctx context.Context) (*big.Int, error)
}

type SharePriceModule struct {
	BaselineSharePrice *big.Int
	PreviousSharePrice *big.Int
	WarnBPS            int64
	UrgentBPS          int64
	Reader             SharePriceReader
	Clock              core.Clock
}

func (module SharePriceModule) ID() core.MonitorModuleID {
	return core.ModuleSharePriceLoss
}

func (module SharePriceModule) ValidateConfig(ctx context.Context) error {
	if module.BaselineSharePrice == nil || module.BaselineSharePrice.Sign() <= 0 {
		return fmt.Errorf("%s %s must be positive", core.ModuleSharePriceLoss, sharePriceBaselineConfigKey)
	}
	if module.WarnBPS <= 0 {
		return fmt.Errorf("%s warn threshold must be positive", core.ModuleSharePriceLoss)
	}
	if module.UrgentBPS <= 0 {
		return fmt.Errorf("%s urgent threshold must be positive", core.ModuleSharePriceLoss)
	}
	if module.WarnBPS > module.UrgentBPS {
		return fmt.Errorf("%s warn threshold must be less than or equal to urgent threshold", core.ModuleSharePriceLoss)
	}
	if module.Reader == nil {
		return fmt.Errorf("%s reader is required", core.ModuleSharePriceLoss)
	}
	return nil
}

func (module SharePriceModule) Bootstrap(ctx context.Context) (map[string]any, error) {
	price, err := module.Reader.CurrentSharePrice(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{sharePriceBaselineConfigKey: price.String()}, nil
}

func (module SharePriceModule) Monitor(ctx context.Context) (core.MonitorResult, error) {
	current, err := module.Reader.CurrentSharePrice(ctx)
	if err != nil {
		return core.MonitorResult{}, err
	}
	if current == nil || current.Sign() <= 0 {
		return core.MonitorResult{}, fmt.Errorf("%s current share price must be positive", core.ModuleSharePriceLoss)
	}
	clock := module.Clock
	if clock == nil {
		clock = core.SystemClock{}
	}
	baselineLoss := lossBPS(module.BaselineSharePrice, current)
	previousLoss := int64(0)
	if module.PreviousSharePrice != nil && module.PreviousSharePrice.Sign() > 0 {
		previousLoss = lossBPS(module.PreviousSharePrice, current)
	}
	status := core.MonitorStatusOK
	findings := []core.Finding{}
	if baselineLoss >= module.UrgentBPS || previousLoss >= module.UrgentBPS {
		status = core.MonitorStatusUrgent
		findings = append(findings, core.Finding{
			Key:      core.FindingSharePriceLoss,
			Severity: core.SeverityUrgent,
			Message:  "share price loss crossed urgent threshold",
			Evidence: map[string]string{
				sharePriceBaselineLossKey: fmt.Sprint(baselineLoss),
				sharePricePreviousLossKey: fmt.Sprint(previousLoss),
			},
		})
	} else if baselineLoss >= module.WarnBPS || previousLoss >= module.WarnBPS {
		status = core.MonitorStatusWarn
		findings = append(findings, core.Finding{
			Key:      core.FindingSharePriceLoss,
			Severity: core.SeverityWarn,
			Message:  "share price loss crossed warn threshold",
			Evidence: map[string]string{
				sharePriceBaselineLossKey: fmt.Sprint(baselineLoss),
				sharePricePreviousLossKey: fmt.Sprint(previousLoss),
			},
		})
	}
	return core.MonitorResult{
		ModuleID:   module.ID(),
		Status:     status,
		ObservedAt: clock.Now(),
		Metrics:    []core.Metric{{Key: sharePriceMetricKey, Value: current.String(), Unit: sharePriceMetricUnit}},
		Findings:   findings,
	}, nil
}

func lossBPS(reference *big.Int, current *big.Int) int64 {
	if current.Cmp(reference) >= 0 {
		return 0
	}
	loss := new(big.Int).Sub(reference, current)
	scaled := new(big.Int).Mul(loss, big.NewInt(10_000))
	scaled.Div(scaled, reference)
	return scaled.Int64()
}
