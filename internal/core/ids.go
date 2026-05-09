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
	FindingIdleLiquidity  FindingKey = "idle_liquidity"
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
