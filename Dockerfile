# Build stage
FROM golang:1.21-alpine AS builder

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o u2b_download .

# Final stage
FROM python:3.11-alpine

# Install yt-dlp and other dependencies
RUN apk add --no-cache ca-certificates tzdata ffmpeg && \
    pip install --no-cache-dir yt-dlp

# Copy the binary from builder stage
COPY --from=builder /app/u2b_download /usr/local/bin/u2b_download

# Create non-root user
RUN addgroup -g 1001 appuser && \
    adduser -u 1001 -G appuser -s /bin/sh -D appuser

# Create directories
RUN mkdir -p /app/downloads /app/config && \
    chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Set working directory
WORKDIR /app

# Expose volume for downloads
VOLUME ["/app/downloads"]

# Set environment variables
ENV PATH="/usr/local/bin:${PATH}"

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD u2b_download --version || exit 1

# Default command
ENTRYPOINT ["u2b_download"]
CMD ["--help"]