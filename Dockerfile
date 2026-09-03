# syntax=docker/dockerfile:1

# --- Build stage -----------------------------------------------------------
FROM golang:1.27-alpine AS build

WORKDIR /src

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Static-ish binary: no cgo (modernc.org/sqlite is pure Go), static net
# resolver, stripped. TARGETOS/TARGETARCH are set automatically by BuildKit
# for multi-platform builds.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -tags 'osusernet,netgo' \
      -ldflags '-s -w -extldflags "-static"' \
      -o /out/oddity ./cmd/oddity

# --- Runtime stage ---------------------------------------------------------
FROM alpine:3.22

# ca-certificates for outbound TLS (R2), tzdata for correct local times,
# wget for the container healthcheck.
RUN apk add --no-cache ca-certificates tzdata wget \
 && addgroup -S oddity \
 && adduser -S -G oddity -h /app oddity \
 && mkdir -p /data \
 && chown -R oddity:oddity /data

WORKDIR /app
COPY --from=build /out/oddity /app/oddity

# All persistent state (SQLite database, uploaded media) lives under /data.
VOLUME ["/data"]

# Sensible container defaults; compose/deployments override as needed.
# The server listens on all interfaces inside the container; expose it on the
# host only via a loopback port mapping (127.0.0.1:8890:8890).
ENV ODDITY_ADDR=0.0.0.0:8890 \
    ODDITY_DB_PATH=/data/oddity.db \
    STORAGE_DRIVER=local \
    LOCAL_STORAGE_PATH=/data/media \
    ADMIN_COOKIE_PATH=/admin

USER oddity
EXPOSE 8890

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8890/health || exit 1

ENTRYPOINT ["/app/oddity"]
