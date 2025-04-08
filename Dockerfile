FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /go/bin/scheduler

# Final stage
FROM alpine:latest

# Install bash for shell commands
RUN apk --no-cache add bash

# Copy the binary from builder
COPY --from=builder /go/bin/scheduler /usr/local/bin/scheduler

# Create directory for static files
WORKDIR /app
COPY static/ /app/static/

# Expose port for API
EXPOSE 8080

# Set default command
ENTRYPOINT ["scheduler"]