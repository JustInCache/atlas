# Build stage — always runs on the host's native arch for speed
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

# Populated automatically by buildx
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk add --no-cache ca-certificates

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code
COPY . .

# Build the binary — cross-compile for the target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s" -o /app/bin/atlas ./cmd/atlas

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates, wget (ECS healthcheck), and AWS CLI (EKS kubeconfig exec auth)
RUN apk add --no-cache ca-certificates wget python3 py3-pip \
    && pip3 install --break-system-packages --no-cache-dir awscli

# Copy binary from builder
COPY --from=builder /app/bin/atlas /app/atlas

# Copy UI static files
COPY --from=builder /app/ui /app/ui

COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Create non-root user before copying kubeconfigs so we can set ownership
RUN adduser -D -u 1000 appuser \
  && mkdir -p /home/appuser/.kube /app/kubeconfigs \
  && chown appuser:appuser /app/kubeconfigs

COPY --chown=appuser:appuser kubeconfigs/ /app/kubeconfigs/
USER appuser

# Expose port
EXPOSE 8080

# Set default environment variables
ENV PORT=8080

ENTRYPOINT ["/app/entrypoint.sh"]
