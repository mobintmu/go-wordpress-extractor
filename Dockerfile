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

# ---- run ----
FROM alpine:3.22
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
