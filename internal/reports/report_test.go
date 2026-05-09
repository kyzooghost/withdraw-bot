package reports

import (
	"testing"

	"withdraw-bot/internal/core"
)

const expectedStatsReport = `Status: URGENT

share_price_loss: OK
- share_price_asset_units: 1000000 asset_units

withdraw_liquidity: URGENT
- idle_assets: 499999 asset_units
- urgent: idle liquidity crossed threshold`

func TestRenderStatsReturnsDeterministicReport(t *testing.T) {
	// Arrange
	results := map[core.MonitorModuleID]core.MonitorResult{
		core.ModuleWithdrawLiquidity: {
			ModuleID: core.ModuleWithdrawLiquidity,
			Status:   core.MonitorStatusUrgent,
			Metrics: []core.Metric{{
				Key:   "idle_assets",
				Value: "499999",
				Unit:  "asset_units",
			}},
			Findings: []core.Finding{{
				Key:      core.FindingIdleLiquidity,
				Severity: core.SeverityUrgent,
				Message:  "idle liquidity crossed threshold",
			}},
		},
		core.ModuleSharePriceLoss: {
			ModuleID: core.ModuleSharePriceLoss,
			Status:   core.MonitorStatusOK,
			Metrics: []core.Metric{{
				Key:   "share_price_asset_units",
				Value: "1000000",
				Unit:  "asset_units",
			}},
		},
	}

	// Act
	result := RenderStats(results)

	// Assert
	if result != expectedStatsReport {
		t.Fatalf("expected report:\n%s\n\ngot:\n%s", expectedStatsReport, result)
	}
}
