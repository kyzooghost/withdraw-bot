# Decouple modules.go from Morpho Protocol

## Problem

`internal/app/modules.go` directly imports and constructs Morpho-specific types (`morphomod.SharePriceModule`, `morphomod.WithdrawLiquidityModule`, `morphomod.VaultStateModule`) and contains a concrete `vaultReader` that calls `morphovault.VaultABI` directly. This violates the design spec requirement that "Morpho-specific code must sit behind adapters and modules" and that "core interfaces should be independent of Morpho-specific concerns."

## Approach: Constructor Injection

The concrete protocol factory is constructed in `main.go` and injected into `app.Run()`. No registry, no `init()` magic. The `app` package accepts a `ProtocolFactory` interface and never imports protocol-specific packages.

### Constraints

- Single protocol per bot instance (config-driven)
- Shared config parsing helpers, protocol-specific validation
- No framework overhead - explicit wiring in one place

## Core Interface

New file: `internal/core/protocol.go`

```go
type ProtocolFactory interface {
    BuildModules(cfg config.Config, ethClient ethereum.MultiClient, vault, owner, receiver common.Address) ([]monitor.Module, error)
}
```

`app` accepts this interface. It never knows which protocol is behind it.

## Wiring

In `cmd/withdraw-bot/main.go`:

```go
import morpho "withdraw-bot/internal/morpho"

factory := morpho.NewFactory()
app.Run(ctx, cfg, ethClient, factory)
```

In `app`, module building is a direct delegation:

```go
modules, err := factory.BuildModules(cfg, ethClient, vault, owner, receiver)
```

Testing: app tests pass a `fakeFactory` returning canned modules.

## Morpho Factory

New file: `internal/morpho/factory.go`

```go
type Factory struct{}

func NewFactory() *Factory { return &Factory{} }

func (f *Factory) BuildModules(cfg config.Config, ethClient ethereum.MultiClient, vault, owner, receiver common.Address) ([]monitor.Module, error) {
    reader := vaultReader{Ethereum: ethClient, Vault: vault, AssetDecimals: cfg.Ethereum.AssetDecimals}
    // validate known modules, build enabled modules
}
```

Contains:
- `vaultReader` struct + all its methods (`CurrentSharePrice`, `IdleAssets`, `CurrentVaultState`, `vaultCall`, `vaultUint256`, `vaultAddress`, `vaultBytes`, `vaultAdapters`, etc.)
- `buildSharePriceModule`, `buildWithdrawLiquidityModule`, `buildVaultStateModule`
- `validateModuleConfigs` and `knownModuleIDs`
- `uint256Output` helper

`vaultReader` implements `SharePriceReader`, `IdleAssetReader`, and the vault state reader interface as it does today. It coexists with the existing `VaultClient` in the same package - different read surfaces for different concerns (monitoring vs. withdraw execution).

## Shared Config Helpers

New file: `internal/config/module_helpers.go`

Exported functions usable by any protocol package:

- `ModuleEnabled(moduleConfig ModuleConfig, moduleID core.MonitorModuleID) (bool, error)`
- `EnabledModuleConfig(cfg Config, moduleID core.MonitorModuleID) (ModuleConfig, bool, error)`
- `ModuleString(moduleConfig ModuleConfig, moduleID core.MonitorModuleID, key string) (string, error)`
- `ModuleBigInt(moduleConfig ModuleConfig, moduleID core.MonitorModuleID, key string) (*big.Int, error)`
- `ModuleInt64(moduleConfig ModuleConfig, moduleID core.MonitorModuleID, key string) (int64, error)`
- `ModuleDecimalUnits(moduleConfig ModuleConfig, moduleID core.MonitorModuleID, key string, decimals uint8) (*big.Int, error)`
- `ModuleStruct(moduleConfig ModuleConfig, moduleID core.MonitorModuleID, key string, target any) error`

`ParseDecimalUnits` already lives in `config`, so these are a natural fit.

Protocol-specific validation (`validateModuleConfigs`) stays in the protocol package since it checks against protocol-defined `knownModuleIDs`.

## File Changes

| Action | File | What happens |
|--------|------|--------------|
| Delete | `internal/app/modules.go` | Entire file removed |
| Create | `internal/core/protocol.go` | `ProtocolFactory` interface |
| Create | `internal/morpho/factory.go` | Factory, vaultReader, module builders, validation |
| Create | `internal/config/module_helpers.go` | Exported module config parsing helpers |
| Edit | `internal/app/app.go` | Accept `ProtocolFactory`, call `factory.BuildModules()` |
| Edit | `cmd/withdraw-bot/main.go` | Construct `morpho.NewFactory()`, pass to app |
| Edit | `internal/app/*_test.go` | Use `fakeFactory` instead of real module building |

## Result

- `internal/app` has zero imports from `internal/morpho` or `internal/monitor/modules/morpho`
- The only file that knows about Morpho is `cmd/withdraw-bot/main.go` (one import, one line) and `internal/morpho/` itself
- Adding a future protocol = new package implementing `ProtocolFactory` + a `switch` in `main.go`
