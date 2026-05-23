package config

import (
	"encoding/json"
	"fmt"
	"math/big"

	"withdraw-bot/internal/core"

	"gopkg.in/yaml.v3"
)

const (
	moduleConfigKeyEnabled     = "enabled"
	errMissingModuleEnabled    = "%s.enabled is required"
	errInvalidModuleEnabled    = "%s.enabled must be a bool"
	errMissingModuleField      = "%s.%s is required"
	errInvalidModuleInteger    = "%s.%s must be an integer"
	errInvalidModuleString     = "%s.%s must be a string"
)

// ModuleEnabled returns whether the module is enabled in its config block.
func ModuleEnabled(moduleConfig ModuleConfig, moduleID core.MonitorModuleID) (bool, error) {
	value, ok := moduleConfig[moduleConfigKeyEnabled]
	if !ok {
		return false, fmt.Errorf(errMissingModuleEnabled, moduleID)
	}
	enabled, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf(errInvalidModuleEnabled, moduleID)
	}
	return enabled, nil
}

// EnabledModuleConfig returns the module config if the module exists and is enabled.
func EnabledModuleConfig(cfg Config, moduleID core.MonitorModuleID) (ModuleConfig, bool, error) {
	moduleConfig, ok := cfg.Modules[string(moduleID)]
	if !ok {
		return nil, false, nil
	}
	enabled, err := ModuleEnabled(moduleConfig, moduleID)
	if err != nil {
		return nil, false, err
	}
	if !enabled {
		return nil, false, nil
	}
	return moduleConfig, true, nil
}

// ModuleString extracts a required non-empty string from module config.
func ModuleString(moduleConfig ModuleConfig, moduleID core.MonitorModuleID, key string) (string, error) {
	value, ok := moduleConfig[key]
	if !ok {
		return "", fmt.Errorf(errMissingModuleField, moduleID, key)
	}
	text, ok := value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf(errInvalidModuleString, moduleID, key)
	}
	return text, nil
}

// ModuleBigInt extracts a required base-10 big.Int string from module config.
func ModuleBigInt(moduleConfig ModuleConfig, moduleID core.MonitorModuleID, key string) (*big.Int, error) {
	text, err := ModuleString(moduleConfig, moduleID, key)
	if err != nil {
		return nil, err
	}
	value, ok := new(big.Int).SetString(text, 10)
	if !ok {
		return nil, fmt.Errorf(errInvalidModuleString, moduleID, key)
	}
	return value, nil
}

// ModuleInt64 extracts a required integer from module config.
func ModuleInt64(moduleConfig ModuleConfig, moduleID core.MonitorModuleID, key string) (int64, error) {
	value, ok := moduleConfig[key]
	if !ok {
		return 0, fmt.Errorf(errMissingModuleField, moduleID, key)
	}
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	default:
		return 0, fmt.Errorf(errInvalidModuleInteger, moduleID, key)
	}
}

// ModuleDecimalUnits extracts a decimal string and scales it to on-chain units.
func ModuleDecimalUnits(moduleConfig ModuleConfig, moduleID core.MonitorModuleID, key string, decimals uint8) (*big.Int, error) {
	text, err := ModuleString(moduleConfig, moduleID, key)
	if err != nil {
		return nil, err
	}
	return ParseDecimalUnits(fmt.Sprintf("%s.%s", moduleID, key), text, decimals)
}

// ModuleStruct unmarshals a nested config value into an arbitrary struct via YAML->JSON round-trip.
func ModuleStruct(moduleConfig ModuleConfig, moduleID core.MonitorModuleID, key string, target any) error {
	value, ok := moduleConfig[key]
	if !ok {
		return fmt.Errorf(errMissingModuleField, moduleID, key)
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	var intermediate any
	if err := yaml.Unmarshal(data, &intermediate); err != nil {
		return err
	}
	normalized, err := json.Marshal(intermediate)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(normalized, target); err != nil {
		return err
	}
	return nil
}
