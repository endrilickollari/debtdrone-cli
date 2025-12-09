FROM golang:1.25-bookworm AS builder
WORKDIR /app
RUN apt-get update && apt-get install -y gcc g++ make
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o debtdrone ./cmd/debtdrone
FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y ca-certificates git && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/debtdrone /usr/local/bin/debtdrone
ENTRYPOINT ["debtdrone"]
