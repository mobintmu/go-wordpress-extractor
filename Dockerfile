# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.27.4-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY docs/ ./docs/
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/apikey ./cmd/apikey && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sitectl ./cmd/sitectl

# ---- run, with the browser fetch engine ----
# Build with: docker build --target runtime-browser -t product-extractor:browser .
# Only instances that serve sites with "fetch": {"engine": "browser"} need
# this image (~1.5 GB, plus ~150 MB RAM for Chrome and 100-300 MB per open
# page). The Playwright image ships Chromium and its system libraries; the
# Go driver is node + playwright-core of the same version as playwright-go.
FROM mcr.microsoft.com/playwright:v1.57.0-noble AS runtime-browser
RUN apt-get update && apt-get install -y --no-install-recommends wget tzdata && rm -rf /var/lib/apt/lists/* && \
    mkdir -p /opt/playwright-driver && cd /tmp && \
    npm pack playwright-core@1.57.0 --silent && \
    tar -xzf playwright-core-1.57.0.tgz -C /opt/playwright-driver && rm playwright-core-1.57.0.tgz && \
    ln -s "$(command -v node)" /opt/playwright-driver/node && \
    useradd -u 10001 -m appuser
WORKDIR /app
COPY --from=build /out/server /out/apikey /out/sitectl /usr/local/bin/
COPY configs/ ./configs/
RUN mkdir -p /app/secrets && chown appuser /app/secrets
USER appuser
ENV APP_ENV=production \
    APP_HTTP_PORT=4000 \
    APP_SITE_CONFIG_DIR=/app/configs \
    APP_API_KEYS_FILE=/app/secrets/.env.apikeys \
    APP_BROWSER_ENABLED=true \
    APP_BROWSER_CHANNEL=chromium \
    PLAYWRIGHT_DRIVER_PATH=/opt/playwright-driver
EXPOSE 4000
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD wget -qO- --no-check-certificate "$( [ -n "$APP_TLS_CERT_FILE" ] && echo https || echo http )://127.0.0.1:${APP_HTTP_PORT}/ready" >/dev/null || exit 1
CMD ["server"]

# ---- run (default: HTTP engine only) ----
# The last stage stays the default build target, so a plain docker build
# still produces the small alpine image.
FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /out/server /out/apikey /out/sitectl /usr/local/bin/
# Site configs are baked in as a default; mount ./configs over them to
# update without rebuilding. API key files are never baked in: mount them.
COPY configs/ ./configs/
RUN mkdir -p /app/secrets && chown appuser /app/secrets
USER appuser
ENV APP_ENV=production \
    APP_HTTP_PORT=4000 \
    APP_SITE_CONFIG_DIR=/app/configs \
    APP_API_KEYS_FILE=/app/secrets/.env.apikeys
EXPOSE 4000
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- --no-check-certificate "$( [ -n "$APP_TLS_CERT_FILE" ] && echo https || echo http )://127.0.0.1:${APP_HTTP_PORT}/ready" >/dev/null || exit 1
CMD ["server"]
