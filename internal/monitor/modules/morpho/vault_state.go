package morpho

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"withdraw-bot/internal/core"
)

const (
	vaultStateFieldOwner                    = "owner"
	vaultStateFieldCurator                  = "curator"
	vaultStateFieldReceiveSharesGate        = "receive_shares_gate"
	vaultStateFieldSendSharesGate           = "send_shares_gate"
	vaultStateFieldReceiveAssetsGate        = "receive_assets_gate"
	vaultStateFieldSendAssetsGate           = "send_assets_gate"
	vaultStateFieldAdapterRegistry          = "adapter_registry"
	vaultStateFieldLiquidityAdapter         = "liquidity_adapter"
	vaultStateFieldLiquidityDataHex         = "liquidity_data_hex"
	vaultStateFieldPerformanceFee           = "performance_fee"
	vaultStateFieldPerformanceFeeRecipient  = "performance_fee_recipient"
	vaultStateFieldManagementFee            = "management_fee"
	vaultStateFieldManagementFeeRecipient   = "management_fee_recipient"
	vaultStateFieldMaxRate                  = "max_rate"
	vaultStateFieldAdapters                 = "adapters"
	vaultStateFieldAllocatorRoles           = "allocator_roles"
	vaultStateFieldSentinelRoles            = "sentinel_roles"
	vaultStateFieldTimelocks                = "timelocks"
	vaultStateFieldAbdicated                = "abdicated"
	vaultStateFieldForceDeallocatePenalties = "force_deallocate_penalties"
	vaultStateDiffCountMetricKey            = "diff_count"
	vaultStateDiffFieldEvidenceKey          = "field"
	vaultStateDiffExpectedEvidenceKey       = "expected"
	vaultStateDiffActualEvidenceKey         = "actual"
	vaultStateMetricUnit                    = "count"
	vaultStateBaselineBootstrapKey          = "baseline"
)

var trackedTimelockSelectorKeys = [...]string{
	"setIsAllocator",
	"setReceiveSharesGate",
	"setSendSharesGate",
	"setReceiveAssetsGate",
	"setSendAssetsGate",
	"setAdapterRegistry",
	"addAdapter",
	"removeAdapter",
	"increaseTimelock",
	"decreaseTimelock",
	"abdicate",
	"setPerformanceFee",
	"setManagementFee",
	"setPerformanceFeeRecipient",
	"setManagementFeeRecipient",
	"setForceDeallocatePenalty",
}

func TrackedTimelockSelectorKeys() []string {
	keys := make([]string, len(trackedTimelockSelectorKeys))
	copy(keys, trackedTimelockSelectorKeys[:])
	return keys
}

type VaultStateSnapshot struct {
	Owner                    string            `json:"owner"`
	Curator                  string            `json:"curator"`
	ReceiveSharesGate        string            `json:"receive_shares_gate"`
	SendSharesGate           string            `json:"send_shares_gate"`
	ReceiveAssetsGate        string            `json:"receive_assets_gate"`
	SendAssetsGate           string            `json:"send_assets_gate"`
	AdapterRegistry          string            `json:"adapter_registry"`
	LiquidityAdapter         string            `json:"liquidity_adapter"`
	LiquidityDataHex         string            `json:"liquidity_data_hex"`
	PerformanceFee           string            `json:"performance_fee"`
	PerformanceFeeRecipient  string            `json:"performance_fee_recipient"`
	ManagementFee            string            `json:"management_fee"`
	ManagementFeeRecipient   string            `json:"management_fee_recipient"`
	MaxRate                  string            `json:"max_rate"`
	Adapters                 []string          `json:"adapters"`
	AllocatorRoles           map[string]bool   `json:"allocator_roles"`
	SentinelRoles            map[string]bool   `json:"sentinel_roles"`
	Timelocks                map[string]string `json:"timelocks"`
	Abdicated                map[string]bool   `json:"abdicated"`
	ForceDeallocatePenalties map[string]string `json:"force_deallocate_penalties"`
}

type StateDiff struct {
	Field    string
	Expected string
	Actual   string
}

type VaultStateReader interface {
	CurrentVaultState(ctx context.Context) (VaultStateSnapshot, error)
}

type VaultStateModule struct {
	Reader         VaultStateReader
	Baseline       VaultStateSnapshot
	ChangeSeverity core.Severity
	Clock          core.Clock
}

func (module VaultStateModule) ID() core.MonitorModuleID {
	return core.ModuleVaultState
}

func (module VaultStateModule) ValidateConfig(ctx context.Context) error {
	if module.Reader == nil {
		return fmt.Errorf("%s reader is required", core.ModuleVaultState)
	}
	if module.ChangeSeverity != "" && module.ChangeSeverity != core.SeverityWarn && module.ChangeSeverity != core.SeverityUrgent {
		return fmt.Errorf("%s change severity must be warn or urgent", core.ModuleVaultState)
	}
	return nil
}

func (module VaultStateModule) Bootstrap(ctx context.Context) (map[string]any, error) {
	if module.Reader == nil {
		return nil, fmt.Errorf("%s reader is required", core.ModuleVaultState)
	}
	snapshot, err := module.Reader.CurrentVaultState(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{vaultStateBaselineBootstrapKey: snapshot}, nil
}

func (module VaultStateModule) Monitor(ctx context.Context) (core.MonitorResult, error) {
	if err := module.ValidateConfig(ctx); err != nil {
		return core.MonitorResult{}, err
	}
	actual, err := module.Reader.CurrentVaultState(ctx)
	if err != nil {
		return core.MonitorResult{}, err
	}
	clock := module.Clock
	if clock == nil {
		clock = core.SystemClock{}
	}
	diffs := DiffVaultState(module.Baseline, actual)
	status := core.MonitorStatusOK
	findings := make([]core.Finding, 0, len(diffs))
	severity := module.resolvedSeverity()
	if len(diffs) > 0 {
		status = monitorStatusForSeverity(severity)
		for _, diff := range diffs {
			findings = append(findings, core.Finding{
				Key:      core.FindingVaultStateDiff,
				Severity: severity,
				Message:  "vault state changed from baseline",
				Evidence: map[string]string{
					vaultStateDiffFieldEvidenceKey:    diff.Field,
					vaultStateDiffExpectedEvidenceKey: diff.Expected,
					vaultStateDiffActualEvidenceKey:   diff.Actual,
				},
			})
		}
	}
	return core.MonitorResult{
		ModuleID:   module.ID(),
		Status:     status,
		ObservedAt: clock.Now(),
		Metrics: []core.Metric{{
			Key:   vaultStateDiffCountMetricKey,
			Value: fmt.Sprint(len(diffs)),
			Unit:  vaultStateMetricUnit,
		}},
		Findings: findings,
	}, nil
}

func (module VaultStateModule) resolvedSeverity() core.Severity {
	if module.ChangeSeverity == core.SeverityWarn {
		return core.SeverityWarn
	}
	return core.SeverityUrgent
}

func monitorStatusForSeverity(severity core.Severity) core.MonitorStatus {
	if severity == core.SeverityWarn {
		return core.MonitorStatusWarn
	}
	return core.MonitorStatusUrgent
}

func DiffVaultState(expected VaultStateSnapshot, actual VaultStateSnapshot) []StateDiff {
	diffs := []StateDiff{}
	diffs = appendStringDiff(diffs, vaultStateFieldOwner, expected.Owner, actual.Owner)
	diffs = appendStringDiff(diffs, vaultStateFieldCurator, expected.Curator, actual.Curator)
	diffs = appendStringDiff(diffs, vaultStateFieldReceiveSharesGate, expected.ReceiveSharesGate, actual.ReceiveSharesGate)
	diffs = appendStringDiff(diffs, vaultStateFieldSendSharesGate, expected.SendSharesGate, actual.SendSharesGate)
	diffs = appendStringDiff(diffs, vaultStateFieldReceiveAssetsGate, expected.ReceiveAssetsGate, actual.ReceiveAssetsGate)
	diffs = appendStringDiff(diffs, vaultStateFieldSendAssetsGate, expected.SendAssetsGate, actual.SendAssetsGate)
	diffs = appendStringDiff(diffs, vaultStateFieldAdapterRegistry, expected.AdapterRegistry, actual.AdapterRegistry)
	diffs = appendStringDiff(diffs, vaultStateFieldLiquidityAdapter, expected.LiquidityAdapter, actual.LiquidityAdapter)
	diffs = appendStringDiff(diffs, vaultStateFieldLiquidityDataHex, expected.LiquidityDataHex, actual.LiquidityDataHex)
	diffs = appendStringDiff(diffs, vaultStateFieldPerformanceFee, expected.PerformanceFee, actual.PerformanceFee)
	diffs = appendStringDiff(diffs, vaultStateFieldPerformanceFeeRecipient, expected.PerformanceFeeRecipient, actual.PerformanceFeeRecipient)
	diffs = appendStringDiff(diffs, vaultStateFieldManagementFee, expected.ManagementFee, actual.ManagementFee)
	diffs = appendStringDiff(diffs, vaultStateFieldManagementFeeRecipient, expected.ManagementFeeRecipient, actual.ManagementFeeRecipient)
	diffs = appendStringDiff(diffs, vaultStateFieldMaxRate, expected.MaxRate, actual.MaxRate)
	diffs = appendComparableDiff(diffs, vaultStateFieldAdapters, expected.Adapters, actual.Adapters)
	diffs = appendComparableDiff(diffs, vaultStateFieldAllocatorRoles, expected.AllocatorRoles, actual.AllocatorRoles)
	diffs = appendComparableDiff(diffs, vaultStateFieldSentinelRoles, expected.SentinelRoles, actual.SentinelRoles)
	diffs = appendComparableDiff(diffs, vaultStateFieldTimelocks, expected.Timelocks, actual.Timelocks)
	diffs = appendComparableDiff(diffs, vaultStateFieldAbdicated, expected.Abdicated, actual.Abdicated)
	diffs = appendComparableDiff(diffs, vaultStateFieldForceDeallocatePenalties, expected.ForceDeallocatePenalties, actual.ForceDeallocatePenalties)
	sort.Slice(diffs, func(i int, j int) bool {
		return diffs[i].Field < diffs[j].Field
	})
	return diffs
}

func appendStringDiff(diffs []StateDiff, field string, expected string, actual string) []StateDiff {
	if expected == actual {
		return diffs
	}
	return append(diffs, StateDiff{Field: field, Expected: expected, Actual: actual})
}

func appendComparableDiff[T any](diffs []StateDiff, field string, expected T, actual T) []StateDiff {
	if reflect.DeepEqual(expected, actual) {
		return diffs
	}
	return append(diffs, StateDiff{Field: field, Expected: stableJSON(expected), Actual: stableJSON(actual)})
}

func stableJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}
