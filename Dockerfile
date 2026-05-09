FROM golang:1.25.10-bookworm@sha256:9422886b8f9b52e88344a24e9b05fd4b37d42233b680019fc3cb6b1fb2f2b0a5 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/withdraw-bot ./cmd/withdraw-bot

FROM gcr.io/distroless/base-debian12@sha256:9dce90e688a57e59ce473ff7bc4c80bc8fe52d2303b4d99b44f297310bbd2210
WORKDIR /app
COPY --from=build /out/withdraw-bot /usr/local/bin/withdraw-bot
ENTRYPOINT ["withdraw-bot"]
CMD ["monitor", "--config", "/app/config.yaml"]
