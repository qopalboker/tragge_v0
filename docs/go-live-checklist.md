# Tragge Platform Go-Live Checklist

This checklist ensures all critical systems, security measures, and operational procedures are in place before launching the Tragge trading tournament platform to production.

**Platform Components:**
- **Services**: user-bff, trade-bff, admin-bff, trading-engine, market-ingestor, leaderboard-worker
- **Frontends**: frontend, frontend, frontend (Vue 3)
- **Infrastructure**: Kubernetes, PostgreSQL, Redis, Redpanda (Kafka)
- **External APIs**: TwelveData, Massive

---

## 1. T-14 Days: Infrastructure Readiness

### Kubernetes Cluster
- [ ] Kubernetes cluster provisioned with adequate resources
  - [ ] Minimum 3 worker nodes for high availability
  - [ ] Node auto-scaling configured (if applicable)
  - [ ] Pod security policies applied
- [ ] Network policies configured
- [ ] Storage classes configured and tested
  - [ ] PostgreSQL persistent volumes
  - [ ] Redis persistent volumes
  - [ ] Redpanda persistent volumes
- [ ] Ingress controller deployed (NGINX)
- [ ] Namespace `tragge` created and configured

### DNS & Networking
- [ ] DNS records created for all domains
  - [ ] Main user application domain
  - [ ] Trading application domain
  - [ ] Admin application domain
  - [ ] API endpoints
  - [ ] Monitoring dashboards (Grafana, Prometheus)
- [ ] DNS propagation verified (use `nslookup` or `dig`)
- [ ] Load balancer configured and tested
- [ ] Static IP addresses allocated

### SSL/TLS & Security
- [ ] SSL certificates issued (Let's Encrypt via cert-manager)
- [ ] Certificate auto-renewal configured
- [ ] Certificate expiry alerts configured
- [ ] All endpoints force HTTPS redirect
- [ ] TLS 1.2+ minimum enforced
- [ ] CDN configured (CloudFlare/Fastly) if applicable
- [ ] DDoS protection enabled
- [ ] WAF rules configured (if applicable)

### Storage & Backups
- [ ] S3/GCS bucket created for backups
- [ ] Backup IAM roles/service accounts configured
- [ ] Backup encryption enabled
- [ ] Backup retention policy set (30 days recommended)
- [ ] Test restore performed successfully
  - [ ] PostgreSQL backup/restore tested
  - [ ] Redis backup/restore tested

### Secrets Management
- [ ] External Secrets Operator deployed
- [ ] Secret store backend configured (AWS Secrets Manager/Vault)
- [ ] All application secrets migrated from development
  - [ ] JWT_SECRET rotated
  - [ ] Database passwords rotated
  - [ ] Redis password rotated
  - [ ] TwelveData API key (production)
  - [ ] Massive API key (production)
  - [ ] NOWPayments API key (production)
  - [ ] NOWPayments IPN secret (webhook signature verification)
  - [ ] Jibit API and secret keys (production)
- [ ] Secret rotation policy established

### Container Registry
- [ ] Container registry access configured
- [ ] Image pull secrets created
- [ ] All Docker images tagged with production versions
- [ ] Image vulnerability scanning enabled

---

## 2. T-7 Days: Application Readiness

### Docker Images
- [ ] All services built with production configuration
  - [ ] user-bff
  - [ ] trade-bff
  - [ ] admin-bff
  - [ ] trading-engine
  - [ ] market-ingestor
  - [ ] leaderboard-worker
  - [ ] frontend
  - [ ] frontend
  - [ ] frontend
  - [ ] gateway (NGINX)
- [ ] All images pushed to container registry
- [ ] Image tags follow semantic versioning

### Kubernetes Deployment
- [ ] All manifests applied successfully
  ```bash
  kubectl apply -k infra/k8s/overlays/production
  ```
- [ ] All pods running and healthy
  ```bash
  kubectl get pods -n tragge
  ```
- [ ] No pods in CrashLoopBackOff or Error state
- [ ] All services have endpoints
  ```bash
  kubectl get endpoints -n tragge
  ```
- [ ] Resource limits configured for all deployments
  - [ ] CPU limits
  - [ ] Memory limits
- [ ] Health probes configured
  - [ ] Liveness probes
  - [ ] Readiness probes
  - [ ] Startup probes (if needed)

### Database
- [ ] PostgreSQL 16 deployed and running
- [ ] Database connection pooling configured (PgBouncer)
- [ ] All migrations applied
  ```bash
  make migrate-up
  ```
- [ ] Migration version verified
  ```bash
  make migrate-version
  ```
- [ ] Database indexes created and optimized
- [ ] Default roles created (user, admin, moderator)
- [ ] Connection limits tested

### Redis
- [ ] Redis 7 deployed and running
- [ ] Redis persistence enabled (AOF or RDB)
- [ ] Redis maxmemory policy configured
- [ ] Redis connection pooling tested
- [ ] Session storage tested
- [ ] Leaderboard sorted sets tested

### Redpanda (Kafka)
- [ ] Redpanda cluster deployed (minimum 3 brokers)
- [ ] All topics created with correct partitions
  - [ ] `ticks.v1`
  - [ ] `orders.v1`
  - [ ] `fills.v1`
  - [ ] `positions.v1`
  - [ ] `order_acks.v1`
  - [ ] `pnl.v1`
  - [ ] `contests.v1`
- [ ] Topic retention policies configured
- [ ] Consumer groups created
- [ ] Redpanda Console accessible for debugging

### Service Verification
- [ ] All BFFs responding to health checks
  - [ ] `curl http://user-bff:8081/healthz`
  - [ ] `curl http://trade-bff:8082/healthz`
  - [ ] `curl http://admin-bff:8083/healthz`
- [ ] Trading engine processing orders
- [ ] Market ingestor receiving price feeds
- [ ] Leaderboard worker updating scores
- [ ] Gateway routing requests correctly

---

## 3. T-7 Days: Security Checklist

### TLS/HTTPS
- [ ] TLS enabled on all public endpoints
- [ ] HTTPS redirect working (HTTP → HTTPS)
- [ ] TLS certificate chain valid
- [ ] Verify with SSL Labs: https://www.ssllabs.com/ssltest/
  - [ ] Grade A or higher
- [ ] HSTS header configured
- [ ] Certificate expiry monitoring enabled

### HTTP Security Headers
- [ ] Security headers configured in gateway
  - [ ] `Strict-Transport-Security`
  - [ ] `X-Content-Type-Options: nosniff`
  - [ ] `X-Frame-Options: DENY`
  - [ ] `X-XSS-Protection: 1; mode=block`
  - [ ] `Content-Security-Policy`
  - [ ] `Referrer-Policy: strict-origin-when-cross-origin`
- [ ] Verify with: https://securityheaders.com

### Rate Limiting
- [ ] Rate limiting configured in gateway
  - [ ] API endpoints: 100 req/min per IP
  - [ ] Login endpoint: 5 attempts per 15 min
  - [ ] Registration endpoint: 3 req/hour per IP
- [ ] Rate limiting tested and verified
- [ ] Rate limit headers returned (`X-RateLimit-*`)

### CORS Configuration
- [ ] CORS configured for all BFFs
- [ ] Allowed origins whitelisted (no wildcards)
- [ ] Credentials allowed only for trusted origins
- [ ] Preflight requests handled correctly

### Authentication & Authorization
- [ ] JWT secrets rotated from development
- [ ] JWT token expiry configured
  - [ ] Access token: 15 minutes
  - [ ] Refresh token: 7 days
- [ ] Password hashing verified (Argon2id)
- [ ] Role-based access control tested
  - [ ] Admin-only endpoints protected
  - [ ] User endpoints require authentication
- [ ] Session management working (Redis)

### API Keys & Credentials
- [ ] All production API keys configured
  - [ ] TwelveData API key (production tier)
  - [ ] Massive API key (production tier)
  - [ ] NOWPayments API key and IPN secret configured
  - [ ] NOWPayments webhook URL registered as `https://yourdomain.com/webhooks/nowpayments`
  - [ ] Jibit API and secret keys configured
  - [ ] Jibit callback URL and source allowlist configured
- [ ] Database credentials rotated
- [ ] Redis password set and rotated
- [ ] Secrets not exposed in logs
- [ ] Secrets not committed to git
- [ ] API key usage limits monitored

### Network Security
- [ ] Network policies applied
  - [ ] Restrict pod-to-pod communication
  - [ ] Only necessary services exposed
- [ ] Service mesh configured (optional: Istio/Linkerd)
- [ ] Private subnets for databases
- [ ] Bastion host for SSH access (if needed)

### RBAC (Kubernetes)
- [ ] Kubernetes RBAC roles defined
- [ ] Service accounts configured with least privilege
- [ ] Admin access restricted to necessary personnel
- [ ] Audit logging enabled for cluster access

### Vulnerability Management
- [ ] Container image scanning completed
- [ ] No critical vulnerabilities in images
- [ ] Dependency scanning completed (Go modules, npm)
- [ ] Security patches applied to base images

---

## 4. T-3 Days: Monitoring Readiness

### Prometheus
- [ ] Prometheus deployed and running
- [ ] All services being scraped
  - [ ] user-bff metrics endpoint
  - [ ] trade-bff metrics endpoint
  - [ ] admin-bff metrics endpoint
  - [ ] trading-engine metrics endpoint
  - [ ] market-ingestor metrics endpoint
  - [ ] leaderboard-worker metrics endpoint
- [ ] Service discovery working
- [ ] Retention period configured (15 days minimum)
- [ ] Prometheus storage sufficient

### Grafana
- [ ] Grafana deployed and accessible
- [ ] Datasources configured
  - [ ] Prometheus
  - [ ] Loki (logs)
  - [ ] Tempo (traces)
- [ ] All dashboards loading correctly
  - [ ] System Overview dashboard
  - [ ] WebSocket Real-time dashboard
  - [ ] Kafka/Redpanda Health dashboard
- [ ] Dashboard permissions configured
- [ ] Anonymous access disabled
- [ ] Admin password changed from default

### Alerting
- [ ] Alertmanager deployed and configured
- [ ] Alert routing rules configured
  - [ ] Critical alerts → PagerDuty + Slack
  - [ ] Warning alerts → Slack
- [ ] Notification channels tested
  - [ ] Slack webhook working
  - [ ] PagerDuty integration working
  - [ ] Email alerts working
- [ ] Alert rules configured
  - [ ] High error rate (>5%)
  - [ ] Pod restarts (>3 in 10 min)
  - [ ] High memory usage (>85%)
  - [ ] High CPU usage (>80%)
  - [ ] Database connection pool exhaustion
  - [ ] Kafka consumer lag (>1000 messages)
  - [ ] WebSocket connection failures
  - [ ] Market data feed down
  - [ ] Disk usage high (>80%)
- [ ] Test alerts fired successfully
  ```bash
  # Fire test alert
  curl -H "Content-Type: application/json" -d '[{"labels":{"alertname":"TestAlert"}}]' http://alertmanager:9093/api/v1/alerts
  ```

### On-Call Setup
- [ ] On-call rotation configured in PagerDuty
- [ ] Primary on-call engineer identified
- [ ] Secondary on-call engineer identified
- [ ] Escalation policy configured
- [ ] On-call team has reviewed runbooks
  - [ ] `docs/runbook/incident-response.md`
  - [ ] `docs/runbook/database-recovery.md`
  - [ ] `docs/runbook/scaling-guide.md`
  - [ ] `docs/runbook/service-restart.md`

### Log Aggregation (Loki)
- [ ] Loki deployed and running
- [ ] Promtail collecting logs from all pods
- [ ] Logs queryable in Grafana
- [ ] Log retention configured (7 days minimum)
- [ ] Log volume within storage limits
- [ ] Structured logging working (JSON format)

### Distributed Tracing (Tempo)
- [ ] Tempo deployed and running
- [ ] All services sending traces
- [ ] Traces viewable in Grafana
- [ ] Trace sampling configured
- [ ] End-to-end traces working
  - [ ] Order placement → fill → position update

### Metrics Verification
- [ ] Key metrics being collected
  - [ ] Request rate (requests/sec)
  - [ ] Error rate (errors/sec)
  - [ ] Latency (p50, p95, p99)
  - [ ] WebSocket connections (active count)
  - [ ] Kafka consumer lag
  - [ ] Database connection pool usage
  - [ ] Order processing latency
  - [ ] Market data tick rate

---

## 5. T-3 Days: Performance Validation

### Load Testing - WebSocket
- [ ] WebSocket load test completed
  ```bash
  make load-test-ws EMAIL=test@example.com PASSWORD=pass123 CONTEST_ID=abc123 N=1000 DURATION=300s
  ```
- [ ] Results documented
  - [ ] Connection success rate: >99%
  - [ ] Connection latency p99: <500ms
  - [ ] Tick delivery latency p99: <100ms
  - [ ] Message throughput: sustained for 5+ minutes
- [ ] No connection drops during test
- [ ] No memory leaks detected
- [ ] CPU/memory usage acceptable under load

### Load Testing - Order Placement
- [ ] Order load test completed
  ```bash
  cd tools/order-load-test && go run . -contest-id <id> -users 100 -duration 300s
  ```
- [ ] Results documented
  - [ ] Order submission success rate: >99%
  - [ ] Order-to-acknowledgment latency p99: <200ms
  - [ ] Order-to-fill latency p99: <500ms
- [ ] No order processing errors
- [ ] Database write performance acceptable
- [ ] Kafka not backing up

### Stress Testing
- [ ] Stress test with 2x expected load completed
- [ ] System remained stable
- [ ] Graceful degradation observed (if applicable)
- [ ] Recovery after load removal verified

### Database Performance
- [ ] Slow query log reviewed (queries >100ms)
- [ ] All queries have appropriate indexes
- [ ] Query plan analysis completed for critical queries
  - [ ] Order insertion
  - [ ] Leaderboard retrieval
  - [ ] Position updates
- [ ] Connection pool size optimized
- [ ] No connection pool exhaustion under load

### Auto-Scaling (HPA)
- [ ] Horizontal Pod Autoscaler configured
  - [ ] user-bff
  - [ ] trade-bff
  - [ ] admin-bff
  - [ ] trading-engine
  - [ ] market-ingestor
- [ ] Scaling thresholds configured
  - [ ] CPU target: 70%
  - [ ] Memory target: 80%
- [ ] Scale-up tested (increase load → pods increase)
- [ ] Scale-down tested (decrease load → pods decrease)
- [ ] Scaling metrics working

### Failover Testing
- [ ] Pod failure tested
  ```bash
  kubectl delete pod <pod-name> -n tragge
  ```
  - [ ] Pod recreated automatically
  - [ ] Health checks pass within 30s
- [ ] Node failure simulated (drain node)
  - [ ] Pods rescheduled to other nodes
  - [ ] No service disruption
- [ ] Database failover tested (if HA setup)
- [ ] Redis failover tested (if HA setup)

### Memory Leak Detection
- [ ] All services monitored for 24+ hours
- [ ] Memory usage stable (no linear growth)
- [ ] Go pprof profiles captured and analyzed
- [ ] No goroutine leaks detected

---

## 6. T-1 Day: Final Verification

### Service Health
- [ ] All health endpoints returning 200 OK
  ```bash
  curl -f https://api.tragge.example.com/user/healthz
  curl -f https://api.tragge.example.com/trade/healthz
  curl -f https://api.tragge.example.com/admin/healthz
  ```
- [ ] Readiness endpoints passing
- [ ] All pods in Running state
- [ ] No recent pod restarts

### Market Data Feed
- [ ] TwelveData connection active
- [ ] Price ticks being received
- [ ] Tick rate acceptable (>1 tick/sec for active symbols)
- [ ] Massive fallback tested (disconnect TwelveData)
- [ ] Failover to Massive working (<30s)
- [ ] Auto-reconnect working

### End-to-End User Flows
- [ ] **User Registration**
  - [ ] Register new user via UI
  - [ ] Email validation (if applicable)
  - [ ] User created in database
  - [ ] Default role assigned
- [ ] **User Login**
  - [ ] Login with correct credentials
  - [ ] JWT tokens received
  - [ ] Session created in Redis
  - [ ] Access token works for authenticated endpoints
- [ ] **Contest Creation (Admin)**
  - [ ] Login as admin
  - [ ] Create new contest
  - [ ] Contest visible in database
  - [ ] Contest status: draft → scheduled → registration_open
- [ ] **Contest Enrollment**
  - [ ] User joins contest
  - [ ] Enrollment recorded in database
  - [ ] Contest appears in user's dashboard
- [ ] **WebSocket Connection**
  - [ ] Connect to `/ws/trade` with JWT
  - [ ] Connection established
  - [ ] Ping/pong keepalive working
  - [ ] Price ticks received
- [ ] **Order Placement**
  - [ ] Place market order
  - [ ] Order acknowledgment received
  - [ ] Order executed (fill event received)
  - [ ] Position updated
  - [ ] Position visible in UI
- [ ] **Order Types**
  - [ ] MARKET order executed immediately
  - [ ] BUY_LIMIT order pending until price reached
  - [ ] SELL_LIMIT order pending until price reached
  - [ ] BUY_STOP order triggered correctly
  - [ ] SELL_STOP order triggered correctly
  - [ ] Take Profit executed
  - [ ] Stop Loss executed
- [ ] **Leaderboard**
  - [ ] Leaderboard updates after trade
  - [ ] PnL calculation correct
  - [ ] Rankings correct
  - [ ] Real-time updates working
- [ ] **Remaining payment providers**
  - [ ] NOWPayments signature, freshness, and replay fixtures pass
  - [ ] Jibit request authentication and callback allowlist fixtures pass
  - [ ] Provider circuit health reports only configured active providers
  - [ ] Deposit confirmation email is sent after successful payment
  - [ ] Remaining providers initialize independently

### Admin Panel
- [ ] Admin login working
- [ ] Contest management UI accessible
- [ ] Contest CRUD operations working
- [ ] Audit logs viewable
- [ ] User search working
- [ ] Statistics dashboard loading

### Frontend Verification
- [ ] All frontends loading without errors
  - [ ] https://tragge.example.com/user
  - [ ] https://tragge.example.com/trade
  - [ ] https://tragge.example.com/admin
- [ ] Assets loading from CDN (if applicable)
- [ ] i18n working (English and Farsi)
- [ ] Responsive design working (mobile, tablet, desktop)
- [ ] No console errors in browser

### Integration Tests
- [ ] All integration tests passing
  ```bash
  cd tests/integration && go test -v ./...
  ```
- [ ] E2E tests passing
  ```bash
  make e2e
  ```

---

## 7. Launch Day (T-0): Go/No-Go Decision

### Final Checklist
- [ ] All T-1 Day checks still passing
- [ ] No open critical bugs
- [ ] On-call engineer available and ready
- [ ] Secondary on-call engineer identified
- [ ] War room established (Slack channel: #launch-war-room)
- [ ] All stakeholders available
  - [ ] Tech Lead
  - [ ] Product Manager
  - [ ] Customer Support Lead
  - [ ] Marketing Lead

### Rollback Preparation
- [ ] Rollback procedure documented and reviewed
- [ ] Previous stable version identified
- [ ] Rollback tested in staging
- [ ] Database migration rollback tested (if schema changed)
- [ ] Rollback can be executed in <5 minutes

### Communication Preparation
- [ ] Customer support team briefed
- [ ] Support documentation updated
- [ ] FAQ prepared for common issues
- [ ] Status page ready (if applicable)
- [ ] Social media/marketing announcement ready
- [ ] Email announcement drafted (if applicable)

### Stakeholder Approval
- [ ] Tech Lead approval: ________ (signature/timestamp)
- [ ] Product Manager approval: ________ (signature/timestamp)
- [ ] CTO/Engineering Director approval: ________ (signature/timestamp)

### Go/No-Go Decision
- [ ] **GO** - All checks passed, proceed with launch
- [ ] **NO-GO** - Critical issues identified, postpone launch

---

## 8. Launch Day: Execution

### Pre-Launch (T-0 minus 1 hour)
- [ ] All team members in war room (Slack)
- [ ] Monitoring dashboards open
  - [ ] Grafana: System Overview
  - [ ] Grafana: WebSocket Real-time
  - [ ] Grafana: Kafka/Redpanda Health
  - [ ] Prometheus: Alerts
  - [ ] Kubernetes Dashboard
- [ ] Silence non-critical alerts (optional)

### Launch (T-0)
- [ ] **Execute traffic switch**
  - Option A: Update DNS to point to production
  - Option B: Enable ingress/load balancer
  - Option C: Remove maintenance page
- [ ] Record launch time: __________
- [ ] Monitor dashboards for 5 minutes
  - [ ] Error rate normal
  - [ ] Latency normal
  - [ ] No critical alerts

### Post-Launch Monitoring (First 30 Minutes)
- [ ] **T+5 min**: First verification
  - [ ] All pods still running
  - [ ] Error rate: <1%
  - [ ] No alerts firing
- [ ] **T+10 min**: User activity verification
  - [ ] First user registration detected
  - [ ] First user login detected
  - [ ] First WebSocket connection detected
- [ ] **T+15 min**: Trading verification
  - [ ] First contest join detected
  - [ ] First order placed
  - [ ] First fill executed
  - [ ] Leaderboard updating
- [ ] **T+20 min**: Performance check
  - [ ] Latency p99 < 200ms
  - [ ] CPU usage < 70%
  - [ ] Memory usage < 80%
  - [ ] Kafka consumer lag < 100
- [ ] **T+30 min**: Final verification
  - [ ] No critical errors in logs
  - [ ] All integrations working (TwelveData, Massive)
  - [ ] Database connections stable
  - [ ] Redis operations normal

### Launch Confirmation
- [ ] Send launch confirmation in Slack
- [ ] Post status update (status page)
- [ ] Send announcement (social media, email)
- [ ] Update documentation with production URLs

### Rollback Decision Point (T+30 min)
- [ ] **CONTINUE** - No critical issues, continue normal operation
- [ ] **ROLLBACK** - Critical issues detected, execute rollback

---

## 9. Post-Launch (T+1 to T+7 Days)

### Daily Monitoring (T+1 to T+7)
- [ ] **Day 1**: Monitor every 2 hours
  - [ ] Review error logs
  - [ ] Review alert history
  - [ ] Check user feedback/support tickets
  - [ ] Verify backup completed successfully
- [ ] **Day 2**: Monitor every 4 hours
  - [ ] Performance trending analysis
  - [ ] Database growth rate
  - [ ] Cost analysis (cloud resources)
- [ ] **Day 3**: Monitor every 6 hours
  - [ ] User growth metrics
  - [ ] Trading volume metrics
  - [ ] Contest participation metrics
- [ ] **Days 4-7**: Monitor daily
  - [ ] Weekly performance report
  - [ ] Security incident review
  - [ ] Capacity planning review

### Issue Resolution
- [ ] All P0/P1 issues resolved
- [ ] User-reported bugs triaged
- [ ] Performance optimization opportunities identified
- [ ] Technical debt documented

### Backup Verification
- [ ] First PostgreSQL backup completed
- [ ] First Redis backup completed
- [ ] Test restore performed successfully
- [ ] Backup monitoring alerts working

### Performance Review
- [ ] Metrics collected and analyzed
  - [ ] Average response time
  - [ ] Error rate
  - [ ] Uptime percentage
  - [ ] Peak concurrent users
  - [ ] Database query performance
  - [ ] Kafka throughput
- [ ] Performance bottlenecks identified
- [ ] Optimization plan created (if needed)

### Security Review
- [ ] No security incidents reported
- [ ] Access logs reviewed
- [ ] Failed login attempts reviewed
- [ ] API key usage reviewed
- [ ] SSL certificate expiry confirmed (90 days+)

### User Feedback
- [ ] Support tickets reviewed
- [ ] User feedback collected
- [ ] Feature requests documented
- [ ] Bug reports prioritized

### Post-Mortem
- [ ] Launch retrospective scheduled
- [ ] What went well documented
- [ ] What could be improved documented
- [ ] Action items created
- [ ] Lessons learned shared with team

---

## Appendix A: Emergency Contacts

| Role | Name | Phone | Email | Slack |
|------|------|-------|-------|-------|
| Tech Lead | TBD | TBD | TBD | @techlead |
| On-Call Primary | TBD | TBD | TBD | @oncall |
| On-Call Secondary | TBD | TBD | TBD | @oncall-backup |
| Product Manager | TBD | TBD | TBD | @pm |
| Database Admin | TBD | TBD | TBD | @dba |
| DevOps Lead | TBD | TBD | TBD | @devops |
| Security Lead | TBD | TBD | TBD | @security |
| Customer Support Lead | TBD | TBD | TBD | @support |

**Escalation Path:**
1. On-Call Primary (respond within 15 minutes)
2. On-Call Secondary (if Primary unavailable)
3. Tech Lead (for critical decisions)
4. CTO/VP Engineering (for business-critical incidents)

---

## Appendix B: Rollback Procedure

### When to Rollback
Rollback immediately if any of the following occur within first hour of launch:
- Error rate >10%
- Service completely unavailable
- Data corruption detected
- Security breach detected
- Critical functionality broken (unable to place orders, login, etc.)

### Rollback Steps

#### Option 1: Kubernetes Rollback (Fastest)
```bash
# Rollback all services
kubectl rollout undo deployment/user-bff -n tragge
kubectl rollout undo deployment/trade-bff -n tragge
kubectl rollout undo deployment/admin-bff -n tragge
kubectl rollout undo deployment/trading-engine -n tragge
kubectl rollout undo deployment/market-ingestor -n tragge
kubectl rollout undo deployment/leaderboard-worker -n tragge
kubectl rollout undo deployment/frontend -n tragge
kubectl rollout undo deployment/frontend -n tragge
kubectl rollout undo deployment/frontend -n tragge
kubectl rollout undo deployment/gateway -n tragge

# Verify rollback
kubectl rollout status deployment/user-bff -n tragge
kubectl rollout status deployment/trade-bff -n tragge
# ... (repeat for all services)

# Check all pods running
kubectl get pods -n tragge
```

#### Option 2: Reapply Previous Version
```bash
# Checkout previous version tag
git checkout v1.0.0  # Replace with previous stable version

# Apply previous manifests
kubectl apply -k infra/k8s/overlays/production

# Verify deployment
kubectl get pods -n tragge
```

#### Option 3: Database Rollback (If Schema Changed)
```bash
# Connect to database pod
kubectl exec -it postgres-0 -n tragge -- bash

# Rollback migration
make migrate-down

# Verify migration version
make migrate-version
```

### Post-Rollback Verification
- [ ] All pods running
- [ ] Health checks passing
- [ ] User registration working
- [ ] Login working
- [ ] Trading working
- [ ] Notify team in #launch-war-room
- [ ] Update status page
- [ ] Post-mortem scheduled

---

## Appendix C: Key URLs

### User-Facing URLs
| Service | URL | Description |
|---------|-----|-------------|
| User App | https://tragge.example.com/user | User registration, login, profile |
| Trade App | https://tragge.example.com/trade | Trading interface |
| Admin App | https://tragge.example.com/admin | Admin dashboard |

### API Endpoints
| Service | URL | Description |
|---------|-----|-------------|
| User API | https://api.tragge.example.com/user | User BFF API |
| Trade API | https://api.tragge.example.com/trade | Trade BFF API |
| Admin API | https://api.tragge.example.com/admin | Admin BFF API |
| WebSocket | wss://api.tragge.example.com/ws/trade | Trading WebSocket |

### Monitoring & Observability
| Service | URL | Description |
|---------|-----|-------------|
| Grafana | https://grafana.tragge.example.com | Dashboards & visualization |
| Prometheus | https://prometheus.tragge.example.com | Metrics & alerts |
| Alertmanager | https://alertmanager.tragge.example.com | Alert management |
| Redpanda Console | https://redpanda-console.tragge.example.com | Kafka topic monitoring |

### Infrastructure
| Service | URL | Description |
|---------|-----|-------------|
| Kubernetes Dashboard | https://k8s-dashboard.tragge.example.com | Cluster management |
| Status Page | https://status.tragge.example.com | Public status page |

---

## Appendix D: Launch Day Runbook

### Critical Commands

**Check all pods:**
```bash
kubectl get pods -n tragge -o wide
```

**Check pod logs:**
```bash
kubectl logs -f deployment/trade-bff -n tragge
```

**Check recent events:**
```bash
kubectl get events -n tragge --sort-by='.lastTimestamp'
```

**Check service endpoints:**
```bash
kubectl get endpoints -n tragge
```

**Scale deployment:**
```bash
kubectl scale deployment/trade-bff --replicas=5 -n tragge
```

**Restart deployment:**
```bash
kubectl rollout restart deployment/trade-bff -n tragge
```

**Check resource usage:**
```bash
kubectl top pods -n tragge
kubectl top nodes
```

**Check Kafka lag:**
```bash
kubectl exec -it redpanda-0 -n tragge -- rpk group describe <consumer-group>
```

**Check PostgreSQL connections:**
```bash
kubectl exec -it postgres-0 -n tragge -- psql -U app -c "SELECT count(*) FROM pg_stat_activity;"
```

### Common Issues & Solutions

| Issue | Symptoms | Solution |
|-------|----------|----------|
| Pod CrashLoopBackOff | Pod restarting repeatedly | Check logs, verify environment variables, check dependencies |
| High latency | Slow response times | Check database slow queries, scale up pods, check Kafka lag |
| WebSocket disconnects | Clients losing connection | Check load balancer timeout, check pod resource limits |
| Kafka consumer lag | Messages backing up | Scale consumer pods, check consumer performance |
| Database connection pool exhausted | Connection errors | Increase pool size, check for connection leaks |
| Out of memory | Pods evicted | Increase memory limits, check for memory leaks |

---

## Appendix E: Success Metrics

Track these metrics for the first week:

### Availability
- [ ] Uptime: >99.9%
- [ ] Error rate: <0.1%
- [ ] P99 latency: <200ms

### Performance
- [ ] Order placement latency p99: <200ms
- [ ] WebSocket connection success rate: >99%
- [ ] Market data tick rate: >1 tick/sec

### User Engagement
- [ ] Total registrations: ________
- [ ] Active users (DAU): ________
- [ ] Contest enrollments: ________
- [ ] Total trades executed: ________

### System Health
- [ ] No critical incidents
- [ ] No security breaches
- [ ] No data loss
- [ ] All backups successful

---

**Checklist Version:** 1.0
**Last Updated:** January 2026
**Owner:** Tech Lead / DevOps Team

**Notes:**
- Review and update this checklist quarterly
- Adapt timelines based on your release schedule
- Add organization-specific requirements as needed
- Keep contact information up to date
