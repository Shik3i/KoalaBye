# syntax=docker/dockerfile:1
FROM golang:1.26.4-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X github.com/koalastuff/koalabye/internal/version.Version=${VERSION} -X github.com/koalastuff/koalabye/internal/version.Commit=${COMMIT} -X github.com/koalastuff/koalabye/internal/version.BuildDate=${BUILD_DATE}" \
    -o /out/koalabye ./cmd/koalabye

FROM alpine:3.22
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG SOURCE_URL=https://github.com/koalastuff/koalabye
LABEL org.opencontainers.image.title="KoalaBye" \
      org.opencontainers.image.description="Privacy-first uninstall feedback platform" \
      org.opencontainers.image.source="${SOURCE_URL}" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"
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
