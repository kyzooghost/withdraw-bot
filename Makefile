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
