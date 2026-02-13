# Stage 1: Build
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o codeforge ./cmd/codeforge

# Stage 2: Runtime
FROM alpine:3.20
ARG CLAUDE_CODE_VERSION=1.0.88
RUN apk add --no-cache git nodejs npm ca-certificates tzdata wget coreutils \
    && npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION} \
    && adduser -D -h /home/codeforge codeforge \
    && mkdir -p /data/workspaces \
    && chown codeforge:codeforge /data/workspaces

USER codeforge
WORKDIR /home/codeforge
COPY --from=builder /build/codeforge /usr/local/bin/

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["codeforge"]
