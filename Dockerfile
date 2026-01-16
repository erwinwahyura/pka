# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the web application
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-linkmode external -extldflags "-static"' -o pka-web ./cmd/pka-web

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite-libs tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/pka-web .

# Create data directory
RUN mkdir -p /data

# Expose port
EXPOSE 8080

# Run as non-root user
RUN addgroup -g 1000 pka && \
    adduser -D -u 1000 -G pka pka && \
    chown -R pka:pka /app /data

USER pka

# Set default environment variables
ENV PORT=8080
ENV DB_PATH=/data/books.db
ENV OLLAMA_URL=http://ollama:11434
ENV OLLAMA_MODEL=nomic-embed-text

# Run the application
CMD ["sh", "-c", "./pka-web -port=$PORT -db=$DB_PATH -ollama-url=$OLLAMA_URL -ollama-model=$OLLAMA_MODEL"]
