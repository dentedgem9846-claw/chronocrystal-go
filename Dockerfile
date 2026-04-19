# Builder stage — compile with CGO for sqlite3
FROM golang:1.26-bookworm AS builder

RUN apt-get update && apt-get install -y gcc libc6-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o chronocrystal ./cmd/chronocrystal

# Runtime stage — minimal image
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /src/chronocrystal .
COPY --from=builder /src/config.example.toml .

VOLUME ["/app/data", "/app/skills", "/app/tools"]

CMD ["./chronocrystal", "start"]