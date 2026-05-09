package config

import "testing"

func TestValidateReturnsErrorWhenTelegramChatIDIsMissing(t *testing.T) {
	// Arrange
	cfg := Config{
		App:      AppConfig{MonitorInterval: "5m", DataDir: "./data"},
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
