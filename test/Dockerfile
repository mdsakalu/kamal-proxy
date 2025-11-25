FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o kamal-proxy ./cmd/kamal-proxy

FROM alpine:latest

RUN apk --no-cache add ca-certificates curl

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /build/kamal-proxy /usr/local/bin/kamal-proxy

# Make it executable
RUN chmod +x /usr/local/bin/kamal-proxy

# Default command
CMD ["kamal-proxy", "run"]
