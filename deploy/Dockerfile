# syntax=docker/dockerfile:1
FROM node:22-alpine AS frontend
WORKDIR /workspace/web
COPY web/package*.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /workspace/webassets/dist/ ./webassets/dist/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /quilldns ./cmd/quilldns

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /quilldns /quilldns
WORKDIR /data
USER nonroot:nonroot
VOLUME ["/data"]
EXPOSE 1053/udp 1053/tcp 8080
ENTRYPOINT ["/quilldns"]
CMD ["-data","/data/quilldns.db","-dns","0.0.0.0:1053","-allow-networks","127.0.0.0/8,::1/128"]
