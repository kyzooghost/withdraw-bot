# Morpho Withdraw Bot Design

Date: 2026-05-09

## Goal

Build a single long-running Go daemon that monitors one DeFi position, reports risk state to one Telegram chat, and can automatically withdraw the full position while the operator is offline.

V1 targets the Morpho Vault V2 Gauntlet USDC Prime vault on Ethereum mainnet:

- Vault address: `0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0`
- Chain: Ethereum mainnet
- Asset: USDC
- Exit primitive: `redeem(allShares, receiver, owner)`

The daemon is intentionally single-position in v1:

- One running instance
- One DeFi position
- One Telegram chat
- One signer EOA that owns the vault shares

The implementation must still use protocol-ready interfaces so future protocols or interaction surfaces can be added with minimal blast radius.

## External References

- Morpho Vault V2 contract documentation: https://morpho-org-vault-v2.mintlify.app/contracts/vault-v2
- Morpho vault integration guide: https://legacy.docs.morpho.org/morpho-vaults/tutorials/integrate-vaults/

## Out Of Scope For V1

- Multi-position or multi-chat management
- Multi-protocol registry
- KMS, hardware wallet, Safe, or contract wallet signing
- Runtime Telegram bootstrap for baseline config
- Runtime Telegram mutation of authorized users
- Prometheus metrics endpoint
- Web dashboard
- Discord or other chat integrations
- Implementing every future broad DeFi risk metric

## Architecture

V1 is a single Go binary with three modes:

- `monitor`: long-running daemon
- `bootstrap`: print candidate baseline config fragments and exit
- `config-check`: validate config, env, RPC connectivity, signer, Telegram auth shape, gas caps, receiver, and module baselines

Core services:

- `Config`: loads YAML config and env secrets
- `Storage`: SQLite repositories and embedded migrations
- `EthereumClient`: RPC wrapper with primary and fallback clients
- `SignerService`: abstracts signing
- `WithdrawService`: owns manual and automatic full-exit orchestration
- `MonitorService`: runs monitor loop and caches latest results
- `AlertService`: maps monitor findings to alerts and withdrawal actions
- `InteractionService`: generic chat/user interaction boundary
- `ReportService`: renders `/stats` and daily reports
- `EventLogService`: stores sanitized operational events for `/logs`

Morpho-specific code must sit behind adapters and modules:

- `MorphoVaultV2WithdrawAdapter`
- `MorphoSharePriceLossModule`
- `MorphoWithdrawLiquidityModule`
- `MorphoVaultStateBaselineModule`

## Core Interfaces

The core interfaces should be small, explicit, and independent of Telegram or Morpho-specific concerns.

```go
type SignerService interface {
    Address(ctx context.Context) (common.Address, error)
    SignTransaction(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
}
```

V1 implementation:

- `PrivateKeySignerService`
- Reads private key from env only
- Never logs private keys or sensitive env values

```go
type WithdrawAdapter interface {
    ID() WithdrawAdapterID
    Position(ctx context.Context) (PositionSnapshot, error)
    BuildFullExit(ctx context.Context, req FullExitRequest) (TxCandidate, error)
    SimulateFullExit(ctx context.Context, req FullExitRequest) (FullExitSimulation, error)
}
```

For Morpho Vault V2, `BuildFullExit` and `SimulateFullExit` use `redeem(allShares, receiver, owner)`.

```go
type MonitorModule interface {
    ID() MonitorModuleID
    ValidateConfig(ctx context.Context) error
    Bootstrap(ctx context.Context) (ModuleBootstrapResult, error)
    Monitor(ctx context.Context) (MonitorResult, error)
}
```

Bootstrapping belongs on `MonitorModule` so each module owns the baseline data it needs. `bootstrap` mode explicitly calls `Bootstrap()` on registered modules, prints proposed config fragments, and exits without mutating config.

```go
type InteractionService interface {
    Start(ctx context.Context) error
    SendAlert(ctx context.Context, msg AlertMessage) error
    SendReport(ctx context.Context, msg ReportMessage) error
    SendCommandResponse(ctx context.Context, msg CommandResponse) error
}
```

V1 implementation:

- `TelegramInteractionService`

This keeps monitor, withdraw, and report logic independent of Telegram-specific APIs.

## Ethereum RPC Policy

The app supports one primary RPC URL and one or more fallback RPC URLs.

Read calls:

- Try primary first
- Rotate through fallbacks when primary fails
- Continue module execution where possible if one module fails

Transaction submission:

- Submit through primary RPC first
- Use fallback RPC only if the submission failure is clearly pre-broadcast
- Do not broadcast blindly to multiple RPCs

This policy reduces duplicate transaction noise while still allowing recovery from provider incidents.

## Monitor Model

`MonitorService` runs every configured interval. Default interval is `5m`.

Each monitor tick calls every registered `MonitorModule`. Results are stored in SQLite and kept in memory for fast `/stats`.

```go
type MonitorResult struct {
    ModuleID   MonitorModuleID
    Status     MonitorStatus
    ObservedAt time.Time
    Metrics    []Metric
    Findings   []Finding
}
```

Supported statuses:

- `OK`
- `WARN`
- `URGENT`
- `UNKNOWN`

Findings include severity and evidence:

```go
type Finding struct {
    Key      FindingKey
    Severity Severity
    Message  string
    Evidence map[string]string
}
```

Alerts repeat every monitor interval while a condition remains true.

## V1 Monitor Modules

### `share_price_loss`

Purpose:

- Detect loss in vault share price.

Inputs:

- Static baseline share price from config
- Previous successful persisted tick
- Current onchain share price

Behavior:

- Compare current share price against static baseline
- Compare current share price against previous successful tick
- Emit `WARN` if either loss crosses warn bps
- Emit `URGENT` if either loss crosses urgent bps

Urgent findings trigger automatic full exit.

### `withdraw_liquidity`

Purpose:

- Detect degraded ability to exit the position.

Inputs:

- Current signer share balance
- Vault idle USDC balance
- `previewRedeem(allShares)`
- `eth_call` simulation of `redeem(allShares, receiver, owner)`

Behavior:

- Report `idleAssets`
- Report `expectedExitAssets`
- Report `fullExitSimulation`
- Emit `WARN` if idle liquidity falls below warn threshold
- Emit `URGENT` if idle liquidity falls below urgent threshold
- Emit `URGENT` if full-exit simulation fails

Idle thresholds use decimal USDC strings in config, for example:

```yaml
idle_warn_threshold_usdc: "1000000"
idle_urgent_threshold_usdc: "500000"
```

Urgent findings trigger automatic full exit.

### `vault_state_baseline`

Purpose:

- Detect unexpected Morpho Vault V2 control, withdrawal, role, adapter, fee, timelock, or gate state changes.

Inputs:

- Static baseline config generated by `bootstrap` mode and reviewed before deployment
- Current readable Morpho Vault V2 state

Behavior:

- Compare exact raw values against baseline
- Any detected change is `URGENT` by default
- Module config may remap detected changes to `WARN`

The exact readable fields will be finalized during implementation after inspecting the Morpho Vault V2 ABI. V1 should include all readable fields exposed by the ABI that are relevant to deposits, withdrawals, privileged control, fees, adapters, timelocks, gates, guardians, or allocator/curator roles.

Urgent findings trigger automatic full exit unless the module config maps them to warn-only.

## Monitor Failure And Staleness

A single module failure must not immediately withdraw funds.

Failure behavior:

- Failed module result is `UNKNOWN`
- Warning alert is sent
- Last successful snapshot remains available but is marked stale
- Other modules continue running

Each module has its own stale-data threshold. If a module remains unknown or stale longer than its configured urgent threshold, it emits an `URGENT` `stale_data` finding.

Stale-data urgent findings trigger automatic full exit.

`/stats` overall status is the worst known severity. `UNKNOWN` modules are called out separately with age since last successful snapshot.

## Withdraw Execution

`WithdrawService` owns full-exit execution. It delegates protocol-specific transaction construction and simulation to `WithdrawAdapter`.

V1 assumptions:

- Signer address is the owner of the Morpho vault shares
- Receiver defaults to owner
- Receiver can be overridden in YAML config

Execution flow:

1. Read current share balance.
2. If shares are zero, record a no-op withdrawal attempt and notify Telegram.
3. Build full-exit transaction through `WithdrawAdapter`.
4. Simulate with `eth_call`.
5. Estimate gas.
6. Set EIP-1559 fee fields.
7. Sign through `SignerService`.
8. Submit through primary RPC.
9. Use fallback RPC only if failure is clearly pre-broadcast.
10. Poll receipt.
11. Store tx hash, status, gas used, fee metadata, trigger reason, simulation summary, timestamps, and failure reason in SQLite.

The app must never store or log private keys, RPC URLs, Telegram tokens, or other sensitive secret values.

## Urgent Auto-Withdraw

Urgent monitor findings trigger automatic full exit. This is a core v1 requirement so the bot can withdraw while the operator is offline.

On every urgent monitor tick:

1. Send Telegram urgent alert.
2. Re-check current shares.
3. Re-simulate full exit.
4. If shares remain and simulation succeeds, try to get an exit transaction mined.
5. If no withdrawal tx is pending, submit one.
6. If a withdrawal tx is pending too long, submit a replacement transaction with bumped gas using the same active withdrawal nonce.

Idempotency means no duplicate concurrent withdrawal attempts. It does not mean trying only once. Repeated urgent ticks keep alerting and keep managing the active exit attempt until shares are gone, the transaction is mined, or a non-retryable error is recorded.

## Gas Policy

V1 uses EIP-1559 transactions.

Initial transaction:

- Estimate gas
- Apply configured gas limit buffer
- Use latest network fee suggestions
- Respect configured max fee caps

Replacement transaction:

- Wait for `replacement_timeout`
- Increase `maxFeePerGas` and `maxPriorityFeePerGas` by configured bump bps
- Respect configured max fee caps
- Keep one active withdrawal nonce

Example config keys:

```yaml
replacement_timeout: 2m
gas_limit_buffer_bps: 2000
fee_bump_bps: 1250
max_fee_per_gas_gwei: "200"
max_priority_fee_per_gas_gwei: "5"
```

## Telegram UX

Telegram is the only v1 interaction implementation.

Authorization:

- One configured Telegram chat ID
- Allowlisted Telegram user IDs in YAML config
- Commands from unknown chats or users are rejected or ignored
- Security-relevant rejects are stored as sanitized event records

Commands:

```text
/stats
/withdraw
/confirm <id>
/thresholds
/threshold set <module> <key> <value>
/logs
/logs info
/help
```

`/stats` returns:

- Overall status
- Latest module status
- Latest metric values
- Active findings
- Unknown modules and stale age
- Last successful monitor time per module
- Pending or recent withdrawal status

`/withdraw`:

- Runs a dry-run full-exit
- Shows vault, owner, receiver, shares, expected assets, simulation result, gas estimate, fee estimate, and confirmation expiry
- Does not sign or submit until confirmed

`/confirm <id>`:

- Confirms a pending manual withdrawal or threshold change
- For withdrawals, re-simulates at confirmation time before signing
- Submits only if current simulation succeeds

`/thresholds`:

- Shows static config thresholds
- Shows active SQLite overrides

`/threshold set <module> <key> <value>`:

- Validates module, threshold key, and value
- Creates pending confirmation
- Persists override to SQLite only after `/confirm`
- Overrides static YAML thresholds after confirmation
- Stores an audit event

`/logs`:

- Shows bounded recent warning, error, and security event records from SQLite
- Does not expose raw local log files

`/logs info`:

- Shows bounded recent info-level operational event records from SQLite

Messages use a small internal Telegram-safe formatter. Dynamic values must be escaped.

## Reports

Daily reports are sent to the same configured Telegram chat as commands and alerts.

Schedule:

- Configured UTC time

Report content:

- Same structured format as `/stats`
- Current overall status
- Module statuses
- Key metrics
- Unknown/stale modules
- Recent alerts
- Recent withdrawal attempts

## Configuration

Non-secret config is YAML. Secrets and sensitive endpoints are env vars.

YAML includes:

- App mode defaults
- Ethereum chain ID
- Morpho vault address
- Asset metadata
- Receiver override
- Telegram chat ID and allowlisted user IDs
- Module thresholds
- Baseline state
- Gas policy
- Report schedule
- SQLite path
- Local rotating log file path and retention

Env includes:

- Private key
- Telegram bot token
- Primary RPC URL
- Fallback RPC URLs

Config keys must include units where values are ambiguous.

Conventions:

- Percentages use bps keys, for example `loss_warn_bps`
- Durations use strict Go duration strings, for example `5m`, `30s`, `24h`
- USDC thresholds use decimal string keys, for example `idle_urgent_threshold_usdc`
- Gas fee values use explicit unit keys, for example `max_fee_per_gas_gwei`

Any env var added during implementation must be reflected in matching `.env.*` template files.

## Storage

SQLite is required in v1.

SQLite stores:

- Monitor snapshots
- Module metrics
- Module findings
- Last successful module state
- Event records for `/logs`
- Threshold overrides
- Pending confirmations
- Withdrawal attempts
- Transaction receipt summaries

Migrations:

- SQL migrations live in the repo
- Migrations are embedded in the Go binary
- Startup applies pending migrations before services start

## Logs And Operational Events

V1 observability:

- Structured stdout logs
- Local rotating log file
- Sanitized SQLite event records for Telegram `/logs`

The local rotating log file is for forensic detail and is not exposed through Telegram.

SQLite event records are bounded, sanitized, and designed for operator debugging without SSH.

Prometheus is out of scope for v1, but service boundaries should make a future metrics exporter straightforward. Core logic should emit structured events or counters through an observability boundary rather than hard-coding a metrics backend.

## Bootstrap

`bootstrap` mode:

1. Load YAML and env needed for read-only chain access.
2. Instantiate registered monitor modules.
3. Call `Bootstrap()` on every module.
4. Print candidate YAML baseline fragments.
5. Exit without mutating config.

Bootstrap is pre-deploy and CLI-only in v1. Runtime Telegram `/bootstrap` is out of scope.

Rationale:

- The baseline should be reviewed before deployment.
- Silently accepting current state as safe would weaken baseline drift detection.
- Runtime baseline approval is powerful enough to deserve a separate design.

## Deployment

V1 deployment artifacts:

- `Dockerfile`
- `docker-compose.yml`
- `.env.*` template files

Runtime layout:

- YAML config is mounted into the container
- Data directory is mounted for SQLite and local logs
- Secrets are passed through compose env file

Systemd is not required for v1. Optional documentation may mention systemd as VPS glue to start Docker Compose after host boot.

## Testing Strategy

Testing should focus on the interface boundaries and safety-critical flows.

All unit tests must follow the local `unit-test` skill guidelines:

- Use Arrange, Act, Assert structure.
- Use test names that describe expected behavior, not implementation details.
- Verify one behavior per test.
- Test public behavior through package APIs and interfaces.
- Mock time, RPC clients, Telegram clients, signers, and transaction submission deterministically.
- Avoid global state and test-order dependencies.
- Prefer real domain value objects where cheap, and use fakes only for external systems or hard-to-control boundaries.
- Reuse shared test fixtures or factories when the same setup appears in two or more test files.

Unit tests:

- Config parsing and validation
- Threshold normalization
- Monitor result severity mapping
- Stale-data handling
- Vault state baseline diffing
- Telegram command authorization
- Telegram formatter escaping
- Threshold override confirmation flow
- WithdrawService no-op when shares are zero
- WithdrawService simulation failure path
- Gas replacement policy

Integration-style tests with fakes:

- Monitor tick stores results and repeats alerts
- Urgent finding triggers auto-withdraw orchestration
- Pending withdrawal replacement after timeout
- `/withdraw` dry-run followed by `/confirm` re-simulation
- RPC fallback read behavior
- Transaction submission fallback only on pre-broadcast failure

External-chain tests should use mocked Ethereum clients or a local simulated backend. Mainnet RPC tests should be opt-in only.

## Open Implementation Details To Resolve During Planning

- Exact Go package layout
- Exact SQLite schema
- Exact Morpho Vault V2 ABI source and field list for `vault_state_baseline`
- Telegram library choice
- Ethereum client wrapper details
- Log rotation library choice
- Migration library choice

These are implementation choices, not unresolved product requirements.
