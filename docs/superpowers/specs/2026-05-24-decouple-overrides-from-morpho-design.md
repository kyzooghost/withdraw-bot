# Decouple overrides.go from Morpho Protocol

**Date:** 2026-05-24
**Status:** Approved
**Continues:** 2026-05-23-decouple-modules-from-morpho-design.md

## Problem

`internal/app/overrides.go` imports `withdraw-bot/internal/monitor/modules/morpho` and contains Morpho-specific logic: type-switching on Morpho module structs, constructing Morpho modules for validation, and implementing noop readers for Morpho interfaces. This contradicts the protocol-agnostic architecture established in the previous refactoring.

## Approach

Extend the existing `ModuleFactory` interface with two new methods that encapsulate the Morpho-specific override logic. The `app` package orchestrates (fetch overrides from DB, pass to factory) with zero protocol knowledge.

## Interface Changes

The `ModuleFactory` interface in `internal/app/runtime.go` grows from 1 to 3 methods:

```go
type ModuleFactory interface {
    BuildModules(cfg config.Config, ethClient ethereum.MultiClient, vault, owner, receiver common.Address, exitSim core.ExitSimulator) ([]monitor.Module, error)
    ApplyOverrides(module monitor.Module, overrides []storage.ThresholdOverride, assetDecimals uint8) (monitor.Module, error)
    ValidateThresholdChange(ctx context.Context, cfg config.Config, assetDecimals uint8, moduleID string, key string, value string, currentOverrides []storage.ThresholdOverride) error
}
```

- `ApplyOverrides` - given a module and overrides, return the module with overrides applied. The factory type-switches on its own types internally.
- `ValidateThresholdChange` - given config, a proposed key/value, and existing overrides, validate the change is legal (value parseable, effective thresholds consistent). Replaces both `validateThresholdValue` and `validateEffectiveThreshold`.

## What Moves to `internal/morpho`

A new file `internal/morpho/overrides.go` contains:

- `applyThresholdOverrides` (type-switch dispatcher)
- `applySharePriceOverrides`, `applyWithdrawLiquidityOverrides`, `applyVaultStateOverrides`
- `validateThresholdValue` (key-to-parser mapping)
- `validateEffectiveThreshold` logic (build baseline, apply proposed override, call ValidateConfig)
- `sharePriceThresholdConfig`, `withdrawLiquidityThresholdConfig`, `vaultStateThresholdConfig`
- `parseThresholdBPS`, `parseThresholdAssetUnits`
- All noop readers (`noopSharePriceReader`, `noopIdleAssetReader`, `noopExitSimulator`, `noopVaultStateReader`)
- Error constants (`errThresholdBPSInteger`, `errThresholdBPSRange`, `errThresholdSeverity`, `errThresholdAssetUnits`, `errThresholdDuration`)
- Validation address constants (`validationOwnerAddress`, `validationReceiverAddress`, `validationVaultAddress`)

These become the implementation of `ApplyOverrides` and `ValidateThresholdChange` on the `Factory` struct.

## What Stays in `internal/app/overrides.go`

- `thresholdOverrideModule` struct and methods (ID, ValidateConfig, Bootstrap, Monitor, effective)
- `withThresholdOverrides` - now takes a `factory ModuleFactory` param
- `overridesForModule` - generic module ID filtering
- `upsertEffectiveOverride` - pure data manipulation
- `telegramThresholdRequest` struct
- Module config key constants (used by providers.go for the telegram threshold list display)

The `thresholdOverrideModule` struct gains a `factory ModuleFactory` field. Its `effective()` method calls `factory.ApplyOverrides` instead of the local function.

## Wiring Changes

1. `withThresholdOverrides` signature gains `factory ModuleFactory` param. Call site in `runtime.go` passes `runtimeDeps.moduleFactory`.
2. `thresholdProvider` struct gains `factory ModuleFactory` field. `BuildSetConfirmation` and `Confirm` call `factory.ValidateThresholdChange(...)` instead of the two local validate functions.
3. `thresholdProvider` is constructed in `runtime.go` where the factory is already available.

## Import Removal

After this change, `internal/app/overrides.go` no longer imports:
- `withdraw-bot/internal/monitor/modules/morpho`
- `math/big`
- `strconv`

The `internal/app` package has zero Morpho protocol imports in production code.

## Testing

- `overrides_test.go` keeps testing the full override path end-to-end. Test files may import `morphomod` and `morpholib` since test coupling to a concrete protocol for integration verification is acceptable.
- `fakeModuleFactory` in `app_test.go` gains stub implementations: `ApplyOverrides` returns module unchanged, `ValidateThresholdChange` returns nil.
- Morpho-specific validation logic retains coverage through the existing integration tests in `app`.

## Success Criteria

- `go build ./...` passes with no import cycles
- `go test ./...` all green
- `internal/app/overrides.go` has no Morpho imports in production code
- The `ModuleFactory` interface is the only protocol boundary the app package touches
