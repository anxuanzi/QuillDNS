# syntax=docker/dockerfile:1
# Build the Vue 3 SPA with Bun, then embed it into the Go executable.
FROM oven/bun:1.3.5-alpine AS frontend
WORKDIR /workspace/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /workspace/webassets/dist/ ./webassets/dist/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /quilldns ./cmd/quilldns \
    && mkdir -p /image-data && chmod 0700 /image-data

# Non-root, no runtime shell, no Bun/Node installation, no external services.
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="QuillDNS" \
      org.opencontainers.image.description="Self-hosted DNS proxy with integrated management UI and observability" \
      org.opencontainers.image.source="https://github.com/anxuanzi/QuillDNS" \
      org.opencontainers.image.url="https://github.com/anxuanzi/QuillDNS" \
      org.opencontainers.image.licenses="Apache-2.0"
COPY --from=builder /quilldns /quilldns
COPY --chown=nonroot:nonroot --from=builder /image-data /data
WORKDIR /data
USER nonroot:nonroot

# Safe defaults for Linux host-network containers. Override with QUILLDNS_* variables.
# HTTP and DNS remain loopback-only unless the operator explicitly opts in.
ENV QUILLDNS_DATA_PATH=/data/quilldns.db \
    QUILLDNS_DNS_LISTEN=127.0.0.1:1053 \
    QUILLDNS_HTTP_LISTEN=127.0.0.1:8080 \
    QUILLDNS_UPSTREAMS=1.1.1.1:53,8.8.8.8:53 \
    QUILLDNS_ALLOW_NETWORKS=127.0.0.0/8,::1/128

EXPOSE 1053/udp 1053/tcp 8080/tcp
ENTRYPOINT ["/quilldns"]
