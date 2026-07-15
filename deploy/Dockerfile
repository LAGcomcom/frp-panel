# Build stage
FROM node:22-alpine AS frontend

WORKDIR /src
COPY web/admin/package*.json web/admin/
COPY web/user/package*.json web/user/
RUN cd web/admin && npm ci && cd ../user && npm ci
COPY web ./web
RUN cd web/admin && npm run build && cd ../user && npm run build

FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

COPY --from=frontend /src/web/admin/dist ./web/admin/dist
COPY --from=frontend /src/web/user/dist ./web/user/dist
RUN mkdir -p internal/api/dist/landing internal/api/dist/admin internal/api/dist/user internal/api/dist/license internal/api/bin \
    && cp web/landing/index.html internal/api/dist/landing/index.html \
    && cp -r web/admin/dist/. internal/api/dist/admin/ \
    && cp -r web/user/dist/. internal/api/dist/user/ \
    && cp web/license/index.html internal/api/dist/license/index.html \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o internal/api/bin/agent ./cmd/agent/

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /frp-panel ./cmd/panel/

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary
COPY --from=builder /frp-panel /app/frp-panel

# Copy config template
COPY deploy/config.yaml.example /app/config.yaml

# Create data directory
RUN mkdir -p /app/data

# Expose port
EXPOSE 8080

# Run
CMD ["/app/frp-panel", "-config", "/app/config.yaml"]
