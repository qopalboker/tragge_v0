# HTTP Security Headers Configuration

This document describes the security headers configuration for the Tragge trading platform.

## Overview

The platform implements defense-in-depth security headers at multiple layers:

1. **Nginx Gateway** - Primary security headers enforcement
2. **Go BFF Services** - Secondary security headers (defense-in-depth)
3. **CORS Middleware** - Cross-Origin Resource Sharing control

## Configuration Files

| File | Purpose |
|------|---------|
| `apps/gateway/nginx.conf` | Development nginx configuration |
| `apps/gateway/nginx.prod.conf` | Production nginx configuration (with HSTS) |
| `packages/validation/cors.go` | CORS middleware for Go services |
| `packages/validation/middleware.go` | Security headers middleware |
| `scripts/security/test-security-headers.sh` | Security headers test script |

## Security Headers

### Global Headers (All Responses)

| Header | Value | Purpose |
|--------|-------|---------|
| `X-Frame-Options` | `DENY` | Prevents clickjacking attacks |
| `X-Content-Type-Options` | `nosniff` | Prevents MIME type sniffing |
| `X-XSS-Protection` | `1; mode=block` | XSS filter (legacy browsers) |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Controls referrer information |
| `Permissions-Policy` | See below | Restricts browser features |
| `X-Request-ID` | UUID | Request tracing correlation |

### Permissions Policy

```
geolocation=()
microphone=()
camera=()
payment=()
usb=()
accelerometer=()
gyroscope=()
magnetometer=()
```

This policy disables potentially sensitive browser features that the trading platform doesn't require.

### Cross-Origin Headers

| Header | Value | Purpose |
|--------|-------|---------|
| `Cross-Origin-Opener-Policy` | `same-origin` | Isolates browsing context |
| `Cross-Origin-Embedder-Policy` | `require-corp` | Requires CORP for resources |
| `Cross-Origin-Resource-Policy` | `same-origin` | Restricts resource sharing |

### HSTS (Production Only)

```
Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
```

- **max-age=31536000**: 1 year duration
- **includeSubDomains**: Applies to all subdomains
- **preload**: Eligible for browser preload list

**Important**: HSTS is only enabled in `nginx.prod.conf`. Do not enable in development as it will break HTTP connections.

### Go BFF Middleware Headers (`packages/validation/middleware.go`)

The `SecurityHeadersMiddleware` in Go services sets these headers as defense-in-depth:

| Header | Value | Condition |
|--------|-------|-----------|
| `X-Content-Type-Options` | `nosniff` | Always |
| `X-XSS-Protection` | `1; mode=block` | Always |
| `X-Frame-Options` | `DENY` | Always |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Always |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` | Always |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | HTTPS only (`r.TLS != nil` or `X-Forwarded-Proto: https`) |
| `Cache-Control` | `no-store, no-cache, must-revalidate, private` | API requests (`/api/*`) |
| `Pragma` | `no-cache` | API requests (`/api/*`) |
| `Content-Security-Policy` | `default-src 'none'; frame-ancestors 'none'` | API requests (`/api/*`) |

**Note**: Nginx is the primary enforcement layer. These headers provide secondary defense in case a request bypasses the gateway.

## Content-Security-Policy (CSP)

CSP policies are customized per frontend to minimize attack surface while maintaining functionality.

### User Frontend (`/user/*`)

```
default-src 'self';
script-src 'self';
style-src 'self' 'unsafe-inline';
img-src 'self' data: https:;
font-src 'self' data:;
connect-src 'self' wss://$host ws://$host;
frame-ancestors 'none';
base-uri 'self';
form-action 'self';
upgrade-insecure-requests
```

### Trade Frontend (`/trade/*`)

```
default-src 'self';
script-src 'self';
style-src 'self' 'unsafe-inline';
img-src 'self' data: https:;
font-src 'self' data:;
connect-src 'self' wss://$host ws://$host https://$host;
frame-ancestors 'none';
base-uri 'self';
form-action 'self';
upgrade-insecure-requests
```

Note: Trade frontend allows `https://$host` in `connect-src` for API calls and WebSocket connections.

### Admin Frontend (`/admin/*`)

```
default-src 'self';
script-src 'self';
style-src 'self' 'unsafe-inline';
img-src 'self' data:;
font-src 'self' data:;
connect-src 'self' https://$host;
frame-ancestors 'none';
base-uri 'self';
form-action 'self';
upgrade-insecure-requests;
require-trusted-types-for 'script'
```

Note: Admin frontend is most restrictive with `require-trusted-types-for 'script'` for additional XSS protection.

### API Endpoints (`/api/*`)

```
default-src 'none';
frame-ancestors 'none'
```

Minimal CSP for API responses since they don't serve HTML.

### CSP Notes

- `'unsafe-inline'` for styles is required by Vue.js. Consider using nonce-based CSP in future.
- `'unsafe-eval'` has been removed for security.
- `frame-ancestors 'none'` prevents embedding in iframes.
- `upgrade-insecure-requests` automatically upgrades HTTP to HTTPS.

## CORS Configuration

### Development Origins

```go
AllowedOrigins: []string{
    "http://localhost:5173", // unified frontend Vite dev server
    "http://localhost:8080", // gateway
}
```

Ports 5174 and 5175 may still appear in legacy tolerance lists
(e.g. `infra/docker/docker-compose.yml` `ALLOWED_ORIGINS`,
`apps/gateway/nginx.conf` `$http_origin` map) left in place as a slow
rollout safety net. They are no longer issued by any app and can be
removed once every environment is confirmed on the consolidated SPA.

### Production Configuration

Set the `CORS_ALLOWED_ORIGINS` environment variable:

```bash
export CORS_ALLOWED_ORIGINS="https://app.tragge.io,https://trade.tragge.io,https://admin.tragge.io"
```

### CORS Headers

| Header | Description |
|--------|-------------|
| `Access-Control-Allow-Origin` | Reflects allowed origin |
| `Access-Control-Allow-Methods` | GET, POST, PUT, PATCH, DELETE, OPTIONS |
| `Access-Control-Allow-Headers` | Accept, Authorization, Content-Type, X-Request-ID, X-Contest-ID |
| `Access-Control-Expose-Headers` | X-Request-ID, X-RateLimit-*, Retry-After |
| `Access-Control-Allow-Credentials` | true |
| `Access-Control-Max-Age` | 86400 (24 hours) |

### Using CORS Middleware

```go
import "github.com/Parsaeffatravesh/tragge/packages/validation"

// In your service setup
corsConfig := validation.CORSConfigFromEnv()
r.Use(validation.CORSMiddleware(corsConfig))
```

For service-specific configurations:

```go
// User BFF
corsConfig := validation.UserBFFCORSConfig()

// Trade BFF (includes WebSocket headers)
corsConfig := validation.TradeBFFCORSConfig()

// Admin BFF (more restrictive)
corsConfig := validation.AdminBFFCORSConfig()
```

## Rate Limiting

### Rate Limit Zones

| Zone | Rate | Purpose |
|------|------|---------|
| `api_limit` | 100 req/min | General API endpoints |
| `auth_limit` | 10 req/min | Login/Register endpoints |
| `ws_limit` | 5 req/min | WebSocket connections |
| `conn_limit` | 20 concurrent | Connection limit per IP |

### Rate Limit Response Headers

```
X-RateLimit-Limit: 100
X-RateLimit-Policy: 100;w=60
```

### Rate Limit Exceeded Response

```json
{
    "error": "rate_limit_exceeded",
    "message": "Too many requests. Please try again later."
}
```

HTTP Status: `429 Too Many Requests`

## Request ID Correlation

### Header Flow

```
Client Request
    ↓
[X-Request-ID: <optional>]
    ↓
Nginx Gateway
    ↓
[X-Request-ID: <uuid> or <propagated>]
    ↓
BFF Service
    ↓
[logged with request_id]
    ↓
Response
    ↓
[X-Request-ID: <same-uuid>]
```

### Accessing Request ID in Go

```go
import "github.com/Parsaeffatravesh/tragge/packages/validation"

func handler(w http.ResponseWriter, r *http.Request) {
    requestID := validation.GetRequestID(r.Context())
    // Use for logging, error responses, etc.
}
```

## Development vs Production

| Feature | Development | Production |
|---------|-------------|------------|
| HSTS | Disabled | Enabled (31536000s) |
| CSP | Permissive | Hardened |
| CORS Origins | localhost:* | Explicit domains |
| SSL/TLS | Optional | Required |
| Logging | Standard | JSON (structured) |

## Testing

### Running Security Tests

```bash
# Development
./scripts/security/test-security-headers.sh

# Production mode (checks HSTS)
./scripts/security/test-security-headers.sh -p

# Custom URL
./scripts/security/test-security-headers.sh -u https://api.tragge.io -p

# Verbose output
./scripts/security/test-security-headers.sh -v
```

### Manual Testing with curl

```bash
# Check all headers
curl -I http://localhost:8080/health

# Check CORS preflight
curl -I -X OPTIONS \
    -H "Origin: http://localhost:5173" \
    -H "Access-Control-Request-Method: GET" \
    http://localhost:8080/api/user/healthz

# Check rate limit headers
curl -I http://localhost:8080/api/user/healthz
```

## Subresource Integrity (SRI)

For static assets served by the frontends, add SRI hashes to script and style tags:

```html
<script src="/assets/main.js"
        integrity="sha384-..."
        crossorigin="anonymous"></script>
```

Generate SRI hashes using:

```bash
# Generate SHA-384 hash
shasum -a 384 file.js | xxd -r -p | base64
```

Or use the build tool integration in Vite:

```typescript
// vite.config.ts
export default defineConfig({
  build: {
    manifest: true,
    // SRI will be added via manifest
  }
})
```

## Security Checklist

### Before Production Deployment

- [ ] HSTS enabled in nginx.prod.conf
- [ ] SSL/TLS certificates configured
- [ ] CORS_ALLOWED_ORIGINS set to production domains
- [ ] CSP policies reviewed for each frontend
- [ ] Rate limits configured appropriately
- [ ] Request ID correlation verified end-to-end
- [ ] Security headers test passes with `-p` flag
- [ ] Server header hidden (server_tokens off)

### Regular Audits

- [ ] Review CSP violation reports
- [ ] Monitor rate limit hits
- [ ] Check for new security header recommendations
- [ ] Update SSL/TLS configuration as needed
- [ ] Review CORS allowed origins

## References

- [MDN HTTP Headers](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers)
- [OWASP Secure Headers Project](https://owasp.org/www-project-secure-headers/)
- [Content Security Policy Level 3](https://www.w3.org/TR/CSP3/)
- [HSTS Preload List](https://hstspreload.org/)
