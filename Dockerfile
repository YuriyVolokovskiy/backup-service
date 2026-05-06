FROM golang:1.22-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/backup-service ./cmd/backup-service

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata postgresql-client \
    && rm -rf /var/lib/apt/lists/*

RUN useradd --system --create-home --uid 10001 backup
COPY --from=builder /out/backup-service /usr/local/bin/backup-service

USER backup
ENTRYPOINT ["backup-service"]
CMD ["serve", "--config", "/etc/backup-service/config.yaml"]
