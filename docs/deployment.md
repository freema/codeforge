# Deployment Guide

## Docker

### Build

```bash
docker build -f deployments/Dockerfile -t codeforge:latest .
```

### Run

```bash
docker run -d \
  -p 8080:8080 \
  -e CODEFORGE_REDIS__URL=redis://your-redis:6379 \
  -e CODEFORGE_SERVER__AUTH_TOKEN=your-secure-token \
  -e CODEFORGE_ENCRYPTION__KEY=$(openssl rand -base64 32) \
  -e CODEFORGE_WEBHOOKS__HMAC_SECRET=your-webhook-secret \
  -v /data/workspaces:/data/workspaces \
  codeforge:latest
```

### Docker Compose (Production)

```yaml
services:
  app:
    image: codeforge:latest
    ports:
      - "8080:8080"
    environment:
      CODEFORGE_REDIS__URL: redis://redis:6379
      CODEFORGE_SERVER__AUTH_TOKEN: ${AUTH_TOKEN}
      CODEFORGE_ENCRYPTION__KEY: ${ENCRYPTION_KEY}
      CODEFORGE_WEBHOOKS__HMAC_SECRET: ${WEBHOOK_SECRET}
      CODEFORGE_WORKERS__CONCURRENCY: "5"
      CODEFORGE_LOGGING__LEVEL: info
      CODEFORGE_LOGGING__FORMAT: json
      CODEFORGE_TRACING__ENABLED: "true"
      CODEFORGE_TRACING__ENDPOINT: otel-collector:4318
    volumes:
      - workspaces:/data/workspaces
    depends_on:
      redis:
        condition: service_healthy

  redis:
    image: redis:7-alpine
    command: redis-server --requirepass ${REDIS_PASSWORD}
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD}", "ping"]
      interval: 5s
      timeout: 3s
      retries: 3

volumes:
  workspaces:
  redis-data:
```

## Kubernetes

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: codeforge
spec:
  replicas: 1
  selector:
    matchLabels:
      app: codeforge
  template:
    metadata:
      labels:
        app: codeforge
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      containers:
        - name: codeforge
          image: codeforge:latest
          ports:
            - containerPort: 8080
          env:
            - name: CODEFORGE_REDIS__URL
              valueFrom:
                secretKeyRef:
                  name: codeforge-secrets
                  key: redis-url
            - name: CODEFORGE_SERVER__AUTH_TOKEN
              valueFrom:
                secretKeyRef:
                  name: codeforge-secrets
                  key: auth-token
            - name: CODEFORGE_ENCRYPTION__KEY
              valueFrom:
                secretKeyRef:
                  name: codeforge-secrets
                  key: encryption-key
          volumeMounts:
            - name: workspaces
              mountPath: /data/workspaces
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /ready
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              cpu: 500m
              memory: 512Mi
            limits:
              cpu: 2000m
              memory: 2Gi
      volumes:
        - name: workspaces
          persistentVolumeClaim:
            claimName: codeforge-workspaces
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: codeforge
spec:
  selector:
    app: codeforge
  ports:
    - port: 8080
      targetPort: 8080
```

## Security Considerations

- **Auth token**: Use a strong, random token (32+ characters)
- **Encryption key**: Must be exactly 32 bytes, base64-encoded
- **Redis**: Use password auth and TLS in production
- **Network**: Keep Redis and CodeForge on a private network
- **Workspaces**: The workspace volume contains cloned repositories; restrict access
- **Non-root**: The Docker image runs as the `codeforge` user (non-root)
- **Webhook secrets**: Use a strong HMAC secret for callback verification

## Monitoring

### Prometheus

Scrape the `/metrics` endpoint. Key metrics to alert on:

- `codeforge_tasks_in_progress` > worker count (queue backing up)
- `codeforge_queue_depth` > threshold (tasks waiting)
- `codeforge_workspace_disk_usage_bytes` approaching disk limits
- `codeforge_http_requests_total{status="500"}` increasing (errors)

### Tracing

Enable OpenTelemetry tracing with an OTLP-compatible collector (Jaeger, Grafana Tempo, etc.):

```bash
CODEFORGE_TRACING__ENABLED=true
CODEFORGE_TRACING__ENDPOINT=your-collector:4318
CODEFORGE_TRACING__SAMPLING_RATE=0.1
```
