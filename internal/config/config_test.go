package config

import (
	"strings"
	"testing"
)

const (
	testVaultAddress        = "0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0"
	testPrivateKey          = "fake-private-key"
	testTelegramToken       = "fake-telegram-token"
	testPrimaryRPCURL       = "https://primary.invalid"
	testFallbackRPCURL      = "https://fallback.invalid"
	testOtherFallbackRPCURL = "https://other-fallback.invalid"
)

func testValidConfig() Config {
	return Config{
		App: AppConfig{MonitorInterval: "5m", DataDir: "./data"},
		Ethereum: EthereumConfig{
			ChainID:       1,
			VaultAddress:  testVaultAddress,
			AssetDecimals: 6,
		},
		Telegram: TelegramConfig{ChatID: 1, AllowedUserIDs: []int64{123}},
		Gas: GasConfig{
			ReplacementTimeout:       "2m",
			GasLimitBufferBPS:        2000,
			FeeBumpBPS:               1250,
			MaxFeePerGasGwei:         "200",
			MaxPriorityFeePerGasGwei: "5",
		},
	}
}

func TestValidateReturnsErrorWhenTelegramChatIDIsMissing(t *testing.T) {
	// Arrange
	cfg := testValidConfig()
	cfg.Telegram.ChatID = 0

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Fatalf("expected missing chat ID to be rejected")
	}
}

func TestValidateReturnsErrorWhenMonitorIntervalIsZero(t *testing.T) {
	// Arrange
	cfg := testValidConfig()
	cfg.App.MonitorInterval = "0s"

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Fatalf("expected zero monitor interval to be rejected")
	}
}

func TestValidateReturnsErrorWhenMonitorIntervalIsNegative(t *testing.T) {
	// Arrange
	cfg := testValidConfig()
	cfg.App.MonitorInterval = "-1s"

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Fatalf("expected negative monitor interval to be rejected")
	}
}

func TestValidateReturnsErrorWhenGasLimitBufferBPSIsInvalid(t *testing.T) {
	// Arrange
	cfg := testValidConfig()
	cfg.Gas.GasLimitBufferBPS = 10001

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Fatalf("expected invalid gas limit buffer bps to be rejected")
	}
}

func TestValidateReturnsErrorWhenFeeBumpBPSIsInvalid(t *testing.T) {
	// Arrange
	cfg := testValidConfig()
	cfg.Gas.FeeBumpBPS = -1

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Fatalf("expected invalid fee bump bps to be rejected")
	}
}

func TestValidateReturnsErrorWhenGasReplacementTimeoutIsMissing(t *testing.T) {
	// Arrange
	cfg := testValidConfig()
	cfg.Gas.ReplacementTimeout = ""

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Fatalf("expected missing gas replacement timeout to be rejected")
	}
}

func TestValidateReturnsErrorWhenGasReplacementTimeoutIsZero(t *testing.T) {
	// Arrange
	cfg := testValidConfig()
	cfg.Gas.ReplacementTimeout = "0s"

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Fatalf("expected zero gas replacement timeout to be rejected")
	}
}

func TestValidateReturnsErrorWhenMaxFeePerGasGweiIsInvalid(t *testing.T) {
	// Arrange
	cfg := testValidConfig()
	cfg.Gas.MaxFeePerGasGwei = "1e6"

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Fatalf("expected invalid max fee per gas gwei to be rejected")
	}
}

func TestValidateReturnsErrorWhenMaxPriorityFeePerGasGweiIsInvalid(t *testing.T) {
	// Arrange
	cfg := testValidConfig()
	cfg.Gas.MaxPriorityFeePerGasGwei = "1/2"

	// Act
	err := cfg.Validate()

	// Assert
	if err == nil {
		t.Fatalf("expected invalid max priority fee per gas gwei to be rejected")
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

func TestParseDecimalUnitsRejectsNonDecimalGrammar(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "fraction", value: "1/2"},
		{name: "scientific notation", value: "1e6"},
		{name: "hex prefix", value: "0x10"},
		{name: "binary prefix", value: "0b10"},
		{name: "positive sign", value: "+1"},
		{name: "negative sign", value: "-1"},
		{name: "leading decimal point", value: ".1"},
		{name: "trailing decimal point", value: "1."},
		{name: "multiple decimal points", value: "1.2.3"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			value := test.value

			// Act
			_, err := ParseDecimalUnits("amount", value, 6)

			// Assert
			if err == nil {
				t.Fatalf("expected %q to be rejected", value)
			}
		})
	}
}

func TestValidateBPSAcceptsBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		value int64
	}{
		{name: "zero", value: 0},
		{name: "maximum", value: 10000},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			value := test.value

			// Act
			err := ValidateBPS("gas_limit_buffer_bps", value)

			// Assert
			if err != nil {
				t.Fatalf("expected %d bps to be accepted: %v", value, err)
			}
		})
	}
}

func TestValidateBPSRejectsOutOfRangeValues(t *testing.T) {
	tests := []struct {
		name  string
		value int64
	}{
		{name: "below minimum", value: -1},
		{name: "above maximum", value: 10001},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			value := test.value

			// Act
			err := ValidateBPS("gas_limit_buffer_bps", value)

			// Assert
			if err == nil {
				t.Fatalf("expected %d bps to be rejected", value)
			}
		})
	}
}

func TestLoadSecretsFromEnvReturnsTrimmedSecretsAndFallbackRPCURLs(t *testing.T) {
	// Arrange
	t.Setenv(EnvPrivateKey, " "+testPrivateKey+" ")
	t.Setenv(EnvTelegramToken, " "+testTelegramToken+" ")
	t.Setenv(EnvPrimaryRPCURL, " "+testPrimaryRPCURL+" ")
	t.Setenv(EnvFallbackRPCURLs, " "+testFallbackRPCURL+" , , "+testOtherFallbackRPCURL+" , ")

	// Act
	secrets, err := LoadSecretsFromEnv()

	// Assert
	if err != nil {
		t.Fatalf("expected secrets to load: %v", err)
	}
	if secrets.PrivateKey != testPrivateKey {
		t.Fatalf("expected trimmed private key")
	}
	if secrets.TelegramToken != testTelegramToken {
		t.Fatalf("expected trimmed telegram token")
	}
	if secrets.PrimaryRPCURL != testPrimaryRPCURL {
		t.Fatalf("expected trimmed primary rpc url")
	}
	if len(secrets.FallbackRPCURLs) != 2 {
		t.Fatalf("expected 2 fallback rpc urls, got %d", len(secrets.FallbackRPCURLs))
	}
	if secrets.FallbackRPCURLs[0] != testFallbackRPCURL {
		t.Fatalf("expected first fallback rpc url to be %q, got %q", testFallbackRPCURL, secrets.FallbackRPCURLs[0])
	}
	if secrets.FallbackRPCURLs[1] != testOtherFallbackRPCURL {
		t.Fatalf("expected second fallback rpc url to be %q, got %q", testOtherFallbackRPCURL, secrets.FallbackRPCURLs[1])
	}
}

func TestLoadSecretsFromEnvReturnsErrorWhenPrivateKeyIsMissing(t *testing.T) {
	// Arrange
	t.Setenv(EnvPrivateKey, " ")
	t.Setenv(EnvTelegramToken, testTelegramToken)
	t.Setenv(EnvPrimaryRPCURL, testPrimaryRPCURL)

	// Act
	_, err := LoadSecretsFromEnv()

	// Assert
	if err == nil {
		t.Fatalf("expected missing private key to be rejected")
	}
	if !strings.Contains(err.Error(), EnvPrivateKey) {
		t.Fatalf("expected error to name %s, got %q", EnvPrivateKey, err.Error())
	}
}

func TestLoadSecretsFromEnvReturnsErrorWhenTelegramTokenIsMissing(t *testing.T) {
	// Arrange
	t.Setenv(EnvPrivateKey, testPrivateKey)
	t.Setenv(EnvTelegramToken, " ")
	t.Setenv(EnvPrimaryRPCURL, testPrimaryRPCURL)

	// Act
	_, err := LoadSecretsFromEnv()

	// Assert
	if err == nil {
		t.Fatalf("expected missing telegram token to be rejected")
	}
	if !strings.Contains(err.Error(), EnvTelegramToken) {
		t.Fatalf("expected error to name %s, got %q", EnvTelegramToken, err.Error())
	}
}

func TestLoadSecretsFromEnvReturnsErrorWhenPrimaryRPCURLIsMissing(t *testing.T) {
	// Arrange
	t.Setenv(EnvPrivateKey, testPrivateKey)
	t.Setenv(EnvTelegramToken, testTelegramToken)
	t.Setenv(EnvPrimaryRPCURL, " ")

	// Act
	_, err := LoadSecretsFromEnv()

	// Assert
	if err == nil {
		t.Fatalf("expected missing primary rpc url to be rejected")
	}
	if !strings.Contains(err.Error(), EnvPrimaryRPCURL) {
		t.Fatalf("expected error to name %s, got %q", EnvPrimaryRPCURL, err.Error())
	}
}

func TestLoadSecretsFromEnvDoesNotIncludeSecretValuesInError(t *testing.T) {
	// Arrange
	t.Setenv(EnvPrivateKey, testPrivateKey)
	t.Setenv(EnvTelegramToken, testTelegramToken)
	t.Setenv(EnvPrimaryRPCURL, " ")

	// Act
	_, err := LoadSecretsFromEnv()

	// Assert
	if err == nil {
		t.Fatalf("expected missing primary rpc url to be rejected")
	}
	if strings.Contains(err.Error(), testPrivateKey) {
		t.Fatalf("expected error to omit private key value")
	}
	if strings.Contains(err.Error(), testTelegramToken) {
		t.Fatalf("expected error to omit telegram token value")
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
