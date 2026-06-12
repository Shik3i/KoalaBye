# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS builder

WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/koalabye ./cmd/koalabye

FROM alpine:3.22
RUN apk add --no-cache ca-certificates wget \
    && addgroup -S koalabye \
    && adduser -S -G koalabye -h /app koalabye \
    && mkdir -p /data \
    && chown koalabye:koalabye /data
WORKDIR /app
COPY --from=builder /out/koalabye /usr/local/bin/koalabye
USER koalabye
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/koalabye"]
