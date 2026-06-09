FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/api

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium \
    ca-certificates \
    fonts-liberation \
    fonts-noto \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /root

COPY --from=builder /app/main .

ENV STAGE=prod
ENV APP_ENV=prod
ENV CHROME_PATH=/usr/bin/chromium

EXPOSE 4000

CMD ["./main"]
