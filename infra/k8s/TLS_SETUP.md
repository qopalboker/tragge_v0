# TLS/HTTPS Setup Guide for Tragge Platform

This guide explains how to set up and verify TLS/HTTPS for the Tragge trading platform using cert-manager and Let's Encrypt.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Architecture Overview](#architecture-overview)
- [Installation Steps](#installation-steps)
- [Configuration](#configuration)
- [Verification](#verification)
- [Troubleshooting](#troubleshooting)
- [Maintenance](#maintenance)

## Prerequisites

### 1. Install cert-manager

cert-manager must be installed in your Kubernetes cluster before deploying the Tragge TLS configuration.

```bash
# Install cert-manager using kubectl
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.3/cert-manager.yaml

# Verify cert-manager is running
kubectl get pods -n cert-manager

# Expected output:
# NAME                                       READY   STATUS    RESTARTS   AGE
# cert-manager-xxxxxxxxx-xxxxx               1/1     Running   0          1m
# cert-manager-cainjector-xxxxxxxxx-xxxxx    1/1     Running   0          1m
# cert-manager-webhook-xxxxxxxxx-xxxxx       1/1     Running   0          1m
```

### 2. Domain Configuration

Ensure your domain DNS is properly configured:

```bash
# Main domain
tragge.example.com          A/CNAME    <your-load-balancer-ip>

# API subdomain
api.tragge.example.com      A/CNAME    <your-load-balancer-ip>

# WebSocket subdomain
ws.tragge.example.com       A/CNAME    <your-load-balancer-ip>
```

**IMPORTANT:** DNS records must be properly configured BEFORE requesting certificates from Let's Encrypt. The HTTP-01 challenge requires the domain to resolve to your cluster's ingress controller.

### 3. nginx-ingress-controller

The nginx-ingress-controller must be installed with the `ingress-nginx/controller-v1.9.0` or later.

```bash
# Verify nginx-ingress is running
kubectl get pods -n ingress-nginx

# Check ingress class exists
kubectl get ingressclass
```

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Let's Encrypt                             │
│                    (ACME Certificate Authority)                  │
└────────────────┬────────────────────────────────────────────────┘
                 │ ACME HTTP-01 Challenge
                 │
┌────────────────▼────────────────────────────────────────────────┐
│                        cert-manager                              │
│  ┌──────────────────┐        ┌─────────────────┐               │
│  │  ClusterIssuers  │        │  Certificate    │               │
│  │  - staging       │───────▶│  - tragge-tls   │               │
│  │  - production    │        └─────────┬───────┘               │
│  └──────────────────┘                  │                        │
└────────────────────────────────────────┼────────────────────────┘
                                         │ Creates/Updates
                                         ▼
                              ┌─────────────────────┐
                              │   Kubernetes Secret │
                              │  tragge-tls-secret  │
                              │  - tls.crt          │
                              │  - tls.key          │
                              └──────────┬──────────┘
                                         │ Referenced by
                                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Ingress Resources                             │
│  ┌────────────────────┐      ┌──────────────────────────┐      │
│  │ tragge-ingress     │      │ tragge-websocket-ingress │      │
│  │ - Main domain      │      │ - WebSocket domain       │      │
│  │ - API subdomain    │      │ - Session affinity       │      │
│  └────────────────────┘      └──────────────────────────┘      │
└─────────────────────────────────────────────────────────────────┘
                                         │
                                         ▼
                              ┌─────────────────────┐
                              │  Backend Services   │
                              │  - user-bff         │
                              │  - trade-bff        │
                              │  - admin-bff        │
                              │  - frontends        │
                              └─────────────────────┘
```

## Installation Steps

### Step 1: Configure Domain Name

Update the domain name in the following files using kustomize or direct editing:

**Option A: Using kustomize patches (Recommended)**

Create a kustomization overlay for your environment:

```yaml
# infra/k8s/overlays/production/domain-patch.yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: tragge-tls
  namespace: tragge
spec:
  dnsNames:
    - your-domain.com
    - api.your-domain.com
    - ws.your-domain.com
  commonName: your-domain.com

---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: tragge-ingress
  namespace: tragge
spec:
  tls:
    - hosts:
        - your-domain.com
        - api.your-domain.com
        - ws.your-domain.com
      secretName: tragge-tls-secret
  rules:
    - host: your-domain.com
      # ... (rest of the rules)
```

**Option B: Direct editing**

Edit the following files and replace `tragge.example.com` with your actual domain:

1. `infra/k8s/base/certificate.yaml` - Update `dnsNames` and `commonName`
2. `infra/k8s/base/ingress.yaml` - Update `hosts` in TLS and `rules` sections
3. `infra/k8s/base/cluster-issuer.yaml` - Update `email` field

### Step 2: Test with Staging Issuer (Recommended)

Before using production certificates, test with the staging issuer to avoid rate limits:

```bash
# Edit certificate.yaml to use staging issuer
# Change: issuerRef.name from "letsencrypt-prod" to "letsencrypt-staging"

# Apply the configuration
kubectl apply -k infra/k8s/base

# Watch certificate creation
kubectl get certificate -n tragge -w

# Check certificate status
kubectl describe certificate tragge-tls -n tragge
```

If the staging certificate is issued successfully, proceed to production.

### Step 3: Switch to Production Issuer

```bash
# Edit certificate.yaml
# Change: issuerRef.name from "letsencrypt-staging" to "letsencrypt-prod"

# Delete the old certificate to force re-issuance
kubectl delete certificate tragge-tls -n tragge
kubectl delete secret tragge-tls-secret -n tragge

# Apply the configuration
kubectl apply -k infra/k8s/base
```

### Step 4: Deploy the Complete Configuration

```bash
# Apply all Kubernetes manifests including TLS configuration
kubectl apply -k infra/k8s/base

# Or for production overlay
kubectl apply -k infra/k8s/overlays/production
```

## Configuration

### Email Notification

Update the email address in `cluster-issuer.yaml` to receive certificate expiry notifications:

```yaml
spec:
  acme:
    email: your-email@example.com  # Change this!
```

### Certificate Renewal

Certificates are automatically renewed by cert-manager:

- **Duration:** 90 days (Let's Encrypt standard)
- **Renewal Window:** 15 days before expiry
- **Automatic:** cert-manager handles renewal automatically

### Security Headers

The following security headers are configured in `ingress.yaml`:

| Header | Value | Purpose |
|--------|-------|---------|
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains; preload` | Force HTTPS for 1 year |
| `X-Content-Type-Options` | `nosniff` | Prevent MIME type sniffing |
| `X-Frame-Options` | `SAMEORIGIN` | Prevent clickjacking |
| `X-XSS-Protection` | `1; mode=block` | Enable XSS protection |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Control referrer information |
| `Permissions-Policy` | `geolocation=(), microphone=(), camera=()` | Disable sensitive features |

## Verification

### 1. Check ClusterIssuers

```bash
# List all cluster issuers
kubectl get clusterissuer

# Expected output:
# NAME                  READY   AGE
# letsencrypt-staging   True    5m
# letsencrypt-prod      True    5m

# Describe the production issuer
kubectl describe clusterissuer letsencrypt-prod

# Look for:
#   Status.Conditions.Type: Ready
#   Status.Conditions.Status: True
```

### 2. Check Certificate Status

```bash
# List certificates in tragge namespace
kubectl get certificate -n tragge

# Expected output:
# NAME          READY   SECRET              AGE
# tragge-tls    True    tragge-tls-secret   5m

# Describe the certificate
kubectl describe certificate tragge-tls -n tragge

# Look for:
#   Status.Conditions.Type: Ready
#   Status.Conditions.Status: True
#   Status.Conditions.Reason: Ready
#   Events: Certificate issued successfully
```

### 3. Check Certificate Secret

```bash
# Verify the secret was created
kubectl get secret tragge-tls-secret -n tragge

# View certificate details
kubectl get secret tragge-tls-secret -n tragge -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -text -noout

# Check certificate expiration
kubectl get secret tragge-tls-secret -n tragge -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -enddate -noout

# Expected output:
# notAfter=<date 90 days from now>
```

### 4. Check Ingress Configuration

```bash
# List ingresses
kubectl get ingress -n tragge

# Describe the main ingress
kubectl describe ingress tragge-ingress -n tragge

# Check TLS configuration
kubectl get ingress tragge-ingress -n tragge -o yaml | grep -A 10 tls:
```

### 5. Test HTTPS Connection

```bash
# Test SSL certificate
curl -vI https://tragge.example.com

# Look for:
#   SSL certificate verify ok
#   Server certificate:
#     subject: CN=tragge.example.com
#     issuer: C=US, O=Let's Encrypt, CN=R3

# Test SSL Labs (for detailed SSL analysis)
# Visit: https://www.ssllabs.com/ssltest/analyze.html?d=tragge.example.com
```

### 6. Test HTTP to HTTPS Redirect

```bash
# Test redirect from HTTP to HTTPS
curl -I http://tragge.example.com

# Expected response:
# HTTP/1.1 308 Permanent Redirect
# Location: https://tragge.example.com/
```

### 7. Test WebSocket over TLS

```bash
# Test WebSocket connection (requires wscat)
wscat -c wss://ws.tragge.example.com/ws/trade

# Should establish secure WebSocket connection
```

### 8. Verify Security Headers

```bash
# Check security headers
curl -I https://tragge.example.com

# Expected headers:
# Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
# X-Content-Type-Options: nosniff
# X-Frame-Options: SAMEORIGIN
# X-XSS-Protection: 1; mode=block
```

## Troubleshooting

### Issue 1: Certificate Not Ready

**Symptom:**
```bash
kubectl get certificate -n tragge
# NAME          READY   SECRET              AGE
# tragge-tls    False   tragge-tls-secret   5m
```

**Diagnosis:**
```bash
# Check certificate events
kubectl describe certificate tragge-tls -n tragge

# Check certificate request
kubectl get certificaterequest -n tragge
kubectl describe certificaterequest -n tragge

# Check ACME challenge
kubectl get challenge -n tragge
kubectl describe challenge -n tragge

# Check cert-manager logs
kubectl logs -n cert-manager -l app=cert-manager -f
```

**Common Causes:**

1. **DNS not configured correctly**
   ```bash
   # Verify DNS resolution
   nslookup tragge.example.com
   dig tragge.example.com
   ```
   **Solution:** Ensure DNS A/CNAME records point to your ingress controller's load balancer IP.

2. **HTTP-01 challenge failing**
   ```bash
   # Check if challenge endpoint is accessible
   curl http://tragge.example.com/.well-known/acme-challenge/test
   ```
   **Solution:**
   - Ensure ingress controller is running
   - Ensure port 80 is accessible from the internet
   - Check firewall rules

3. **Rate limit hit**
   ```bash
   # Check certificate request for rate limit errors
   kubectl describe certificaterequest -n tragge | grep -i "rate limit"
   ```
   **Solution:**
   - Wait for rate limit to reset (1 week for production)
   - Use staging issuer for testing

### Issue 2: Certificate Issued but Not Trusted

**Symptom:** Browser shows "Not Secure" or certificate warnings

**Diagnosis:**
```bash
# Check which issuer was used
kubectl get certificate tragge-tls -n tragge -o yaml | grep issuerRef -A 3
```

**Solution:**
- If using `letsencrypt-staging`, switch to `letsencrypt-prod`
- Staging certificates are NOT trusted by browsers (this is expected for testing)

### Issue 3: HTTPS Not Working

**Symptom:** Cannot access site via HTTPS

**Diagnosis:**
```bash
# Check ingress TLS configuration
kubectl get ingress tragge-ingress -n tragge -o yaml | grep -A 10 tls:

# Check if secret exists
kubectl get secret tragge-tls-secret -n tragge

# Check nginx-ingress logs
kubectl logs -n ingress-nginx -l app.kubernetes.io/component=controller -f
```

**Common Causes:**

1. **Secret not found**
   ```bash
   kubectl get events -n tragge | grep -i secret
   ```
   **Solution:** Ensure certificate is ready and secret is created

2. **Ingress not referencing secret**
   **Solution:** Check `spec.tls.secretName` matches `tragge-tls-secret`

3. **Ingress controller not configured for TLS**
   **Solution:** Ensure nginx-ingress-controller is properly installed

### Issue 4: WebSocket Not Working Over TLS

**Symptom:** WebSocket connections fail with wss://

**Diagnosis:**
```bash
# Check WebSocket ingress
kubectl describe ingress tragge-websocket-ingress -n tragge

# Check for WebSocket upgrade headers
kubectl get ingress tragge-websocket-ingress -n tragge -o yaml | grep -i upgrade
```

**Solution:**
- Ensure `nginx.ingress.kubernetes.io/proxy-http-version: "1.1"` annotation is present
- Ensure configuration snippet includes upgrade headers
- Check that TLS is configured for ws.tragge.example.com

### Issue 5: Mixed Content Warnings

**Symptom:** Browser console shows mixed content warnings

**Diagnosis:**
- Check if any resources are loaded over HTTP instead of HTTPS

**Solution:**
- Update all resource URLs to use HTTPS or relative paths
- Ensure `nginx.ingress.kubernetes.io/force-ssl-redirect: "true"` is set
- Check `X-Forwarded-Proto` header is passed correctly

### Issue 6: Certificate Renewal Failing

**Symptom:** Certificate not renewing before expiry

**Diagnosis:**
```bash
# Check certificate status
kubectl describe certificate tragge-tls -n tragge

# Check cert-manager logs during renewal
kubectl logs -n cert-manager -l app=cert-manager --since=1h | grep tragge-tls
```

**Solution:**
- Ensure cert-manager is running
- Check HTTP-01 challenge is still accessible
- Verify DNS still points to correct IP

## Maintenance

### Certificate Lifecycle

1. **Initial Issuance:**
   - cert-manager creates a CertificateRequest
   - ACME challenge is performed (HTTP-01)
   - Certificate is issued and stored in Secret

2. **Automatic Renewal:**
   - cert-manager checks certificates daily
   - Renewal starts 15 days before expiry
   - New certificate seamlessly replaces old one

3. **Manual Renewal (if needed):**
   ```bash
   # Force certificate renewal
   kubectl delete certificaterequest -n tragge --all
   kubectl delete secret tragge-tls-secret -n tragge

   # cert-manager will automatically recreate
   kubectl get certificate -n tragge -w
   ```

### Monitoring Certificate Expiry

```bash
# Create a monitoring script
cat > check-cert-expiry.sh << 'EOF'
#!/bin/bash
CERT=$(kubectl get secret tragge-tls-secret -n tragge -o jsonpath='{.data.tls\.crt}' | base64 -d)
EXPIRY=$(echo "$CERT" | openssl x509 -enddate -noout | cut -d= -f2)
DAYS_UNTIL_EXPIRY=$(( ($(date -d "$EXPIRY" +%s) - $(date +%s)) / 86400 ))

echo "Certificate expires: $EXPIRY"
echo "Days until expiry: $DAYS_UNTIL_EXPIRY"

if [ $DAYS_UNTIL_EXPIRY -lt 30 ]; then
    echo "WARNING: Certificate expires in less than 30 days!"
    exit 1
fi
EOF

chmod +x check-cert-expiry.sh
./check-cert-expiry.sh
```

### Updating Domain Names

To add or remove domains from the certificate:

1. Edit `certificate.yaml`:
   ```yaml
   spec:
     dnsNames:
       - tragge.example.com
       - api.tragge.example.com
       - ws.tragge.example.com
       - new-subdomain.tragge.example.com  # Add new domain
   ```

2. Apply changes:
   ```bash
   kubectl apply -f infra/k8s/base/certificate.yaml

   # cert-manager will automatically request a new certificate
   kubectl get certificate -n tragge -w
   ```

3. Update ingress.yaml to use the new domain in routes

### Switching Between Staging and Production

**Switch to Staging (for testing):**
```bash
# Edit certificate.yaml
kubectl edit certificate tragge-tls -n tragge
# Change issuerRef.name to "letsencrypt-staging"

# Delete and recreate
kubectl delete secret tragge-tls-secret -n tragge
```

**Switch to Production:**
```bash
# Edit certificate.yaml
kubectl edit certificate tragge-tls -n tragge
# Change issuerRef.name to "letsencrypt-prod"

# Delete and recreate
kubectl delete secret tragge-tls-secret -n tragge
```

### Backup Certificate

```bash
# Backup the certificate and private key
kubectl get secret tragge-tls-secret -n tragge -o yaml > tragge-tls-backup.yaml

# Store securely (encrypted)
gpg --encrypt --recipient your-email@example.com tragge-tls-backup.yaml

# Restore if needed
kubectl apply -f tragge-tls-backup.yaml
```

## Security Best Practices

1. **Use Strong TLS Configuration:**
   - The ingress is configured to use modern TLS protocols (TLS 1.2+)
   - nginx-ingress-controller handles cipher suite selection

2. **Enable HSTS Preload:**
   - After verifying HTTPS works correctly, submit your domain to HSTS preload list
   - Visit: https://hstspreload.org/

3. **Regular Security Audits:**
   - Run SSL Labs test monthly: https://www.ssllabs.com/ssltest/
   - Target: A+ rating

4. **Monitor Certificate Expiry:**
   - Set up alerts for certificates expiring in < 30 days
   - cert-manager should auto-renew, but monitor for failures

5. **Rotate Secrets:**
   - Private keys are automatically rotated on renewal (rotationPolicy: Always)
   - Consider rotating earlier if security incident occurs

6. **Review Security Headers:**
   - Periodically review and update security headers
   - Use https://securityheaders.com/ for analysis

## Additional Resources

- **cert-manager Documentation:** https://cert-manager.io/docs/
- **Let's Encrypt Documentation:** https://letsencrypt.org/docs/
- **nginx-ingress Annotations:** https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/annotations/
- **SSL Labs Testing:** https://www.ssllabs.com/ssltest/
- **HSTS Preload:** https://hstspreload.org/
- **Security Headers:** https://securityheaders.com/

## Support

If you encounter issues not covered in this guide:

1. Check cert-manager logs: `kubectl logs -n cert-manager -l app=cert-manager`
2. Check ingress-nginx logs: `kubectl logs -n ingress-nginx -l app.kubernetes.io/component=controller`
3. Review cert-manager troubleshooting: https://cert-manager.io/docs/troubleshooting/
4. Check Let's Encrypt status: https://letsencrypt.status.io/

---

**Last Updated:** January 2026
**Version:** 1.0.0
