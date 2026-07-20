# API Examples

## Health checks

```http
GET /health/live
```

```json
{
  "status": "ok",
  "service": "platform-api",
  "version": "development",
  "commit": "unknown",
  "buildTime": "unknown"
}
```

## Create run

```http
POST /v1/projects/0190/runs
Authorization: Bearer <token>
Idempotency-Key: 8bc0...
Content-Type: application/json
```

```json
{
  "prompt": "Build a bookmark API with tests",
  "runtime": "python-3.12",
  "resources": {"cpuMillis": 1000, "memoryMiB": 2048},
  "timeoutSeconds": 1800,
  "maxAttempts": 3,
  "deployment": {"enabled": true, "environment": "dev"}
}
```

```json
{
  "data": {
    "id": "0191...",
    "status": "QUEUED",
    "createdAt": "2026-07-20T16:00:00Z"
  }
}
```

## Error envelope

```json
{
  "error": {
    "code": "TENANT_CONCURRENCY_LIMIT",
    "message": "The tenant has reached its concurrent run limit.",
    "requestId": "req_...",
    "retryable": true,
    "details": {"limit": 20}
  }
}
```

Platform API foundation errors use the same shape. For example, a request that
exceeds the configured body limit returns `413`:

```json
{
  "error": {
    "code": "REQUEST_TOO_LARGE",
    "message": "Request body exceeds the allowed size.",
    "requestId": "req_...",
    "retryable": false
  }
}
```
