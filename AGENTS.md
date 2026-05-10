# withdraw-bot - Agent Guide and Architecture Index

Generated: 2026-05-10

This is the canonical repo guide for agents working in `withdraw-bot`. It summarizes the codebase, operational model, commands, tests, and local rules that matter before changing code.

## 1. Purpose

`withdraw-bot` is a Go daemon for monitoring one Morpho Vault V2 position on Ethereum mainnet and operating it through Telegram. The intended product is a long-running process that watches risk conditions, reports state to one Telegram chat, and supports a full-position exit through Morpho Vault V2 `redeem(shares, receiver, owner)`.

V1 is centered on the Gauntlet USDC Prime Morpho vault:

```text
Chain:  Ethereum mainnet, chain_id 1
Vault:  0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0
Asset:  USDC, 6 decimals
Exit:   redeem(allShares, receiver, owner)
```

Current behavior note: the code contains `internal/app.AlertService` and `withdraw.Service.ExecuteFullExit`, but no runtime call from `monitor.Service.RunLoop` into `AlertService.HandleMonitorResults` was found during the 2026-05-10 discovery. Telegram `/withdraw` currently performs `DryRunFullExit`. Verify runtime wiring before claiming urgent monitor findings automatically broadcast withdrawals.

## 2. Architecture

The application has one binary and three modes:

```text
cmd/withdraw-bot/main.go
  parses: monitor | bootstrap | config-check
  calls:  app.Run(ctx, mode, configPath)
```

Runtime data flow:

```text
CLI or Docker
  |
  v
internal/app.Run
  |
  +-- buildRuntime
  |     +-- config.Load(config/config.example.yaml or operator config)
  |     +-- config.LoadSecretsFromEnv
  |     +-- ethereum.DialMulti(primary RPC, fallback RPCs)
  |     +-- signer.PrivateKeyService
  |     +-- morpho.VaultClient
  |     +-- withdraw.MorphoAdapter
  |     +-- monitor modules
  |
  +-- monitor mode
  |     +-- SQLite storage in app.data_dir
  |     +-- rotating JSON logs
  |     +-- monitor.Service.RunLoop
  |     +-- telegram.Service.Start
  |     +-- withdraw.Service for dry-run and execution primitives
  |
  +-- bootstrap mode
  |     +-- prints module baseline YAML fragments
  |
  +-- config-check mode
        +-- validates config, RPC chain ID, signer, receiver
```

Monitor loop data path:

```text
monitor.Service.RunLoop
  -> RunOnce
  -> module.ValidateConfig
  -> module.Monitor
  -> Morpho vault reads and withdrawal simulation
  -> core.MonitorResult
  -> in-memory latest snapshot
  -> storage.monitor_snapshots
```

Telegram command path:

```text
telegram.Service.Start
  -> getUpdates polling
  -> chat/user authorization
  -> command dispatch
  -> providers and services
  -> send Telegram response
  -> optional event_records insert
```

Withdrawal execution primitive:

```text
withdraw.Service.ExecuteFullExit
  -> Adapter.Position
  -> Adapter.SimulateFullExit
  -> Adapter.BuildFullExit
  -> gas fee selection and validation
  -> signer.SignTx
  -> primary RPC SendTransaction
  -> storage.withdrawal_attempts
```

## 3. Directory Map

```text
.
+-- cmd/withdraw-bot/main.go
|   Binary entrypoint for monitor, bootstrap, and config-check.
+-- config/config.example.yaml
|   Non-secret runtime config template. Contains placeholder receiver, chat, users, and baseline values.
+-- docs/superpowers/specs/2026-05-09-morpho-withdraw-bot-design.md
|   Original product and architecture spec.
+-- docs/superpowers/plans/2026-05-09-morpho-withdraw-bot-implementation.md
|   Original implementation plan. Its Go version note is stale; go.mod is authoritative.
+-- internal/app
|   Mode dispatch, runtime wiring, bootstrap/config-check, threshold overrides, providers, alerts.
+-- internal/config
|   YAML config structs, validation, env secret loading, unit parsing.
+-- internal/core
|   Shared constants, statuses, DTOs, clone helpers, clock abstraction.
+-- internal/ethereum
|   RPC client abstraction with primary/fallback reads and primary-only transaction sends.
+-- internal/events
|   Event recorder interface.
+-- internal/interactions/telegram
|   Telegram authorization, command parsing, command dispatch, response formatting.
+-- internal/logging
|   JSON slog logger backed by lumberjack log rotation.
+-- internal/monitor
|   Module interface, run loop, snapshot cache, storage writes.
+-- internal/monitor/modules/morpho
|   Share price loss, withdrawal liquidity, and vault state baseline modules.
+-- internal/morpho
|   Morpho Vault V2 ABI fragments, redeem calldata packing, vault reads.
+-- internal/reports
|   Deterministic stats report rendering.
+-- internal/signer
|   Private-key Ethereum signer service.
+-- internal/storage
|   SQLite open path, embedded goose migrations, repositories.
+-- internal/withdraw
    Morpho withdrawal adapter, gas policy, full-exit dry-run and execution service.
```

## 4. Key Components

`cmd/withdraw-bot/main.go` parses the mode and `--config` flag, then exits with the status returned by `app.Run`.

`internal/app/app.go` owns mode dispatch. `runMonitor` starts Telegram polling and the monitor loop in separate goroutines and returns the first service error. `runBootstrap` prints normalized YAML fragments. `runConfigCheck` validates RPC chain ID, signer access, and receiver address consistency.

`internal/app/runtime.go` wires the production runtime: config, env secrets, Ethereum client, signer, Morpho vault client, Morpho withdrawal adapter, monitor modules, SQLite repositories, Telegram service, and withdrawal service.

`internal/config` loads YAML config and required env secrets. It parses basis points, decimal asset units, and gwei strings into typed numeric values.

`internal/core` defines shared IDs and DTOs. Put new module IDs, command names, event types, and finding IDs here instead of scattering raw strings.

`internal/ethereum.MultiClient` tries primary then fallback clients for read operations. `SendTransaction` uses the primary RPC only. Keep that distinction unless the product decision changes.

`internal/monitor.Service` runs modules, stores latest snapshots in memory, and persists monitor results. Module failures become `UNKNOWN` results when recoverable enough to continue the loop.

`internal/monitor/modules/morpho` contains the three monitoring modules:

```text
share_price_loss       compares current share price against baseline and previous value
withdraw_liquidity     checks idle assets and full-exit simulation health
vault_state_baseline   diffs vault control/state fields against configured baseline
```

`internal/interactions/telegram` is the V1 operator interface. Supported commands include `/stats`, `/withdraw`, `/threshold set`, `/confirm`, `/thresholds`, `/logs`, and `/help`.

`internal/storage` opens SQLite, applies embedded migrations from `internal/storage/migrations`, and exposes repositories for snapshots, events, threshold overrides, confirmations, and withdrawal attempts.

`internal/withdraw.Service` handles dry-run and full-exit execution. It re-reads the position before execution, simulates, builds an EIP-1559 transaction, validates gas caps, signs, submits, and records the attempt.

## 5. Configuration And Secrets

Use `.env.example` as the secret template:

```text
WITHDRAW_BOT_PRIVATE_KEY
WITHDRAW_BOT_TELEGRAM_BOT_TOKEN
WITHDRAW_BOT_ETHEREUM_PRIMARY_RPC_URL
WITHDRAW_BOT_ETHEREUM_FALLBACK_RPC_URLS
```

`WITHDRAW_BOT_ETHEREUM_FALLBACK_RPC_URLS` is optional and comma-separated. The other three are required for normal runtime construction.

Use `config/config.example.yaml` as the non-secret template. The example is not production-ready as-is:

```text
ethereum.receiver_address is empty
telegram.chat_id is 0
telegram.allowed_user_ids is empty
vault_state_baseline uses zero-address placeholders
```

When adding or removing an env var, update `.env.example` in the same change. Never commit private keys, RPC URLs, Telegram tokens, or unredacted logs containing them.

Operational state lives under `app.data_dir`, which defaults to `./data`:

```text
data/withdraw-bot.sqlite
data/withdraw-bot.log
```

Both are ignored by git.

## 6. Build And Dev Workflow

The repo uses Go modules with `go 1.25.7` and `toolchain go1.25.10` in `go.mod`.

Primary commands:

```bash
make test
make build
make run-config-check
make run-bootstrap
make run-monitor
```

Equivalent direct commands:

```bash
go test ./... -count=1
go build -o dist/withdraw-bot ./cmd/withdraw-bot
go run ./cmd/withdraw-bot config-check --config config/config.example.yaml
go run ./cmd/withdraw-bot bootstrap --config config/config.example.yaml
go run ./cmd/withdraw-bot monitor --config config/config.example.yaml
```

Docker workflow:

```bash
docker compose config
docker compose build
docker compose up -d
```

No CI workflow, lint config, `justfile`, or repo-provided format target was found during discovery. Use `gofmt` for changed Go files.

## 7. Dependencies

Direct dependencies in `go.mod`:

```text
github.com/ethereum/go-ethereum v1.17.2
github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
github.com/mattn/go-sqlite3 v1.14.44
github.com/pressly/goose/v3 v3.27.1
github.com/stretchr/testify v1.11.1
gopkg.in/natefinch/lumberjack.v2 v2.2.1
gopkg.in/yaml.v3 v3.0.1
```

Dependency versions must be exact. Do not add floating versions, ranges, wildcards, or `latest`.

The Dockerfile uses pinned images:

```text
golang:1.25.10-bookworm@sha256:9422886b8f9b52e88344a24e9b05fd4b37d42233b680019fc3cb6b1fb2f2b0a5
gcr.io/distroless/base-debian12@sha256:9dce90e688a57e59ce473ff7bc4c80bc8fe52d2303b4d99b44f297310bbd2210
```

## 8. Test Infrastructure

There are 167 top-level `func Test...` tests across 26 `_test.go` files as of 2026-05-10.

Testing style:

```text
Framework: standard Go testing package
Assertions: mostly plain if/fatal checks
External services: replaced with local fakes
Storage tests: in-memory SQLite with Open(ctx, ":memory:")
Time: deterministic clocks such as core.FixedClock
Filesystem: t.TempDir where needed
Environment: t.Setenv where needed
```

Important test areas:

```text
internal/app                       runtime modes, module wiring, alerts, config-check, threshold overrides
internal/config                    config validation, env loading, decimal and gwei parsing
internal/core                      status ordering, cloning, fixed clock
internal/ethereum                  multi-RPC fallback, send classification, close behavior
internal/interactions/telegram     authorization, commands, formatting, threshold command validation
internal/logging                   JSON log file output
internal/monitor                   run loop, snapshots, storage errors, module errors
internal/monitor/modules/morpho    share-price, withdrawal-liquidity, vault-state behavior
internal/morpho                    Morpho redeem ABI encoding
internal/reports                   deterministic stats report rendering
internal/signer                    private-key signing and nil rejection
internal/storage                   SQLite migrations and repositories
internal/withdraw                  gas policy, adapter behavior, execution, pending replacement, concurrency
```

Use targeted tests while iterating, then run the full suite before finishing:

```bash
go test ./internal/storage -count=1
go test ./internal/app -run TestRunConfigCheckReturnsChainIDMismatch -count=1
go test ./... -count=1
```

When adding or changing tests, follow the local style first. This repo mostly keeps fakes local to each test file rather than using a shared `testutil` package.

## 9. Implementation Choices And Invariants

Secrets are loaded only from environment variables. Config YAML is non-secret. Runtime errors are sanitized against private key, Telegram token, primary RPC URL, and fallback RPC URLs before returning from `app.Run`.

Read RPCs can fail over from primary to fallback. Transaction broadcast is primary-only. Do not broadcast the same signed transaction through multiple RPC providers unless the product requirement changes and idempotency is re-reviewed.

Bootstrap mode prints baseline fragments and does not mutate config. This keeps baseline drift detection explicit.

Threshold overrides are storage-backed and applied by wrapping monitor modules at runtime. Do not add new override keys as raw strings in multiple places; centralize them with the existing override validation pattern.

SQLite migrations are embedded with `//go:embed migrations/*.sql`. Add new schema changes as new goose migration files under `internal/storage/migrations`.

Withdrawal idempotency is about avoiding duplicate concurrent withdrawal attempts while still allowing replacement or retry behavior. Preserve the concurrency tests in `internal/withdraw/service_test.go` when changing this flow.

## 10. Local Agent Rules

Before changing implementation, inspect 2-3 sibling files in the target directory and match local conventions.

Keep changes surgical. Do not refactor unrelated code or reformat files outside the task.

Use constants or constant objects for new string values. Prefer `internal/core/ids.go` or local package constants depending on scope.

Do not log sensitive values. Mask private keys, Telegram tokens, RPC URLs, transaction signing material, and secret-bearing errors.

If env vars change, update `.env.example` in the same patch.

Run `gofmt` on changed Go files. Run `go test ./... -count=1` before claiming code changes are complete. For docs-only changes, at minimum run `git diff --check`.

Do not touch a local `HANDOVER.md` unless the user explicitly asks. It may exist as untracked session state.

At session start, review the current project lessons file if present under `~/.claude`. The known local lessons file for this repo is `~/.claude/local-withdraw-bot-lessons.md`.

Do not use em dash characters in repo docs or code comments. Use a regular hyphen.

When writing bash scripts, start with:

```bash
set -euo pipefail
```

## 11. Known Gaps And Verification Notes

Automatic urgent withdrawal wiring needs verification. `AlertService.HandleMonitorResults` exists and is tested, but the monitor runtime currently starts only Telegram and `Monitor.RunLoop`.

The implementation plan mentions Go `1.24.0`, but `go.mod` is authoritative and currently declares Go `1.25.7` with toolchain `go1.25.10`.

No repository CI config was found. Branch protection or remote rulesets are not visible from the local checkout.

The root `withdraw-bot` executable may appear as a local ignored build artifact. Do not stage it.
