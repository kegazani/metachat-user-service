# Build stage
FROM golang:1.24 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential librdkafka-dev pkg-config ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY metachat-event-sourcing ../metachat-event-sourcing
COPY metachat-user-service/go.mod metachat-user-service/go.sum ./
RUN go mod download

COPY metachat-user-service/ .

RUN go mod tidy

RUN CGO_ENABLED=1 GOOS=linux go build -o main ./cmd/main.go

FROM debian:stable-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates librdkafka1 && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/main .

COPY --from=builder /app/config ./config

EXPOSE 8080 50051

CMD ["./main"]