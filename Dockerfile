# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.22-bookworm AS backend-builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/urestic ./cmd/urestic

FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata restic rclone \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /app/data /app/web /backups /restore
COPY --from=backend-builder /out/urestic /usr/local/bin/urestic
COPY --from=frontend-builder /src/frontend/dist /app/web
ENV URESTIC_ADDR=:8080 \
    URESTIC_LANG=zh-CN \
    URESTIC_DATA_DIR=/app/data \
    URESTIC_DATABASE_PATH=/app/data/urestic.db \
    URESTIC_WEB_DIR=/app/web \
    URESTIC_AUTH_ENABLED=true \
    URESTIC_ADMIN_USERNAME=admin \
    URESTIC_SESSION_TTL_HOURS=12 \
    URESTIC_RCLONE_CONFIG=/app/data/rclone/rclone.conf \
    URESTIC_RCLONE_IMPORT_PATH=/host-rclone/rclone.conf \
    URESTIC_RCLONE_CACHE_DIR=/app/data/rclone/cache \
    GIN_MODE=release
EXPOSE 8080
CMD ["urestic"]
