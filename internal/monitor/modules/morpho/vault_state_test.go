package morpho

import (
	"context"
	"testing"
	"time"

	"withdraw-bot/internal/core"
)

func TestDiffVaultStateReportsOwnerChange(t *testing.T) {
	// Arrange
	expected := VaultStateSnapshot{Owner: "0x0000000000000000000000000000000000000001"}
	actual := VaultStateSnapshot{Owner: "0x0000000000000000000000000000000000000002"}

	// Act
	diffs := DiffVaultState(expected, actual)

	// Assert
	if len(diffs) != 1 {
		t.Fatalf("expected one diff, got %d", len(diffs))
	}
	if diffs[0].Field != "owner" {
		t.Fatalf("expected owner field, got %s", diffs[0].Field)
	}
	if diffs[0].Expected != expected.Owner {
		t.Fatalf("expected previous owner %s, got %s", expected.Owner, diffs[0].Expected)
	}
	if diffs[0].Actual != actual.Owner {
		t.Fatalf("expected actual owner %s, got %s", actual.Owner, diffs[0].Actual)
	}
}

func TestDiffVaultStateTreatsNilAndEmptyCollectionsAsEqual(t *testing.T) {
	// Arrange
	expected := VaultStateSnapshot{
		Adapters:                 []string{},
		AllocatorRoles:           map[string]bool{},
		SentinelRoles:            map[string]bool{},
		Timelocks:                map[string]string{},
		Abdicated:                map[string]bool{},
		ForceDeallocatePenalties: map[string]string{},
	}
	actual := VaultStateSnapshot{}

	// Act
	diffs := DiffVaultState(expected, actual)

	// Assert
	if len(diffs) != 0 {
		t.Fatalf("expected no diffs, got %d", len(diffs))
	}
}

func TestDiffVaultStateReportsMapChangeWithDeterministicEvidence(t *testing.T) {
	// Arrange
	expected := VaultStateSnapshot{
		Timelocks: map[string]string{
			"setSendSharesGate":    "10",
			"setReceiveSharesGate": "20",
		},
	}
	actual := VaultStateSnapshot{
		Timelocks: map[string]string{
			"setSendSharesGate":    "10",
			"setReceiveSharesGate": "30",
		},
	}

	// Act
	diffs := DiffVaultState(expected, actual)

	// Assert
	if len(diffs) != 1 {
		t.Fatalf("expected one diff, got %d", len(diffs))
	}
	if diffs[0].Field != vaultStateFieldTimelocks {
		t.Fatalf("expected timelocks field, got %s", diffs[0].Field)
	}
	expectedEvidence := `{"setReceiveSharesGate":"20","setSendSharesGate":"10"}`
	if diffs[0].Expected != expectedEvidence {
		t.Fatalf("expected deterministic evidence %s, got %s", expectedEvidence, diffs[0].Expected)
	}
}

func TestVaultStateModuleReturnsWarnWhenChangeSeverityIsWarn(t *testing.T) {
	// Arrange
	observedAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	module := VaultStateModule{
		Reader: fakeVaultStateReader{
			snapshot: VaultStateSnapshot{Owner: "0x0000000000000000000000000000000000000002"},
		},
		Baseline:       VaultStateSnapshot{Owner: "0x0000000000000000000000000000000000000001"},
		ChangeSeverity: core.SeverityWarn,
		Clock:          core.FixedClock{Value: observedAt},
	}

	// Act
	result, err := module.Monitor(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("monitor vault state: %v", err)
	}
	if result.Status != core.MonitorStatusWarn {
		t.Fatalf("expected warn status, got %s", result.Status)
	}
	if result.ModuleID != core.ModuleVaultState {
		t.Fatalf("expected module %s, got %s", core.ModuleVaultState, result.ModuleID)
	}
	if !result.ObservedAt.Equal(observedAt) {
		t.Fatalf("expected observed time %s, got %s", observedAt, result.ObservedAt)
	}
	assertFinding(t, result, core.FindingVaultStateDiff, core.SeverityWarn)
}

func TestVaultStateModuleReturnsUrgentWhenChangeSeverityIsUrgent(t *testing.T) {
	// Arrange
	module := VaultStateModule{
		Reader: fakeVaultStateReader{
			snapshot: VaultStateSnapshot{Owner: "0x0000000000000000000000000000000000000002"},
		},
		Baseline:       VaultStateSnapshot{Owner: "0x0000000000000000000000000000000000000001"},
		ChangeSeverity: core.SeverityUrgent,
		Clock:          core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	result, err := module.Monitor(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("monitor vault state: %v", err)
	}
	if result.Status != core.MonitorStatusUrgent {
		t.Fatalf("expected urgent status, got %s", result.Status)
	}
	assertFinding(t, result, core.FindingVaultStateDiff, core.SeverityUrgent)
}

func TestTrackedTimelockSelectorKeysIncludesRequiredSelectors(t *testing.T) {
	// Arrange
	required := []string{
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
		"setManagementFeeRecipient",
		"setForceDeallocatePenalty",
	}
	keys := TrackedTimelockSelectorKeys()
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		seen[key] = true
	}

	// Act and Assert
	for _, key := range required {
		if !seen[key] {
			t.Fatalf("expected tracked timelock selector %s", key)
		}
	}
	if len(keys) != len(seen) {
		t.Fatalf("expected tracked timelock selectors to be unique, got %v", keys)
	}
}

func TestTrackedTimelockSelectorKeysReturnsCopy(t *testing.T) {
	// Arrange
	keys := TrackedTimelockSelectorKeys()
	keys[0] = "mutated"

	// Act
	next := TrackedTimelockSelectorKeys()

	// Assert
	if next[0] == "mutated" {
		t.Fatal("expected tracked timelock selector keys copy")
	}
}

type fakeVaultStateReader struct {
	snapshot VaultStateSnapshot
	err      error
}

func (reader fakeVaultStateReader) CurrentVaultState(ctx context.Context) (VaultStateSnapshot, error) {
	if reader.err != nil {
		return VaultStateSnapshot{}, reader.err
	}
	return reader.snapshot, nil
}
