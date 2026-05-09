package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App      AppConfig               `yaml:"app"`
	Ethereum EthereumConfig          `yaml:"ethereum"`
	Telegram TelegramConfig          `yaml:"telegram"`
	Gas      GasConfig               `yaml:"gas"`
	Logs     LogConfig               `yaml:"logs"`
	Modules  map[string]ModuleConfig `yaml:"modules"`
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
