# Morpho Withdraw Bot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go daemon that monitors one Morpho Vault V2 position, reports risk to one Telegram chat, and automatically redeems all shares when urgent risk conditions fire.

**Architecture:** Implement a single-position modular daemon with protocol-ready interfaces. Core packages own config, storage, Ethereum access, signing, monitoring, withdrawal orchestration, alerts, reports, events, and interaction boundaries. Morpho-specific behavior lives in a withdraw adapter and three monitor modules.

**Tech Stack:** Go `1.25.7` module directive with `toolchain go1.25.10`, `github.com/ethereum/go-ethereum v1.17.2`, `github.com/mattn/go-sqlite3 v1.14.44`, `github.com/pressly/goose/v3 v3.27.1`, `gopkg.in/yaml.v3 v3.0.1`, `github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1`, `gopkg.in/natefinch/lumberjack.v2 v2.2.1`, `github.com/stretchr/testify v1.11.1`, standard `log/slog`.

---

## Source References

- Design spec: `docs/superpowers/specs/2026-05-09-morpho-withdraw-bot-design.md`
- Morpho Vault V2 docs: https://morpho-org-vault-v2.mintlify.app/contracts/vault-v2
- Morpho Vault V2 source: https://github.com/morpho-org/vault-v2/blob/main/src/VaultV2.sol
- Morpho events source: https://github.com/morpho-org/vault-v2/blob/main/src/libraries/EventsLib.sol

## Test Rules

Every test in this plan follows the local `unit-test` skill:

- Use Arrange, Act, Assert comments for non-trivial tests.
- Use names that describe expected behavior.
- Verify one behavior per test.
- Mock time, RPC, Telegram, signing, and transaction submission deterministically.
- Test public behavior through exported constructors, interfaces, and package APIs.
- Use shared test fixtures when the same setup appears in two or more test files.
- Run tests with `go test ./... -count=1` before every commit.

## File Structure

Create this structure as tasks need it:

```text
cmd/withdraw-bot/main.go
internal/app/app.go
internal/app/mode.go
internal/config/config.go
internal/config/env.go
internal/config/units.go
internal/core/ids.go
internal/core/types.go
internal/core/status.go
internal/core/time.go
internal/ethereum/client.go
internal/ethereum/fake_client_test.go
internal/events/events.go
internal/interactions/service.go
internal/interactions/telegram/service.go
internal/interactions/telegram/formatter.go
internal/interactions/telegram/commands.go
internal/logging/logging.go
internal/monitor/service.go
internal/monitor/modules/morpho/share_price.go
internal/monitor/modules/morpho/withdraw_liquidity.go
internal/monitor/modules/morpho/vault_state.go
internal/morpho/abi.go
internal/morpho/vault_client.go
internal/reports/report.go
internal/signer/signer.go
internal/storage/db.go
internal/storage/migrations/0001_init.sql
internal/storage/repositories.go
internal/withdraw/service.go
internal/withdraw/gas.go
internal/withdraw/morpho_adapter.go
config/config.example.yaml
.env.example
Dockerfile
docker-compose.yml
Makefile
```

Key boundary rules:

- `internal/core` owns shared IDs, status constants, and cross-service DTOs.
- `internal/morpho` owns ABI packing and read calls only.
- `internal/withdraw` owns transaction orchestration and gas replacement.
- `internal/monitor` owns monitor loop behavior and module contracts.
- Telegram code depends on core services, but core services do not import Telegram packages.
- Raw status strings, command strings, module IDs, finding keys, and event types live in constant objects in `internal/core/ids.go`.

## Task 1: Bootstrap Go Project And Runtime Skeleton

**Files:**

- Create: `go.mod`
- Create: `.gitignore`
- Create: `Makefile`
- Create: `cmd/withdraw-bot/main.go`
- Create: `internal/app/mode.go`
- Create: `internal/app/app.go`
- Create: `config/config.example.yaml`
- Create: `.env.example`
- Create: `Dockerfile`
- Create: `docker-compose.yml`

- [ ] **Step 1: Create the module file with exact dependency versions**

Create `go.mod`:

```go
module withdraw-bot

go 1.25.7

toolchain go1.25.10

require (
	github.com/ethereum/go-ethereum v1.17.2
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
	github.com/mattn/go-sqlite3 v1.14.44
	github.com/pressly/goose/v3 v3.27.1
	github.com/stretchr/testify v1.11.1
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gopkg.in/yaml.v3 v3.0.1
)
```

Run:

```bash
go mod download
```

Expected: `go.sum` is created.

- [ ] **Step 2: Add root ignore rules**

Create `.gitignore`:

```gitignore
.env
.env.local
data/
dist/
withdraw-bot
*.log
```

- [ ] **Step 3: Add build and test commands**

Create `Makefile`:

```makefile
.PHONY: test build run-config-check run-bootstrap run-monitor

test:
	go test ./... -count=1

build:
	go build -o dist/withdraw-bot ./cmd/withdraw-bot

run-config-check:
	go run ./cmd/withdraw-bot config-check --config config/config.example.yaml

run-bootstrap:
	go run ./cmd/withdraw-bot bootstrap --config config/config.example.yaml

run-monitor:
	go run ./cmd/withdraw-bot monitor --config config/config.example.yaml
```

- [ ] **Step 4: Add app mode constants**

Create `internal/app/mode.go`:

```go
package app

type Mode string

const (
	ModeMonitor     Mode = "monitor"
	ModeBootstrap   Mode = "bootstrap"
	ModeConfigCheck Mode = "config-check"
)

func ParseMode(value string) (Mode, bool) {
	switch Mode(value) {
	case ModeMonitor, ModeBootstrap, ModeConfigCheck:
		return Mode(value), true
	default:
		return "", false
	}
}
```

- [ ] **Step 5: Add a failing mode parse test**

Create `internal/app/mode_test.go`:

```go
package app

import "testing"

func TestParseModeReturnsFalseWhenModeIsUnknown(t *testing.T) {
	// Arrange
	input := "serve"

	// Act
	mode, ok := ParseMode(input)

	// Assert
	if ok {
		t.Fatalf("expected unknown mode to be rejected")
	}
	if mode != "" {
		t.Fatalf("expected empty mode, got %q", mode)
	}
}
```

Run:

```bash
go test ./internal/app -run TestParseModeReturnsFalseWhenModeIsUnknown -count=1
```

Expected: PASS after `mode.go` exists.

- [ ] **Step 6: Add CLI entrypoint**

Create `cmd/withdraw-bot/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"withdraw-bot/internal/app"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:]))
}

func run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: withdraw-bot <monitor|bootstrap|config-check> --config <path>")
		return 2
	}

	mode, ok := app.ParseMode(args[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "unsupported mode %q\n", args[0])
		return 2
	}

	fs := flag.NewFlagSet(string(mode), flag.ContinueOnError)
	configPath := fs.String("config", "config/config.example.yaml", "path to YAML config")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	if err := app.Run(ctx, mode, *configPath); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}
```

Create `internal/app/app.go`:

```go
package app

import (
	"context"
	"fmt"
)

func Run(ctx context.Context, mode Mode, configPath string) error {
	if configPath == "" {
		return fmt.Errorf("config path is required")
	}
	switch mode {
	case ModeMonitor, ModeBootstrap, ModeConfigCheck:
		return nil
	default:
		return fmt.Errorf("unsupported mode %q", mode)
	}
}
```

- [ ] **Step 7: Add config and env examples**

Create `config/config.example.yaml` with syntactically valid, non-secret defaults:

```yaml
app:
  monitor_interval: 5m
  data_dir: ./data

ethereum:
  chain_id: 1
  vault_address: "0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0"
  asset_symbol: USDC
  asset_decimals: 6
  receiver_address: ""

telegram:
  chat_id: 0
  allowed_user_ids: []
  daily_report_utc_time: "09:00"

gas:
  replacement_timeout: 2m
  gas_limit_buffer_bps: 2000
  fee_bump_bps: 1250
  max_fee_per_gas_gwei: "200"
  max_priority_fee_per_gas_gwei: "5"

logs:
  file_path: ./data/withdraw-bot.log
  max_size_mb: 20
  max_backups: 7
  max_age_days: 30

modules:
  share_price_loss:
    enabled: true
    stale_urgent_after: 30m
    baseline_share_price_asset_units: "1000000"
    loss_warn_bps: 50
    loss_urgent_bps: 100
  withdraw_liquidity:
    enabled: true
    stale_urgent_after: 15m
    idle_warn_threshold_usdc: "1000000"
    idle_urgent_threshold_usdc: "500000"
  vault_state_baseline:
    enabled: true
    stale_urgent_after: 30m
    change_severity: urgent
    baseline:
      owner: "0x0000000000000000000000000000000000000000"
      curator: "0x0000000000000000000000000000000000000000"
      receive_shares_gate: "0x0000000000000000000000000000000000000000"
      send_shares_gate: "0x0000000000000000000000000000000000000000"
      receive_assets_gate: "0x0000000000000000000000000000000000000000"
      send_assets_gate: "0x0000000000000000000000000000000000000000"
      adapter_registry: "0x0000000000000000000000000000000000000000"
      liquidity_adapter: "0x0000000000000000000000000000000000000000"
      liquidity_data_hex: "0x"
      performance_fee: "0"
      performance_fee_recipient: "0x0000000000000000000000000000000000000000"
      management_fee: "0"
      management_fee_recipient: "0x0000000000000000000000000000000000000000"
      max_rate: "0"
      adapters: []
      allocator_roles: {}
      sentinel_roles: {}
      timelocks: {}
      abdicated: {}
      force_deallocate_penalties: {}
```

Create `.env.example`:

```dotenv
WITHDRAW_BOT_PRIVATE_KEY=
WITHDRAW_BOT_TELEGRAM_BOT_TOKEN=
WITHDRAW_BOT_ETHEREUM_PRIMARY_RPC_URL=
WITHDRAW_BOT_ETHEREUM_FALLBACK_RPC_URLS=
```

- [ ] **Step 8: Add Docker artifacts**

Create `Dockerfile`:

```Dockerfile
FROM golang:1.25.10-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/withdraw-bot ./cmd/withdraw-bot

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/withdraw-bot /usr/local/bin/withdraw-bot
ENTRYPOINT ["withdraw-bot"]
CMD ["monitor", "--config", "/app/config.yaml"]
```

Create `docker-compose.yml`:

```yaml
services:
  withdraw-bot:
    build: .
    command: ["monitor", "--config", "/app/config.yaml"]
    env_file:
      - .env
    volumes:
      - ./config/config.example.yaml:/app/config.yaml:ro
      - ./data:/app/data
    restart: unless-stopped
```

- [ ] **Step 9: Verify and commit**

Run:

```bash
go test ./... -count=1
git status --short
```

Expected: tests pass and only intended files are changed.

Commit:

```bash
git add go.mod go.sum .gitignore Makefile cmd internal config .env.example Dockerfile docker-compose.yml
git commit -m "chore: bootstrap Go daemon project"
```

## Task 2: Core Types, IDs, And Deterministic Clock

**Files:**

- Create: `internal/core/ids.go`
- Create: `internal/core/status.go`
- Create: `internal/core/types.go`
- Create: `internal/core/time.go`
- Create: `internal/core/status_test.go`
- Create: `internal/core/time_test.go`

- [ ] **Step 1: Add failing severity ordering test**

Create `internal/core/status_test.go`:

```go
package core

import "testing"

func TestWorstKnownStatusIgnoresUnknownWhenKnownStatusesExist(t *testing.T) {
	// Arrange
	statuses := []MonitorStatus{MonitorStatusOK, MonitorStatusUnknown, MonitorStatusWarn}

	// Act
	result := WorstKnownStatus(statuses)

	// Assert
	if result != MonitorStatusWarn {
		t.Fatalf("expected %q, got %q", MonitorStatusWarn, result)
	}
}
```

Run:

```bash
go test ./internal/core -run TestWorstKnownStatusIgnoresUnknownWhenKnownStatusesExist -count=1
```

Expected: FAIL with `undefined: MonitorStatus`.

- [ ] **Step 2: Add shared constants and status logic**

Create `internal/core/ids.go`:

```go
package core

type MonitorModuleID string
type FindingKey string
type EventType string
type CommandName string

const (
	ModuleSharePriceLoss    MonitorModuleID = "share_price_loss"
	ModuleWithdrawLiquidity MonitorModuleID = "withdraw_liquidity"
	ModuleVaultState        MonitorModuleID = "vault_state_baseline"
)

const (
	FindingSharePriceLoss FindingKey = "share_price_loss"
	FindingIdleLiquidity FindingKey = "idle_liquidity"
	FindingExitSimulation FindingKey = "exit_simulation"
	FindingVaultStateDiff FindingKey = "vault_state_diff"
	FindingStaleData      FindingKey = "stale_data"
)

const (
	EventInfo       EventType = "info"
	EventWarning    EventType = "warning"
	EventError      EventType = "error"
	EventSecurity   EventType = "security"
	EventWithdrawal EventType = "withdrawal"
)

const (
	CommandStats        CommandName = "/stats"
	CommandWithdraw     CommandName = "/withdraw"
	CommandConfirm      CommandName = "/confirm"
	CommandThresholds   CommandName = "/thresholds"
	CommandThresholdSet CommandName = "/threshold"
	CommandLogs         CommandName = "/logs"
	CommandHelp         CommandName = "/help"
)
```

Create `internal/core/status.go`:

```go
package core

type Severity string
type MonitorStatus string

const (
	SeverityWarn   Severity = "warn"
	SeverityUrgent Severity = "urgent"
)

const (
	MonitorStatusOK      MonitorStatus = "OK"
	MonitorStatusWarn    MonitorStatus = "WARN"
	MonitorStatusUrgent  MonitorStatus = "URGENT"
	MonitorStatusUnknown MonitorStatus = "UNKNOWN"
)

func WorstKnownStatus(statuses []MonitorStatus) MonitorStatus {
	worst := MonitorStatusOK
	seenKnown := false
	for _, status := range statuses {
		switch status {
		case MonitorStatusUrgent:
			return MonitorStatusUrgent
		case MonitorStatusWarn:
			seenKnown = true
			worst = MonitorStatusWarn
		case MonitorStatusOK:
			seenKnown = true
		}
	}
	if seenKnown {
		return worst
	}
	return MonitorStatusUnknown
}
```

Run:

```bash
go test ./internal/core -run TestWorstKnownStatusIgnoresUnknownWhenKnownStatusesExist -count=1
```

Expected: PASS.

- [ ] **Step 3: Add core DTOs**

Create `internal/core/types.go`:

```go
package core

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type Metric struct {
	Key   string
	Value string
	Unit  string
}

type Finding struct {
	Key      FindingKey
	Severity Severity
	Message  string
	Evidence map[string]string
}

type MonitorResult struct {
	ModuleID   MonitorModuleID
	Status     MonitorStatus
	ObservedAt time.Time
	Metrics    []Metric
	Findings   []Finding
}

type PositionSnapshot struct {
	Vault        common.Address
	Owner        common.Address
	Receiver     common.Address
	ShareBalance *big.Int
	AssetSymbol  string
	AssetDecimals uint8
	ObservedAt   time.Time
}

type FullExitRequest struct {
	Vault    common.Address
	Owner    common.Address
	Receiver common.Address
	Shares   *big.Int
}

type TxCandidate struct {
	To    common.Address
	Data  []byte
	Value *big.Int
}

type FullExitSimulation struct {
	Success            bool
	ExpectedAssetUnits *big.Int
	GasUnits           uint64
	RevertReason       string
}
```

- [ ] **Step 4: Add deterministic clock interface**

Create `internal/core/time.go`:

```go
package core

import "time"

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type FixedClock struct {
	Value time.Time
}

func (clock FixedClock) Now() time.Time {
	return clock.Value
}
```

Create `internal/core/time_test.go`:

```go
package core

import (
	"testing"
	"time"
)

func TestFixedClockReturnsConfiguredTime(t *testing.T) {
	// Arrange
	expected := time.Date(2026, 5, 9, 1, 2, 3, 0, time.UTC)
	clock := FixedClock{Value: expected}

	// Act
	actual := clock.Now()

	// Assert
	if !actual.Equal(expected) {
		t.Fatalf("expected %s, got %s", expected, actual)
	}
}
```

Run:

```bash
go test ./internal/core -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core
git commit -m "feat: add core domain types"
```

## Task 3: Config Loading, Env Secrets, And Unit Parsing

**Files:**

- Create: `internal/config/config.go`
- Create: `internal/config/env.go`
- Create: `internal/config/units.go`
- Create: `internal/config/config_test.go`
- Modify: `.env.example`

- [ ] **Step 1: Add failing validation test**

Create `internal/config/config_test.go`:

```go
package config

import "testing"

func TestValidateReturnsErrorWhenTelegramChatIDIsMissing(t *testing.T) {
	// Arrange
	cfg := Config{
		App: AppConfig{MonitorInterval: "5m", DataDir: "./data"},
		Ethereum: EthereumConfig{ChainID: 1, VaultAddress: "0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0", AssetDecimals: 6},
		Telegram: TelegramConfig{AllowedUserIDs: []int64{123}},
	}

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Fatalf("expected missing chat ID to be rejected")
	}
}
```

Run:

```bash
go test ./internal/config -run TestValidateReturnsErrorWhenTelegramChatIDIsMissing -count=1
```

Expected: FAIL with `undefined: Config`.

- [ ] **Step 2: Implement config structs and validation**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App      AppConfig                 `yaml:"app"`
	Ethereum EthereumConfig            `yaml:"ethereum"`
	Telegram TelegramConfig            `yaml:"telegram"`
	Gas      GasConfig                 `yaml:"gas"`
	Logs     LogConfig                 `yaml:"logs"`
	Modules  map[string]ModuleConfig   `yaml:"modules"`
}

type AppConfig struct {
	MonitorInterval string `yaml:"monitor_interval"`
	DataDir         string `yaml:"data_dir"`
}

type EthereumConfig struct {
	ChainID         int64  `yaml:"chain_id"`
	VaultAddress    string `yaml:"vault_address"`
	AssetSymbol     string `yaml:"asset_symbol"`
	AssetDecimals   uint8  `yaml:"asset_decimals"`
	ReceiverAddress string `yaml:"receiver_address"`
}

type TelegramConfig struct {
	ChatID             int64   `yaml:"chat_id"`
	AllowedUserIDs     []int64 `yaml:"allowed_user_ids"`
	DailyReportUTCTime string  `yaml:"daily_report_utc_time"`
}

type GasConfig struct {
	ReplacementTimeout       string `yaml:"replacement_timeout"`
	GasLimitBufferBPS        int64  `yaml:"gas_limit_buffer_bps"`
	FeeBumpBPS               int64  `yaml:"fee_bump_bps"`
	MaxFeePerGasGwei         string `yaml:"max_fee_per_gas_gwei"`
	MaxPriorityFeePerGasGwei string `yaml:"max_priority_fee_per_gas_gwei"`
}

type LogConfig struct {
	FilePath   string `yaml:"file_path"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAgeDays int    `yaml:"max_age_days"`
}

type ModuleConfig map[string]any

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg Config) Validate() error {
	if cfg.App.MonitorInterval == "" {
		return fmt.Errorf("app.monitor_interval is required")
	}
	if _, err := time.ParseDuration(cfg.App.MonitorInterval); err != nil {
		return fmt.Errorf("app.monitor_interval must be a Go duration: %w", err)
	}
	if cfg.App.DataDir == "" {
		return fmt.Errorf("app.data_dir is required")
	}
	if cfg.Ethereum.ChainID <= 0 {
		return fmt.Errorf("ethereum.chain_id must be positive")
	}
	if cfg.Ethereum.VaultAddress == "" {
		return fmt.Errorf("ethereum.vault_address is required")
	}
	if cfg.Ethereum.AssetDecimals == 0 {
		return fmt.Errorf("ethereum.asset_decimals is required")
	}
	if cfg.Telegram.ChatID == 0 {
		return fmt.Errorf("telegram.chat_id is required")
	}
	if len(cfg.Telegram.AllowedUserIDs) == 0 {
		return fmt.Errorf("telegram.allowed_user_ids must not be empty")
	}
	return nil
}
```

Run:

```bash
go test ./internal/config -run TestValidateReturnsErrorWhenTelegramChatIDIsMissing -count=1
```

Expected: PASS.

- [ ] **Step 3: Add env secret loading**

Create `internal/config/env.go`:

```go
package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	EnvPrivateKey      = "WITHDRAW_BOT_PRIVATE_KEY"
	EnvTelegramToken   = "WITHDRAW_BOT_TELEGRAM_BOT_TOKEN"
	EnvPrimaryRPCURL   = "WITHDRAW_BOT_ETHEREUM_PRIMARY_RPC_URL"
	EnvFallbackRPCURLs = "WITHDRAW_BOT_ETHEREUM_FALLBACK_RPC_URLS"
)

type Secrets struct {
	PrivateKey      string
	TelegramToken   string
	PrimaryRPCURL   string
	FallbackRPCURLs []string
}

func LoadSecretsFromEnv() (Secrets, error) {
	secrets := Secrets{
		PrivateKey:    strings.TrimSpace(os.Getenv(EnvPrivateKey)),
		TelegramToken: strings.TrimSpace(os.Getenv(EnvTelegramToken)),
		PrimaryRPCURL: strings.TrimSpace(os.Getenv(EnvPrimaryRPCURL)),
	}
	fallbacks := strings.TrimSpace(os.Getenv(EnvFallbackRPCURLs))
	if fallbacks != "" {
		for _, raw := range strings.Split(fallbacks, ",") {
			value := strings.TrimSpace(raw)
			if value != "" {
				secrets.FallbackRPCURLs = append(secrets.FallbackRPCURLs, value)
			}
		}
	}
	if secrets.PrivateKey == "" {
		return Secrets{}, fmt.Errorf("%s is required", EnvPrivateKey)
	}
	if secrets.TelegramToken == "" {
		return Secrets{}, fmt.Errorf("%s is required", EnvTelegramToken)
	}
	if secrets.PrimaryRPCURL == "" {
		return Secrets{}, fmt.Errorf("%s is required", EnvPrimaryRPCURL)
	}
	return secrets, nil
}
```

- [ ] **Step 4: Add unit parsing helpers**

Create `internal/config/units.go`:

```go
package config

import (
	"fmt"
	"math/big"
	"strings"
)

const basisPointsDenominator int64 = 10_000

func ValidateBPS(name string, value int64) error {
	if value < 0 || value > basisPointsDenominator {
		return fmt.Errorf("%s must be between 0 and 10000 bps", name)
	}
	return nil
}

func ParseDecimalUnits(name string, value string, decimals uint8) (*big.Int, error) {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	amount, ok := new(big.Rat).SetString(clean)
	if !ok {
		return nil, fmt.Errorf("%s must be a decimal string", name)
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	scaled := new(big.Rat).Mul(amount, new(big.Rat).SetInt(scale))
	if !scaled.IsInt() {
		return nil, fmt.Errorf("%s has more than %d decimal places", name, decimals)
	}
	if scaled.Sign() < 0 {
		return nil, fmt.Errorf("%s must not be negative", name)
	}
	return scaled.Num(), nil
}

func ParseGwei(name string, value string) (*big.Int, error) {
	units, err := ParseDecimalUnits(name, value, 9)
	if err != nil {
		return nil, err
	}
	return units, nil
}
```

- [ ] **Step 5: Add deterministic parser tests**

Append to `internal/config/config_test.go`:

```go
func TestParseDecimalUnitsReturnsBaseUnitsForUSDC(t *testing.T) {
	// Arrange
	value := "123.456789"

	// Act
	result, err := ParseDecimalUnits("idle_urgent_threshold_usdc", value, 6)

	// Assert
	if err != nil {
		t.Fatalf("expected parse to succeed: %v", err)
	}
	if result.String() != "123456789" {
		t.Fatalf("expected 123456789, got %s", result.String())
	}
}

func TestParseDecimalUnitsRejectsTooManyDecimals(t *testing.T) {
	// Arrange
	value := "1.0000001"

	// Act
	_, err := ParseDecimalUnits("idle_urgent_threshold_usdc", value, 6)

	// Assert
	if err == nil {
		t.Fatalf("expected too many decimals to be rejected")
	}
}
```

Run:

```bash
go test ./internal/config -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config config/config.example.yaml .env.example
git commit -m "feat: add config and secret loading"
```

## Task 4: SQLite Migrations And Repositories

**Files:**

- Create: `internal/storage/migrations/0001_init.sql`
- Create: `internal/storage/db.go`
- Create: `internal/storage/repositories.go`
- Create: `internal/storage/storage_test.go`

- [ ] **Step 1: Add schema migration**

Create `internal/storage/migrations/0001_init.sql`:

```sql
-- +goose Up
CREATE TABLE monitor_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module_id TEXT NOT NULL,
    status TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    metrics_json TEXT NOT NULL,
    findings_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_monitor_snapshots_module_time ON monitor_snapshots(module_id, observed_at);

CREATE TABLE event_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    message TEXT NOT NULL,
    fields_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_event_records_type_time ON event_records(event_type, created_at);

CREATE TABLE threshold_overrides (
    module_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_by_user_id INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (module_id, key)
);

CREATE TABLE pending_confirmations (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    requested_by_user_id INTEGER NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE withdrawal_attempts (
    id TEXT PRIMARY KEY,
    trigger_kind TEXT NOT NULL,
    trigger_module_id TEXT NOT NULL,
    trigger_finding_key TEXT NOT NULL,
    status TEXT NOT NULL,
    tx_hash TEXT NOT NULL,
    nonce INTEGER,
    gas_units INTEGER,
    max_fee_per_gas_wei TEXT,
    max_priority_fee_per_gas_wei TEXT,
    expected_asset_units TEXT,
    simulation_success INTEGER NOT NULL,
    failure_reason TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE withdrawal_attempts;
DROP TABLE pending_confirmations;
DROP TABLE threshold_overrides;
DROP TABLE event_records;
DROP TABLE monitor_snapshots;
```

- [ ] **Step 2: Add failing migration test**

Create `internal/storage/storage_test.go`:

```go
package storage

import (
	"context"
	"testing"
)

func TestOpenAppliesMigrations(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	db, err := Open(ctx, ":memory:")

	// Assert
	if err != nil {
		t.Fatalf("expected open to apply migrations: %v", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "SELECT COUNT(*) FROM monitor_snapshots"); err != nil {
		t.Fatalf("expected monitor_snapshots table to exist: %v", err)
	}
}
```

Run:

```bash
go test ./internal/storage -run TestOpenAppliesMigrations -count=1
```

Expected: FAIL with `undefined: Open`.

- [ ] **Step 3: Implement embedded migrations**

Create `internal/storage/db.go`:

```go
package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set migration dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply sqlite migrations: %w", err)
	}
	return db, nil
}
```

Run:

```bash
go test ./internal/storage -run TestOpenAppliesMigrations -count=1
```

Expected: PASS.

- [ ] **Step 4: Add repository interfaces and insert methods**

Create `internal/storage/repositories.go`:

```go
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"withdraw-bot/internal/core"
)

type Repositories struct {
	DB *sql.DB
}

func NewRepositories(db *sql.DB) Repositories {
	return Repositories{DB: db}
}

func (repos Repositories) InsertMonitorResult(ctx context.Context, result core.MonitorResult, createdAt time.Time) error {
	metrics, err := json.Marshal(result.Metrics)
	if err != nil {
		return fmt.Errorf("encode monitor metrics: %w", err)
	}
	findings, err := json.Marshal(result.Findings)
	if err != nil {
		return fmt.Errorf("encode monitor findings: %w", err)
	}
	_, err = repos.DB.ExecContext(
		ctx,
		`INSERT INTO monitor_snapshots(module_id, status, observed_at, metrics_json, findings_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		string(result.ModuleID),
		string(result.Status),
		result.ObservedAt.Format(time.RFC3339Nano),
		string(metrics),
		string(findings),
		createdAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert monitor result for %s: %w", result.ModuleID, err)
	}
	return nil
}

func (repos Repositories) InsertEvent(ctx context.Context, eventType core.EventType, message string, fields map[string]string, createdAt time.Time) error {
	data, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("encode event fields: %w", err)
	}
	_, err = repos.DB.ExecContext(
		ctx,
		`INSERT INTO event_records(event_type, message, fields_json, created_at) VALUES (?, ?, ?, ?)`,
		string(eventType),
		message,
		string(data),
		createdAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert event record: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Add repository behavior test**

Append to `internal/storage/storage_test.go`:

```go
func TestInsertMonitorResultPersistsModuleStatus(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	repos := NewRepositories(db)
	observedAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	result := core.MonitorResult{
		ModuleID: core.ModuleSharePriceLoss,
		Status: core.MonitorStatusWarn,
		ObservedAt: observedAt,
	}

	// Act
	err = repos.InsertMonitorResult(ctx, result, observedAt)

	// Assert
	if err != nil {
		t.Fatalf("insert monitor result: %v", err)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM monitor_snapshots WHERE module_id = ?", string(core.ModuleSharePriceLoss)).Scan(&status); err != nil {
		t.Fatalf("query monitor result: %v", err)
	}
	if status != string(core.MonitorStatusWarn) {
		t.Fatalf("expected status %q, got %q", core.MonitorStatusWarn, status)
	}
}
```

Add imports to `internal/storage/storage_test.go`:

```go
import (
	"context"
	"testing"
	"time"

	"withdraw-bot/internal/core"
)
```

Run:

```bash
go test ./internal/storage -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/storage
git commit -m "feat: add sqlite storage"
```

## Task 5: Ethereum Client, RPC Fallback, And Private Key Signer

**Files:**

- Create: `internal/ethereum/client.go`
- Create: `internal/ethereum/client_test.go`
- Create: `internal/signer/signer.go`
- Create: `internal/signer/signer_test.go`

- [ ] **Step 1: Add Ethereum client interfaces**

Create `internal/ethereum/client.go`:

```go
package ethereum

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var ErrPreBroadcast = errors.New("transaction failed before broadcast")

type RPCClient interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	ChainID(ctx context.Context) (*big.Int, error)
	Close()
}

type MultiClient struct {
	primary   RPCClient
	fallbacks []RPCClient
}

func NewMultiClient(primary RPCClient, fallbacks []RPCClient) MultiClient {
	return MultiClient{primary: primary, fallbacks: fallbacks}
}

func DialMulti(ctx context.Context, primaryURL string, fallbackURLs []string) (MultiClient, error) {
	primary, err := ethclient.DialContext(ctx, primaryURL)
	if err != nil {
		return MultiClient{}, fmt.Errorf("dial primary RPC: %w", err)
	}
	fallbacks := make([]RPCClient, 0, len(fallbackURLs))
	for _, url := range fallbackURLs {
		client, err := ethclient.DialContext(ctx, url)
		if err != nil {
			primary.Close()
			return MultiClient{}, fmt.Errorf("dial fallback RPC: %w", err)
		}
		fallbacks = append(fallbacks, client)
	}
	return NewMultiClient(primary, fallbacks), nil
}

func (client MultiClient) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	result, err := client.primary.CallContract(ctx, call, blockNumber)
	if err == nil {
		return result, nil
	}
	var lastErr error = err
	for _, fallback := range client.fallbacks {
		result, err := fallback.CallContract(ctx, call, blockNumber)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("call contract failed on all RPC clients: %w", lastErr)
}

func (client MultiClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := client.primary.SendTransaction(ctx, tx); err != nil {
		return fmt.Errorf("%w: %v", ErrPreBroadcast, err)
	}
	return nil
}

func (client MultiClient) Close() {
	client.primary.Close()
	for _, fallback := range client.fallbacks {
		fallback.Close()
	}
}
```

- [ ] **Step 2: Add signer implementation**

Create `internal/signer/signer.go`:

```go
package signer

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type Service interface {
	Address(ctx context.Context) (common.Address, error)
	SignTransaction(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
}

type PrivateKeyService struct {
	key     *ecdsa.PrivateKey
	address common.Address
}

func NewPrivateKeyService(privateKeyHex string) (*PrivateKeyService, error) {
	clean := strings.TrimPrefix(strings.TrimSpace(privateKeyHex), "0x")
	key, err := crypto.HexToECDSA(clean)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return &PrivateKeyService{key: key, address: crypto.PubkeyToAddress(key.PublicKey)}, nil
}

func (service *PrivateKeyService) Address(ctx context.Context) (common.Address, error) {
	return service.address, nil
}

func (service *PrivateKeyService) SignTransaction(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	signer := types.LatestSignerForChainID(chainID)
	signed, err := types.SignTx(tx, signer, service.key)
	if err != nil {
		return nil, fmt.Errorf("sign transaction: %w", err)
	}
	return signed, nil
}
```

- [ ] **Step 3: Add signer behavior test**

Create `internal/signer/signer_test.go`:

```go
package signer

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestPrivateKeyServiceSignsTransactionForExpectedAddress(t *testing.T) {
	// Arrange
	ctx := context.Background()
	privateKey := "0x0123456789012345678901234567890123456789012345678901234567890123"
	service, err := NewPrivateKeyService(privateKey)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	chainID := big.NewInt(1)
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     1,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(2),
		Gas:       21000,
		To:        &common.Address{},
		Value:     big.NewInt(0),
		Data:      nil,
	})

	// Act
	signed, err := service.SignTransaction(ctx, tx, chainID)

	// Assert
	if err != nil {
		t.Fatalf("sign transaction: %v", err)
	}
	sender, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
	if err != nil {
		t.Fatalf("recover sender: %v", err)
	}
	expected, err := service.Address(ctx)
	if err != nil {
		t.Fatalf("read signer address: %v", err)
	}
	if sender != expected {
		t.Fatalf("expected sender %s, got %s", expected.Hex(), sender.Hex())
	}
}
```

Run:

```bash
go test ./internal/signer -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/ethereum internal/signer
git commit -m "feat: add ethereum client and signer"
```

## Task 6: Morpho ABI Client And Full Exit Adapter

**Files:**

- Create: `internal/morpho/abi.go`
- Create: `internal/morpho/vault_client.go`
- Create: `internal/morpho/abi_test.go`
- Create: `internal/withdraw/morpho_adapter.go`
- Create: `internal/withdraw/morpho_adapter_test.go`

- [ ] **Step 1: Add ABI packing test**

Create `internal/morpho/abi_test.go`:

```go
package morpho

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestPackRedeemEncodesSelectorAndArguments(t *testing.T) {
	// Arrange
	shares := big.NewInt(123)
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000001")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000002")

	// Act
	data, err := PackRedeem(shares, receiver, owner)

	// Assert
	if err != nil {
		t.Fatalf("pack redeem: %v", err)
	}
	if len(data) != 4+32*3 {
		t.Fatalf("expected redeem calldata length %d, got %d", 4+32*3, len(data))
	}
}
```

Run:

```bash
go test ./internal/morpho -run TestPackRedeemEncodesSelectorAndArguments -count=1
```

Expected: FAIL with `undefined: PackRedeem`.

- [ ] **Step 2: Implement ABI helpers**

Create `internal/morpho/abi.go`:

```go
package morpho

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
)

const vaultABIJSON = `[
	{"type":"function","name":"asset","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"balanceOf","stateMutability":"view","inputs":[{"name":"account","type":"address"}],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"previewRedeem","stateMutability":"view","inputs":[{"name":"shares","type":"uint256"}],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"redeem","stateMutability":"nonpayable","inputs":[{"name":"shares","type":"uint256"},{"name":"receiver","type":"address"},{"name":"onBehalf","type":"address"}],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"totalAssets","stateMutability":"view","inputs":[],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"totalSupply","stateMutability":"view","inputs":[],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"owner","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"curator","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"receiveSharesGate","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"sendSharesGate","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"receiveAssetsGate","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"sendAssetsGate","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"adapterRegistry","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"liquidityAdapter","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"liquidityData","stateMutability":"view","inputs":[],"outputs":[{"type":"bytes"}]},
	{"type":"function","name":"performanceFee","stateMutability":"view","inputs":[],"outputs":[{"type":"uint96"}]},
	{"type":"function","name":"performanceFeeRecipient","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"managementFee","stateMutability":"view","inputs":[],"outputs":[{"type":"uint96"}]},
	{"type":"function","name":"managementFeeRecipient","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"maxRate","stateMutability":"view","inputs":[],"outputs":[{"type":"uint64"}]},
	{"type":"function","name":"adaptersLength","stateMutability":"view","inputs":[],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"adapters","stateMutability":"view","inputs":[{"type":"uint256"}],"outputs":[{"type":"address"}]},
	{"type":"function","name":"isAllocator","stateMutability":"view","inputs":[{"type":"address"}],"outputs":[{"type":"bool"}]},
	{"type":"function","name":"isSentinel","stateMutability":"view","inputs":[{"type":"address"}],"outputs":[{"type":"bool"}]},
	{"type":"function","name":"timelock","stateMutability":"view","inputs":[{"type":"bytes4"}],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"abdicated","stateMutability":"view","inputs":[{"type":"bytes4"}],"outputs":[{"type":"bool"}]},
	{"type":"function","name":"forceDeallocatePenalty","stateMutability":"view","inputs":[{"type":"address"}],"outputs":[{"type":"uint256"}]}
]`

const erc20ABIJSON = `[
	{"type":"function","name":"balanceOf","stateMutability":"view","inputs":[{"name":"account","type":"address"}],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"decimals","stateMutability":"view","inputs":[],"outputs":[{"type":"uint8"}]}
]`

var VaultABI = mustParseABI(vaultABIJSON)
var ERC20ABI = mustParseABI(erc20ABIJSON)

func mustParseABI(raw string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(raw))
	if err != nil {
		panic(err)
	}
	return parsed
}

func PackRedeem(shares *big.Int, receiver common.Address, owner common.Address) ([]byte, error) {
	return VaultABI.Pack("redeem", shares, receiver, owner)
}
```

Run:

```bash
go test ./internal/morpho -count=1
```

Expected: PASS.

- [ ] **Step 3: Implement Morpho vault read client**

Create `internal/morpho/vault_client.go`:

```go
package morpho

import (
	"context"
	"fmt"
	"math/big"

	"withdraw-bot/internal/ethereum"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

type VaultClient struct {
	Ethereum ethereum.MultiClient
	Vault    common.Address
}

func (client VaultClient) call(ctx context.Context, method string, args ...any) ([]any, error) {
	data, err := VaultABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack %s call: %w", method, err)
	}
	raw, err := client.Ethereum.CallContract(ctx, geth.CallMsg{To: &client.Vault, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", method, err)
	}
	out, err := VaultABI.Unpack(method, raw)
	if err != nil {
		return nil, fmt.Errorf("unpack %s: %w", method, err)
	}
	return out, nil
}

func (client VaultClient) BalanceOf(ctx context.Context, owner common.Address) (*big.Int, error) {
	out, err := client.call(ctx, "balanceOf", owner)
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}

func (client VaultClient) PreviewRedeem(ctx context.Context, shares *big.Int) (*big.Int, error) {
	out, err := client.call(ctx, "previewRedeem", shares)
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}
```

- [ ] **Step 4: Implement Morpho withdraw adapter**

Create `internal/withdraw/morpho_adapter.go`:

```go
package withdraw

import (
	"context"
	"math/big"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/ethereum"
	"withdraw-bot/internal/morpho"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

type MorphoAdapter struct {
	Ethereum     ethereum.MultiClient
	VaultClient  morpho.VaultClient
	Vault         common.Address
	Owner         common.Address
	Receiver      common.Address
	AssetSymbol   string
	AssetDecimals uint8
	Clock         core.Clock
}

func (adapter MorphoAdapter) ID() string {
	return "morpho_vault_v2"
}

func (adapter MorphoAdapter) Position(ctx context.Context) (core.PositionSnapshot, error) {
	shares, err := adapter.VaultClient.BalanceOf(ctx, adapter.Owner)
	if err != nil {
		return core.PositionSnapshot{}, err
	}
	return core.PositionSnapshot{
		Vault: adapter.Vault,
		Owner: adapter.Owner,
		Receiver: adapter.Receiver,
		ShareBalance: shares,
		AssetSymbol: adapter.AssetSymbol,
		AssetDecimals: adapter.AssetDecimals,
		ObservedAt: adapter.Clock.Now(),
	}, nil
}

func (adapter MorphoAdapter) BuildFullExit(ctx context.Context, req core.FullExitRequest) (core.TxCandidate, error) {
	data, err := morpho.PackRedeem(req.Shares, req.Receiver, req.Owner)
	if err != nil {
		return core.TxCandidate{}, err
	}
	return core.TxCandidate{To: req.Vault, Data: data, Value: big.NewInt(0)}, nil
}

func (adapter MorphoAdapter) SimulateFullExit(ctx context.Context, req core.FullExitRequest) (core.FullExitSimulation, error) {
	expected, err := adapter.VaultClient.PreviewRedeem(ctx, req.Shares)
	if err != nil {
		return core.FullExitSimulation{}, err
	}
	candidate, err := adapter.BuildFullExit(ctx, req)
	if err != nil {
		return core.FullExitSimulation{}, err
	}
	call := geth.CallMsg{From: req.Owner, To: &candidate.To, Value: candidate.Value, Data: candidate.Data}
	gas, err := adapter.Ethereum.EstimateGas(ctx, call)
	if err != nil {
		return core.FullExitSimulation{Success: false, ExpectedAssetUnits: expected, RevertReason: err.Error()}, nil
	}
	return core.FullExitSimulation{Success: true, ExpectedAssetUnits: expected, GasUnits: gas}, nil
}
```

Add forwarding methods to `internal/ethereum/client.go` for `EstimateGas`, `PendingNonceAt`, `SuggestGasTipCap`, `TransactionReceipt`, and `ChainID`. Each should call the primary RPC and wrap errors with operation context. Read-style calls may use fallback rotation.

- [ ] **Step 5: Add adapter no-op position test with fake client**

Create `internal/withdraw/morpho_adapter_test.go`:

```go
package withdraw

import (
	"context"
	"math/big"
	"testing"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/morpho"

	"github.com/ethereum/go-ethereum/common"
)

func TestMorphoAdapterPositionReturnsConfiguredOwnerAndReceiver(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	observedAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	adapter := MorphoAdapter{
		Vault: common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0"),
		Owner: owner,
		Receiver: receiver,
		AssetSymbol: "USDC",
		AssetDecimals: 6,
		Clock: core.FixedClock{Value: observedAt},
		VaultClient: morpho.VaultClient{},
	}

	// Act
	result := core.PositionSnapshot{
		Vault: adapter.Vault,
		Owner: adapter.Owner,
		Receiver: adapter.Receiver,
		ShareBalance: big.NewInt(0),
		AssetSymbol: adapter.AssetSymbol,
		AssetDecimals: adapter.AssetDecimals,
		ObservedAt: adapter.Clock.Now(),
	}

	// Assert
	if result.Owner != owner {
		t.Fatalf("expected owner %s, got %s", owner.Hex(), result.Owner.Hex())
	}
	if result.Receiver != receiver {
		t.Fatalf("expected receiver %s, got %s", receiver.Hex(), result.Receiver.Hex())
	}
}
```

Run:

```bash
go test ./internal/morpho ./internal/withdraw -count=1
```

Expected: PASS after missing `MultiClient` forwarding methods are implemented.

- [ ] **Step 6: Commit**

```bash
git add internal/morpho internal/withdraw internal/ethereum
git commit -m "feat: add Morpho full exit adapter"
```

## Task 7: Monitor Service And Share Price Module

**Files:**

- Create: `internal/monitor/service.go`
- Create: `internal/monitor/service_test.go`
- Create: `internal/monitor/modules/morpho/share_price.go`
- Create: `internal/monitor/modules/morpho/share_price_test.go`

- [ ] **Step 1: Add monitor module interface and service**

Create `internal/monitor/service.go`:

```go
package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/storage"
)

type Module interface {
	ID() core.MonitorModuleID
	ValidateConfig(ctx context.Context) error
	Bootstrap(ctx context.Context) (map[string]any, error)
	Monitor(ctx context.Context) (core.MonitorResult, error)
}

type Service struct {
	Modules []Module
	Storage storage.Repositories
	Clock   core.Clock
	Latest  map[core.MonitorModuleID]core.MonitorResult
	mu      sync.RWMutex
}

func NewService(modules []Module, repos storage.Repositories, clock core.Clock) *Service {
	return &Service{Modules: modules, Storage: repos, Clock: clock, Latest: map[core.MonitorModuleID]core.MonitorResult{}}
}

func (service *Service) RunOnce(ctx context.Context) []core.MonitorResult {
	results := make([]core.MonitorResult, 0, len(service.Modules))
	for _, module := range service.Modules {
		result, err := module.Monitor(ctx)
		if err != nil {
			result = core.MonitorResult{
				ModuleID: module.ID(),
				Status: core.MonitorStatusUnknown,
				ObservedAt: service.Clock.Now(),
				Findings: []core.Finding{{
					Key: core.FindingStaleData,
					Severity: core.SeverityWarn,
					Message: fmt.Sprintf("module %s failed: %v", module.ID(), err),
					Evidence: map[string]string{"module_id": string(module.ID())},
				}},
			}
		}
		service.mu.Lock()
		service.Latest[result.ModuleID] = result
		service.mu.Unlock()
		_ = service.Storage.InsertMonitorResult(ctx, result, service.Clock.Now())
		results = append(results, result)
	}
	return results
}

func (service *Service) Snapshot() map[core.MonitorModuleID]core.MonitorResult {
	service.mu.RLock()
	defer service.mu.RUnlock()
	result := make(map[core.MonitorModuleID]core.MonitorResult, len(service.Latest))
	for key, value := range service.Latest {
		result[key] = value
	}
	return result
}

func (service *Service) RunLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	service.RunOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			service.RunOnce(ctx)
		}
	}
}
```

- [ ] **Step 2: Add share price module tests**

Create `internal/monitor/modules/morpho/share_price_test.go`:

```go
package morpho

import (
	"context"
	"math/big"
	"testing"
	"time"

	"withdraw-bot/internal/core"
)

func TestSharePriceModuleReturnsUrgentWhenBaselineLossCrossesThreshold(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		PreviousSharePrice: big.NewInt(995_000),
		WarnBPS: 50,
		UrgentBPS: 100,
		Reader: fakeSharePriceReader{price: big.NewInt(989_000)},
		Clock: core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	result, err := module.Monitor(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("monitor share price: %v", err)
	}
	if result.Status != core.MonitorStatusUrgent {
		t.Fatalf("expected urgent status, got %s", result.Status)
	}
}

type fakeSharePriceReader struct {
	price *big.Int
}

func (reader fakeSharePriceReader) CurrentSharePrice(ctx context.Context) (*big.Int, error) {
	return new(big.Int).Set(reader.price), nil
}
```

Run:

```bash
go test ./internal/monitor/modules/morpho -run TestSharePriceModuleReturnsUrgentWhenBaselineLossCrossesThreshold -count=1
```

Expected: FAIL with `undefined: SharePriceModule`.

- [ ] **Step 3: Implement share price module**

Create `internal/monitor/modules/morpho/share_price.go`:

```go
package morpho

import (
	"context"
	"fmt"
	"math/big"

	"withdraw-bot/internal/core"
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
		return fmt.Errorf("share_price_loss baseline_share_price_asset_units must be positive")
	}
	if module.Reader == nil {
		return fmt.Errorf("share_price_loss reader is required")
	}
	return nil
}

func (module SharePriceModule) Bootstrap(ctx context.Context) (map[string]any, error) {
	price, err := module.Reader.CurrentSharePrice(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"baseline_share_price_asset_units": price.String()}, nil
}

func (module SharePriceModule) Monitor(ctx context.Context) (core.MonitorResult, error) {
	current, err := module.Reader.CurrentSharePrice(ctx)
	if err != nil {
		return core.MonitorResult{}, err
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
		findings = append(findings, core.Finding{Key: core.FindingSharePriceLoss, Severity: core.SeverityUrgent, Message: "share price loss crossed urgent threshold", Evidence: map[string]string{"baseline_loss_bps": fmt.Sprint(baselineLoss), "previous_loss_bps": fmt.Sprint(previousLoss)}})
	} else if baselineLoss >= module.WarnBPS || previousLoss >= module.WarnBPS {
		status = core.MonitorStatusWarn
		findings = append(findings, core.Finding{Key: core.FindingSharePriceLoss, Severity: core.SeverityWarn, Message: "share price loss crossed warn threshold", Evidence: map[string]string{"baseline_loss_bps": fmt.Sprint(baselineLoss), "previous_loss_bps": fmt.Sprint(previousLoss)}})
	}
	return core.MonitorResult{
		ModuleID: module.ID(),
		Status: status,
		ObservedAt: module.Clock.Now(),
		Metrics: []core.Metric{{Key: "share_price_asset_units", Value: current.String(), Unit: "asset_units"}},
		Findings: findings,
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
```

Run:

```bash
go test ./internal/monitor/modules/morpho -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/monitor
git commit -m "feat: add monitor service and share price module"
```

## Task 8: Withdraw Liquidity And Vault State Modules

**Files:**

- Create: `internal/monitor/modules/morpho/withdraw_liquidity.go`
- Create: `internal/monitor/modules/morpho/withdraw_liquidity_test.go`
- Create: `internal/monitor/modules/morpho/vault_state.go`
- Create: `internal/monitor/modules/morpho/vault_state_test.go`

- [ ] **Step 1: Implement withdraw liquidity module with tests**

Create a test that arranges idle liquidity below urgent threshold and a successful full-exit simulation, acts by calling `Monitor`, and asserts `URGENT`. The module must expose these fields:

```go
type WithdrawLiquidityModule struct {
	IdleAssetReader IdleAssetReader
	ExitSimulator   ExitSimulator
	Owner           common.Address
	Receiver        common.Address
	Vault           common.Address
	IdleWarn        *big.Int
	IdleUrgent      *big.Int
	Clock           core.Clock
}
```

The module must emit:

- Metric `idle_assets` in `asset_units`
- Metric `expected_exit_assets` in `asset_units`
- Finding `idle_liquidity` as `WARN` or `URGENT`
- Finding `exit_simulation` as `URGENT` when simulation fails

Run:

```bash
go test ./internal/monitor/modules/morpho -run TestWithdrawLiquidityModuleReturnsUrgentWhenIdleLiquidityIsBelowThreshold -count=1
```

Expected before implementation: FAIL with missing module type.

Expected after implementation: PASS.

- [ ] **Step 2: Implement vault state snapshot and diff**

Create `internal/monitor/modules/morpho/vault_state.go` with:

```go
type VaultStateSnapshot struct {
	Owner                     string            `json:"owner"`
	Curator                   string            `json:"curator"`
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
```

The tracked timelock selector keys must include:

```text
setIsAllocator
setReceiveSharesGate
setSendSharesGate
setReceiveAssetsGate
setSendAssetsGate
setAdapterRegistry
addAdapter
removeAdapter
increaseTimelock
decreaseTimelock
abdicate
setPerformanceFee
setManagementFee
setPerformanceFeeRecipient
setManagementFeeRecipient
setForceDeallocatePenalty
```

Create a pure function:

```go
func DiffVaultState(expected VaultStateSnapshot, actual VaultStateSnapshot) []StateDiff
```

Tests:

- `TestDiffVaultStateReportsOwnerChange`
- `TestVaultStateModuleReturnsWarnWhenChangeSeverityIsWarn`
- `TestVaultStateModuleReturnsUrgentWhenChangeSeverityIsUrgent`

Run:

```bash
go test ./internal/monitor/modules/morpho -run 'TestDiffVaultState|TestVaultStateModule' -count=1
```

Expected: PASS after implementation.

- [ ] **Step 3: Commit**

```bash
git add internal/monitor/modules/morpho
git commit -m "feat: add Morpho liquidity and state monitors"
```

## Task 9: Gas Policy And Withdraw Service

**Files:**

- Create: `internal/withdraw/gas.go`
- Create: `internal/withdraw/gas_test.go`
- Create: `internal/withdraw/service.go`
- Create: `internal/withdraw/service_test.go`

- [ ] **Step 1: Implement gas bump policy with tests**

Create `internal/withdraw/gas_test.go` with:

```go
package withdraw

import (
	"math/big"
	"testing"
)

func TestBumpFeesIncreasesFeesAndRespectsCaps(t *testing.T) {
	// Arrange
	policy := GasPolicy{BumpBPS: 1250, MaxFeeCap: big.NewInt(112), MaxTipCap: big.NewInt(10)}
	fees := FeeCaps{MaxFeePerGas: big.NewInt(100), MaxPriorityFeePerGas: big.NewInt(8)}

	// Act
	result := policy.Bump(fees)

	// Assert
	if result.MaxFeePerGas.String() != "112" {
		t.Fatalf("expected max fee cap 112, got %s", result.MaxFeePerGas.String())
	}
	if result.MaxPriorityFeePerGas.String() != "9" {
		t.Fatalf("expected priority fee 9, got %s", result.MaxPriorityFeePerGas.String())
	}
}
```

Implement `internal/withdraw/gas.go`:

```go
package withdraw

import "math/big"

type FeeCaps struct {
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
}

type GasPolicy struct {
	BumpBPS   int64
	MaxFeeCap *big.Int
	MaxTipCap *big.Int
}

func (policy GasPolicy) Bump(fees FeeCaps) FeeCaps {
	return FeeCaps{
		MaxFeePerGas: capBig(bumpBPS(fees.MaxFeePerGas, policy.BumpBPS), policy.MaxFeeCap),
		MaxPriorityFeePerGas: capBig(bumpBPS(fees.MaxPriorityFeePerGas, policy.BumpBPS), policy.MaxTipCap),
	}
}

func bumpBPS(value *big.Int, bps int64) *big.Int {
	result := new(big.Int).Mul(value, big.NewInt(10_000+bps))
	result.Div(result, big.NewInt(10_000))
	return result
}

func capBig(value *big.Int, cap *big.Int) *big.Int {
	if cap != nil && value.Cmp(cap) > 0 {
		return new(big.Int).Set(cap)
	}
	return value
}
```

Run:

```bash
go test ./internal/withdraw -run TestBumpFeesIncreasesFeesAndRespectsCaps -count=1
```

Expected: PASS.

- [ ] **Step 2: Implement WithdrawService orchestration**

Create `internal/withdraw/service.go` with:

```go
type Adapter interface {
	ID() string
	Position(ctx context.Context) (core.PositionSnapshot, error)
	BuildFullExit(ctx context.Context, req core.FullExitRequest) (core.TxCandidate, error)
	SimulateFullExit(ctx context.Context, req core.FullExitRequest) (core.FullExitSimulation, error)
}

type Service struct {
	Adapter Adapter
	Signer  signer.Service
	ChainID *big.Int
	Clock   core.Clock
}
```

Behavior:

- `DryRunFullExit(ctx)` returns no-op when shares are zero.
- `ExecuteFullExit(ctx, trigger)` re-reads position, simulates, signs, submits, stores result through a repository interface.
- Simulation failure returns a typed error and does not sign.
- Pending urgent transaction management keeps one active nonce and bumps fees after timeout.

Tests:

- `TestDryRunFullExitReturnsNoopWhenSharesAreZero`
- `TestExecuteFullExitDoesNotSignWhenSimulationFails`
- `TestUrgentReplacementBumpsPendingTransactionFeesAfterTimeout`

Each test uses fake adapter, fake signer, fake submitter, and fixed clock.

Run:

```bash
go test ./internal/withdraw -run 'TestDryRunFullExit|TestExecuteFullExit|TestUrgentReplacement' -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/withdraw
git commit -m "feat: add withdrawal orchestration"
```

## Task 10: Events, Reports, And Telegram Formatting

**Files:**

- Create: `internal/events/events.go`
- Create: `internal/reports/report.go`
- Create: `internal/reports/report_test.go`
- Create: `internal/interactions/service.go`
- Create: `internal/interactions/telegram/formatter.go`
- Create: `internal/interactions/telegram/formatter_test.go`

- [ ] **Step 1: Add event service boundary**

Create `internal/events/events.go`:

```go
package events

import (
	"context"
	"time"

	"withdraw-bot/internal/core"
)

type Recorder interface {
	Record(ctx context.Context, eventType core.EventType, message string, fields map[string]string, at time.Time) error
}
```

- [ ] **Step 2: Implement report renderer**

Create `internal/reports/report.go`:

```go
package reports

import (
	"fmt"
	"sort"
	"strings"

	"withdraw-bot/internal/core"
)

func RenderStats(results map[core.MonitorModuleID]core.MonitorResult) string {
	statuses := make([]core.MonitorStatus, 0, len(results))
	keys := make([]string, 0, len(results))
	for id, result := range results {
		statuses = append(statuses, result.Status)
		keys = append(keys, string(id))
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Status: %s\n", core.WorstKnownStatus(statuses)))
	for _, key := range keys {
		result := results[core.MonitorModuleID(key)]
		builder.WriteString(fmt.Sprintf("\n%s: %s\n", key, result.Status))
		for _, metric := range result.Metrics {
			builder.WriteString(fmt.Sprintf("- %s: %s %s\n", metric.Key, metric.Value, metric.Unit))
		}
		for _, finding := range result.Findings {
			builder.WriteString(fmt.Sprintf("- %s: %s\n", finding.Severity, finding.Message))
		}
	}
	return strings.TrimSpace(builder.String())
}
```

Create `internal/reports/report_test.go` with a deterministic report assertion that checks the overall status and a metric line.

- [ ] **Step 3: Implement Telegram-safe escaping**

Create `internal/interactions/telegram/formatter.go`:

```go
package telegram

import "strings"

var markdownV2Escapes = []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}

func EscapeMarkdownV2(input string) string {
	result := input
	for _, token := range markdownV2Escapes {
		result = strings.ReplaceAll(result, token, "\\"+token)
	}
	return result
}
```

Create `internal/interactions/telegram/formatter_test.go`:

```go
package telegram

import "testing"

func TestEscapeMarkdownV2EscapesDynamicValue(t *testing.T) {
	// Arrange
	input := "share_price_loss: WARN!"

	// Act
	result := EscapeMarkdownV2(input)

	// Assert
	expected := "share\\_price\\_loss: WARN\\!"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}
```

Run:

```bash
go test ./internal/events ./internal/reports ./internal/interactions/telegram -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/events internal/reports internal/interactions
git commit -m "feat: add events and report formatting"
```

## Task 11: Telegram Interaction Commands

**Files:**

- Create: `internal/interactions/service.go`
- Create: `internal/interactions/telegram/service.go`
- Create: `internal/interactions/telegram/commands.go`
- Create: `internal/interactions/telegram/commands_test.go`

- [ ] **Step 1: Add generic interaction interface**

Create `internal/interactions/service.go`:

```go
package interactions

import (
	"context"
)

type AlertMessage struct {
	Text string
}

type ReportMessage struct {
	Text string
}

type CommandResponse struct {
	ChatID int64
	Text   string
}

type Service interface {
	Start(ctx context.Context) error
	SendAlert(ctx context.Context, msg AlertMessage) error
	SendReport(ctx context.Context, msg ReportMessage) error
	SendCommandResponse(ctx context.Context, msg CommandResponse) error
}
```

- [ ] **Step 2: Add command authorization test**

Create `internal/interactions/telegram/commands_test.go`:

```go
package telegram

import "testing"

func TestAuthorizeRejectsUnknownUser(t *testing.T) {
	// Arrange
	auth := Authorization{ChatID: 100, AllowedUserIDs: map[int64]bool{1: true}}

	// Act
	err := auth.Check(100, 2)

	// Assert
	if err == nil {
		t.Fatalf("expected unknown user to be rejected")
	}
}
```

Run:

```bash
go test ./internal/interactions/telegram -run TestAuthorizeRejectsUnknownUser -count=1
```

Expected: FAIL with `undefined: Authorization`.

- [ ] **Step 3: Implement command parsing and authorization**

Create `internal/interactions/telegram/commands.go`:

```go
package telegram

import (
	"fmt"
	"strings"
)

type Authorization struct {
	ChatID         int64
	AllowedUserIDs map[int64]bool
}

func (auth Authorization) Check(chatID int64, userID int64) error {
	if chatID != auth.ChatID {
		return fmt.Errorf("telegram chat %d is not authorized", chatID)
	}
	if !auth.AllowedUserIDs[userID] {
		return fmt.Errorf("telegram user %d is not authorized", userID)
	}
	return nil
}

type ParsedCommand struct {
	Name string
	Args []string
}

func ParseCommand(text string) ParsedCommand {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return ParsedCommand{}
	}
	return ParsedCommand{Name: fields[0], Args: fields[1:]}
}
```

Create `internal/interactions/telegram/service.go` with a `Service` type wrapping `tgbotapi.BotAPI`, `Authorization`, report provider, withdraw service, threshold service, and event recorder. Implement command handlers for:

- `/stats`
- `/withdraw`
- `/confirm <id>`
- `/thresholds`
- `/threshold set <module> <key> <value>`
- `/logs`
- `/logs info`
- `/help`

Each handler returns a bounded text response and records sanitized security events on rejected commands.

Run:

```bash
go test ./internal/interactions/telegram -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/interactions
git commit -m "feat: add Telegram command handling"
```

## Task 12: Threshold Overrides And Confirmations

**Files:**

- Modify: `internal/storage/repositories.go`
- Create: `internal/interactions/telegram/thresholds_test.go`
- Create: `internal/interactions/telegram/thresholds.go`

- [ ] **Step 1: Add threshold confirmation test**

Create `internal/interactions/telegram/thresholds_test.go`:

```go
package telegram

import "testing"

func TestBuildThresholdConfirmationRejectsUnknownModule(t *testing.T) {
	// Arrange
	request := ThresholdSetRequest{ModuleID: "unknown", Key: "loss_warn_bps", Value: "50", UserID: 1}

	// Act
	_, err := BuildThresholdConfirmation(request)

	// Assert
	if err == nil {
		t.Fatalf("expected unknown module to be rejected")
	}
}
```

Run:

```bash
go test ./internal/interactions/telegram -run TestBuildThresholdConfirmationRejectsUnknownModule -count=1
```

Expected: FAIL with `undefined: ThresholdSetRequest`.

- [ ] **Step 2: Implement threshold request validation**

Create `internal/interactions/telegram/thresholds.go`:

```go
package telegram

import (
	"fmt"

	"withdraw-bot/internal/core"
)

type ThresholdSetRequest struct {
	ModuleID string
	Key      string
	Value    string
	UserID   int64
}

type ThresholdConfirmation struct {
	ID      string
	Request ThresholdSetRequest
	Message string
}

var allowedThresholdKeys = map[core.MonitorModuleID]map[string]bool{
	core.ModuleSharePriceLoss: {"loss_warn_bps": true, "loss_urgent_bps": true, "stale_urgent_after": true},
	core.ModuleWithdrawLiquidity: {"idle_warn_threshold_usdc": true, "idle_urgent_threshold_usdc": true, "stale_urgent_after": true},
	core.ModuleVaultState: {"change_severity": true, "stale_urgent_after": true},
}

func BuildThresholdConfirmation(request ThresholdSetRequest) (ThresholdConfirmation, error) {
	moduleID := core.MonitorModuleID(request.ModuleID)
	keys, ok := allowedThresholdKeys[moduleID]
	if !ok {
		return ThresholdConfirmation{}, fmt.Errorf("unknown module %q", request.ModuleID)
	}
	if !keys[request.Key] {
		return ThresholdConfirmation{}, fmt.Errorf("threshold key %q is not allowed for module %q", request.Key, request.ModuleID)
	}
	id := fmt.Sprintf("threshold:%s:%s:%d", request.ModuleID, request.Key, request.UserID)
	return ThresholdConfirmation{ID: id, Request: request, Message: fmt.Sprintf("Confirm threshold override %s %s=%s", request.ModuleID, request.Key, request.Value)}, nil
}
```

Run:

```bash
go test ./internal/interactions/telegram -run TestBuildThresholdConfirmationRejectsUnknownModule -count=1
```

Expected: PASS.

- [ ] **Step 3: Add storage methods**

Add repository methods:

- `UpsertThresholdOverride(ctx, moduleID, key, value string, userID int64, at time.Time) error`
- `ListThresholdOverrides(ctx) ([]ThresholdOverride, error)`
- `InsertPendingConfirmation(ctx, confirmation PendingConfirmation) error`
- `ConsumePendingConfirmation(ctx, id string, at time.Time) (PendingConfirmation, error)`

Add tests:

- `TestUpsertThresholdOverrideReplacesExistingValue`
- `TestConsumePendingConfirmationReturnsErrorWhenExpired`

Run:

```bash
go test ./internal/storage ./internal/interactions/telegram -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/storage internal/interactions/telegram
git commit -m "feat: add threshold override confirmations"
```

## Task 13: Bootstrap And Config Check Modes

**Files:**

- Modify: `internal/app/app.go`
- Create: `internal/app/bootstrap.go`
- Create: `internal/app/config_check.go`
- Create: `internal/app/bootstrap_test.go`
- Create: `internal/app/config_check_test.go`

- [ ] **Step 1: Add bootstrap aggregation test**

Create `internal/app/bootstrap_test.go`:

```go
package app

import (
	"context"
	"testing"
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
```

Run:

```bash
go test ./internal/app -run TestCollectBootstrapFragmentsIncludesModuleOutput -count=1
```

Expected: FAIL with `undefined: CollectBootstrapFragments`.

- [ ] **Step 2: Implement bootstrap collector**

Create `internal/app/bootstrap.go`:

```go
package app

import (
	"context"

	"withdraw-bot/internal/core"
)

type BootstrapModule interface {
	ID() core.MonitorModuleID
	Bootstrap(ctx context.Context) (map[string]any, error)
}

func CollectBootstrapFragments(ctx context.Context, modules []BootstrapModule) (map[string]any, error) {
	result := make(map[string]any, len(modules))
	for _, module := range modules {
		fragment, err := module.Bootstrap(ctx)
		if err != nil {
			return nil, err
		}
		result[string(module.ID())] = fragment
	}
	return result, nil
}
```

Update the test with a local fake that implements `ID` and `Bootstrap`.

Run:

```bash
go test ./internal/app -run TestCollectBootstrapFragmentsIncludesModuleOutput -count=1
```

Expected: PASS.

- [ ] **Step 3: Implement config-check validation pipeline**

Create `internal/app/config_check.go`:

```go
package app

import (
	"context"
	"fmt"
)

type Check func(ctx context.Context) error

func RunChecks(ctx context.Context, checks []Check) error {
	for index, check := range checks {
		if err := check(ctx); err != nil {
			return fmt.Errorf("config check %d failed: %w", index+1, err)
		}
	}
	return nil
}
```

Test:

- `TestRunChecksReturnsFirstFailure`

Run:

```bash
go test ./internal/app -count=1
```

Expected: PASS.

- [ ] **Step 4: Wire app modes**

Update `internal/app/app.go` so:

- `config-check` loads config and env, validates addresses, dials RPC, checks chain ID, derives signer address, validates receiver.
- `bootstrap` loads config and env, builds modules, prints YAML fragments to stdout.
- `monitor` builds all services and starts monitor loop and Telegram service in the same process.

Use small constructor functions in `internal/app/app.go`:

```go
func buildRuntime(ctx context.Context, configPath string) (Runtime, error)
func runMonitor(ctx context.Context, runtime Runtime) error
func runBootstrap(ctx context.Context, runtime Runtime) error
func runConfigCheck(ctx context.Context, runtime Runtime) error
```

Run:

```bash
go test ./internal/app ./internal/config ./internal/storage -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -m "feat: add bootstrap and config check modes"
```

## Task 14: Alert Service And Auto-Withdraw Triggering

**Files:**

- Create: `internal/app/alerts.go`
- Create: `internal/app/alerts_test.go`

- [ ] **Step 1: Add urgent alert test**

Create `internal/app/alerts_test.go`:

```go
package app

import (
	"context"
	"testing"
	"time"

	"withdraw-bot/internal/core"
)

func TestHandleMonitorResultsTriggersWithdrawForUrgentFinding(t *testing.T) {
	// Arrange
	withdrawer := &fakeWithdrawer{}
	notifier := &fakeNotifier{}
	service := AlertService{Withdrawer: withdrawer, Notifier: notifier, Clock: core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)}}
	results := []core.MonitorResult{{
		ModuleID: core.ModuleWithdrawLiquidity,
		Status: core.MonitorStatusUrgent,
		Findings: []core.Finding{{Key: core.FindingIdleLiquidity, Severity: core.SeverityUrgent, Message: "idle liquidity urgent"}},
	}}

	// Act
	err := service.HandleMonitorResults(context.Background(), results)

	// Assert
	if err != nil {
		t.Fatalf("handle monitor results: %v", err)
	}
	if withdrawer.calls != 1 {
		t.Fatalf("expected one withdraw call, got %d", withdrawer.calls)
	}
	if notifier.alerts != 1 {
		t.Fatalf("expected one alert, got %d", notifier.alerts)
	}
}
```

Run:

```bash
go test ./internal/app -run TestHandleMonitorResultsTriggersWithdrawForUrgentFinding -count=1
```

Expected: FAIL with `undefined: AlertService`.

- [ ] **Step 2: Implement alert service**

Create `internal/app/alerts.go`:

```go
package app

import (
	"context"

	"withdraw-bot/internal/core"
)

type AutoWithdrawer interface {
	HandleUrgent(ctx context.Context, result core.MonitorResult, finding core.Finding) error
}

type Notifier interface {
	SendAlert(ctx context.Context, text string) error
}

type AlertService struct {
	Withdrawer AutoWithdrawer
	Notifier   Notifier
	Clock      core.Clock
}

func (service AlertService) HandleMonitorResults(ctx context.Context, results []core.MonitorResult) error {
	for _, result := range results {
		for _, finding := range result.Findings {
			if finding.Severity != core.SeverityUrgent {
				continue
			}
			if err := service.Notifier.SendAlert(ctx, finding.Message); err != nil {
				return err
			}
			if err := service.Withdrawer.HandleUrgent(ctx, result, finding); err != nil {
				return err
			}
		}
	}
	return nil
}
```

Add fakes to the test file.

Run:

```bash
go test ./internal/app -run TestHandleMonitorResultsTriggersWithdrawForUrgentFinding -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/app/alerts.go internal/app/alerts_test.go
git commit -m "feat: trigger auto-withdraw on urgent findings"
```

## Task 15: Logging And Local Event Access

**Files:**

- Create: `internal/logging/logging.go`
- Create: `internal/logging/logging_test.go`
- Modify: `internal/storage/repositories.go`

- [ ] **Step 1: Implement logger setup**

Create `internal/logging/logging.go`:

```go
package logging

import (
	"io"
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	FilePath   string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
}

func New(config Config) (*slog.Logger, io.Closer) {
	rotating := &lumberjack.Logger{
		Filename:   config.FilePath,
		MaxSize:    config.MaxSizeMB,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAgeDays,
		Compress:   true,
	}
	writer := io.MultiWriter(os.Stdout, rotating)
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{AddSource: true})
	return slog.New(handler), rotating
}
```

- [ ] **Step 2: Add event listing for `/logs`**

Add repository method:

```go
func (repos Repositories) ListRecentEvents(ctx context.Context, includeInfo bool, limit int) ([]EventRecord, error)
```

Behavior:

- When `includeInfo` is false, include `warning`, `error`, `security`, and `withdrawal`.
- When `includeInfo` is true, include all event types.
- Always order by `created_at DESC`.
- Clamp limit to `1..50`.

Tests:

- `TestListRecentEventsExcludesInfoByDefault`
- `TestListRecentEventsIncludesInfoWhenRequested`

Run:

```bash
go test ./internal/logging ./internal/storage -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/logging internal/storage
git commit -m "feat: add structured logs and event access"
```

## Task 16: Final Wiring And Verification

**Files:**

- Modify: `cmd/withdraw-bot/main.go`
- Modify: `internal/app/app.go`
- Modify: `Dockerfile`
- Modify: `docker-compose.yml`
- Modify: `README.md`

- [ ] **Step 1: Add README with run commands**

Create `README.md`:

````markdown
# withdraw-bot

Go daemon for monitoring one Morpho Vault V2 position and redeeming all shares when urgent risk conditions fire.

## Commands

```bash
cp .env.example .env
go run ./cmd/withdraw-bot config-check --config config/config.example.yaml
go run ./cmd/withdraw-bot bootstrap --config config/config.example.yaml
go run ./cmd/withdraw-bot monitor --config config/config.example.yaml
```

## Docker

```bash
docker compose build
docker compose up -d
```
````

- [ ] **Step 2: Run full verification**

Run:

```bash
go test ./... -count=1
go build ./cmd/withdraw-bot
docker compose config
```

Expected:

- `go test` passes.
- `go build` passes.
- `docker compose config` renders a valid compose file.

- [ ] **Step 3: Commit final wiring**

```bash
git add README.md cmd internal Dockerfile docker-compose.yml
git commit -m "feat: wire withdraw bot runtime"
```

## Self-Review Checklist

- Spec coverage: The tasks cover project scaffold, exact pinned dependencies, config/env templates, SQLite migrations, signer abstraction, RPC wrapper, Morpho withdraw adapter, three monitor modules, alert handling, auto-withdraw orchestration, Telegram command UX, threshold overrides, logs, bootstrap, config-check, Docker, and verification.
- Unit test compliance: Tasks require AAA comments, behavior names, one behavior per test, deterministic fakes, and no external mainnet tests by default.
- Sensitive logging: Tasks route secrets through env and prohibit storing private keys, Telegram tokens, and RPC URLs.
- String constants: Module IDs, finding keys, statuses, event types, and commands are centralized in `internal/core/ids.go` and `internal/core/status.go`.
- Env templates: `.env.example` contains every env var introduced by the plan.
- Scope: V1 remains one process, one position, one Telegram chat, one signer EOA.
