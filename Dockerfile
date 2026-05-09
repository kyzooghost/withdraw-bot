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
