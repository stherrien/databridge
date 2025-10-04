# Multi-stage Dockerfile for DataBridge
# Stage 1: Build the Go backend
FROM golang:1.21-alpine AS backend-builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application
ARG VERSION=docker
ARG BUILD_DATE
ARG GIT_COMMIT
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-X main.version=${VERSION} -X main.buildDate=${BUILD_DATE} -X main.gitCommit=${GIT_COMMIT} -s -w" \
    -trimpath \
    -o databridge \
    ./cmd/databridge

# Stage 2: Build the React frontend
FROM node:20-alpine AS frontend-builder

WORKDIR /frontend

# Copy package files first for better caching
COPY frontend/package*.json ./
RUN npm ci --only=production

# Copy frontend source
COPY frontend/ ./

# Build frontend for production
RUN npm run build

# Stage 3: Final runtime image
FROM alpine:3.22

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    && addgroup -g 1000 databridge \
    && adduser -D -u 1000 -G databridge databridge

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=backend-builder /build/databridge /app/databridge

# Copy frontend build from frontend-builder (if serving from Go)
COPY --from=frontend-builder /frontend/.next /app/frontend/.next
COPY --from=frontend-builder /frontend/public /app/frontend/public
COPY --from=frontend-builder /frontend/package.json /app/frontend/package.json

# Create data directories
RUN mkdir -p /app/data/flowfiles \
    /app/data/content \
    /app/data/provenance \
    /app/data/logs \
    && chown -R databridge:databridge /app

# Switch to non-root user
USER databridge

# Expose ports
# 8080: REST API
# 3000: Frontend (if running separately)
EXPOSE 8080 3000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD curl -f http://localhost:8080/api/health || exit 1

# Set environment variables
ENV DATA_DIR=/app/data \
    LOG_LEVEL=info \
    PORT=8080

# Default command
CMD ["/app/databridge", "--data-dir", "/app/data", "--api-port", "8080"]
