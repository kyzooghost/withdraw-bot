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
