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
