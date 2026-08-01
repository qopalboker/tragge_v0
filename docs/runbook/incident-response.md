# Incident Response Runbook

This document outlines procedures for responding to incidents affecting the tragge Trading Tournament Platform.

## Table of Contents

1. [Incident Severity Levels](#incident-severity-levels)
2. [Initial Response](#initial-response)
3. [Communication Protocols](#communication-protocols)
4. [Common Incident Types](#common-incident-types)
5. [Escalation Procedures](#escalation-procedures)
6. [Post-Incident Review](#post-incident-review)

---

## Incident Severity Levels

| Level | Name | Description | Response Time | Examples |
|-------|------|-------------|---------------|----------|
| **SEV-1** | Critical | Complete system outage, data loss risk | 15 minutes | Database down, all services unreachable |
| **SEV-2** | Major | Significant feature degradation | 30 minutes | Trading engine down, authentication failing |
| **SEV-3** | Minor | Single service impacted | 2 hours | Leaderboard delayed, market data gaps |
| **SEV-4** | Low | Minor issues, no user impact | 24 hours | Log errors, minor performance issues |

---

## Initial Response

### Step 1: Acknowledge and Assess (5 minutes)

1. **Acknowledge the alert** in your monitoring system
2. **Open an incident channel** (e.g., `#incident-YYYYMMDD-N`)
3. **Initial assessment checklist:**
   - [ ] What symptoms are being reported?
   - [ ] When did the issue start?
   - [ ] What is the blast radius (users/services affected)?
   - [ ] Are there any recent deployments or changes?

### Step 2: Assemble Response Team

```bash
# Check who is on-call
kubectl get configmap on-call-schedule -o yaml

# Page additional responders if needed (SEV-1/SEV-2)
```

**Roles:**
- **Incident Commander (IC)**: Coordinates response, makes decisions
- **Technical Lead**: Investigates and implements fixes
- **Communications Lead**: Updates stakeholders

### Step 3: Initial Diagnostics

Run the health check script:

```bash
# Quick system health check
./scripts/health-check.sh

# Or check individual services
for svc in user-bff trade-bff admin-bff trading-engine market-ingestor leaderboard-worker; do
    echo "=== $svc ==="
    curl -s "http://${svc}:8080/healthz" | jq .
done
```

Check infrastructure status:

```bash
# PostgreSQL
pg_isready -h postgres -U app

# Redis
redis-cli -h redis PING

# Kafka/Redpanda
rpk cluster health

# All pods status
kubectl get pods -o wide
```

---

## Communication Protocols

### Internal Updates

Post updates to incident channel every **15 minutes** for SEV-1/SEV-2:

```
[UPDATE HH:MM UTC]
Status: Investigating/Mitigating/Resolved
Current state: [brief description]
Impact: [users/features affected]
Next steps: [what we're doing]
ETA: [if known]
```

### External Status Page Updates

**SEV-1/SEV-2:** Update status page within 10 minutes

Template:
```
Title: [Service] Degraded/Down
Body: We are experiencing issues with [describe impact].
      We are actively investigating and will provide updates.
      Started: [timestamp]
```

### Stakeholder Notifications

| Severity | Notify | Within |
|----------|--------|--------|
| SEV-1 | CTO, Engineering Lead, Support Lead | 15 min |
| SEV-2 | Engineering Lead, Support Lead | 30 min |
| SEV-3 | Team Lead | 2 hours |
| SEV-4 | N/A | Daily standup |

---

## Common Incident Types

### Database Unavailable (SEV-1)

**Symptoms:**
- Services returning 500 errors
- "connection refused" in logs
- Health checks failing

**Immediate Actions:**

```bash
# Check PostgreSQL status
kubectl get pods -l app=postgres
kubectl logs -l app=postgres --tail=100

# Check connections
psql -h postgres -U app -c "SELECT count(*) FROM pg_stat_activity;"

# If pod is crashing
kubectl describe pod -l app=postgres

# If disk is full
kubectl exec -it postgres-0 -- df -h

# Restart PostgreSQL (last resort)
kubectl rollout restart statefulset/postgres
```

**Recovery:**
- See [Database Recovery Runbook](./database-recovery.md)

---

### Trading Engine Down (SEV-1)

**Symptoms:**
- Orders not being processed
- WebSocket disconnections
- "trading-engine unhealthy" alerts

**Immediate Actions:**

```bash
# Check trading-engine pods
kubectl get pods -l app=trading-engine
kubectl logs -l app=trading-engine --tail=200

# Check Kafka connectivity
rpk topic consume orders.v1 --num 1

# Check for stuck consumers
rpk group describe trading-engine-group

# Restart trading-engine
kubectl rollout restart deployment/trading-engine
```

**Impact Mitigation:**
1. Enable maintenance mode on frontend
2. Notify active contest participants
3. Consider pausing active contests

---

### Market Data Gaps (SEV-2)

**Symptoms:**
- Stale prices on frontend
- "No ticks received" alerts
- market-ingestor health check failing

**Immediate Actions:**

```bash
# Check market-ingestor status
kubectl logs -l app=market-ingestor --tail=100

# Check provider status
curl -s http://market-ingestor:8085/healthz | jq .

# Verify tick flow
rpk topic consume ticks.v1 --num 5

# Force provider switch
kubectl exec -it market-ingestor-xxx -- kill -USR1 1
```

---

### Authentication Failures (SEV-2)

**Symptoms:**
- Users unable to log in
- "invalid token" errors
- Session-related errors

**Immediate Actions:**

```bash
# Check user-bff status
kubectl logs -l app=user-bff --tail=100

# Check Redis (sessions)
redis-cli -h redis PING
redis-cli -h redis DBSIZE

# Verify JWT secret is set
kubectl get secret jwt-secret -o yaml

# Restart user-bff
kubectl rollout restart deployment/user-bff
```

---

### High Latency (SEV-3)

**Symptoms:**
- Slow response times
- Timeout errors
- User complaints about performance

**Diagnostic Steps:**

```bash
# Check pod resource usage
kubectl top pods

# Check database slow queries
psql -h postgres -U app -c "
SELECT pid, now() - pg_stat_activity.query_start AS duration, query
FROM pg_stat_activity
WHERE state = 'active' AND now() - pg_stat_activity.query_start > interval '5 seconds';"

# Check Redis memory
redis-cli -h redis INFO memory

# Check Kafka lag
rpk group describe trading-engine-group
```

---

### Kafka/Redpanda Issues (SEV-2)

**Symptoms:**
- Event processing delays
- Consumer lag increasing
- Connection errors in logs

**Immediate Actions:**

```bash
# Check Redpanda cluster health
rpk cluster health

# Check topic status
rpk topic list

# Check consumer groups
rpk group list

# Check for under-replicated partitions
rpk cluster partition-status

# Restart Redpanda (if necessary)
kubectl rollout restart statefulset/redpanda
```

---

## Escalation Procedures

### When to Escalate

Escalate when:
- Issue persists beyond response time SLA
- Root cause unclear after 30 minutes
- Fix requires changes beyond your access/expertise
- Customer impact is expanding

### Escalation Path

```
On-Call Engineer
      ↓
Team Lead (if needed)
      ↓
Engineering Manager (SEV-1/SEV-2)
      ↓
CTO (SEV-1 lasting > 1 hour)
```

### How to Escalate

1. Document current state and actions taken
2. Contact next level via PagerDuty/Slack/Phone
3. Brief them on: symptoms, impact, actions taken, what help is needed
4. Hand off IC role if appropriate

---

## Post-Incident Review

### Timeline (SEV-1/SEV-2)

| Timeframe | Action |
|-----------|--------|
| 24 hours | Draft incident summary |
| 48 hours | Schedule post-mortem meeting |
| 5 days | Complete post-mortem document |
| 2 weeks | Implement critical action items |

### Post-Mortem Template

```markdown
# Incident Post-Mortem: [Title]

**Date:** YYYY-MM-DD
**Duration:** X hours Y minutes
**Severity:** SEV-X
**Author:** [Name]

## Summary
[1-2 sentence description]

## Impact
- Users affected: X
- Duration of impact: X
- Revenue impact: $X

## Timeline (UTC)
- HH:MM - [Event]
- HH:MM - [Event]

## Root Cause
[Detailed explanation]

## Resolution
[What fixed it]

## Action Items
- [ ] [Action] - Owner - Due Date
- [ ] [Action] - Owner - Due Date

## Lessons Learned
- What went well
- What could be improved
```

### Blameless Culture

- Focus on systems and processes, not individuals
- Ask "what" and "how", not "who"
- Encourage sharing mistakes to improve collectively
- Celebrate transparency and learning

---

## Quick Reference

### Key Commands

```bash
# System overview
kubectl get pods,svc,deploy

# Logs for all services
stern -l "app in (user-bff,trade-bff,admin-bff,trading-engine)"

# Database connections
psql -h postgres -U app -c "SELECT * FROM pg_stat_activity;"

# Redis status
redis-cli -h redis INFO

# Kafka topics
rpk topic list && rpk group list
```

### Key Contacts

| Role | Contact |
|------|---------|
| On-Call | Check PagerDuty |
| Engineering Lead | @eng-lead |
| Database Admin | @dba-team |
| Infrastructure | @infra-team |

### Useful Links

- Monitoring Dashboard: [link]
- Status Page Admin: [link]
- Runbooks: `/docs/runbook/`
- Architecture Docs: `/docs/architecture/`

---

*Last Updated: January 2026*
