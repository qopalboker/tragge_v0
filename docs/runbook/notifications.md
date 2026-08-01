# Notifications Runbook

This runbook covers troubleshooting notification delivery issues in the Tragge platform.

## Alert Types

- **NotificationDeliveryFailure**: Notification service is experiencing delivery failures
- **NotificationHighFailureRate**: Notification failure rate exceeds threshold
- **NotificationQueueBacklog**: Notification queue has accumulated backlog

## Common Issues and Resolutions

### Notification Delivery Failure

**Symptoms:**
- Notifications not being delivered to users
- High error rates in notification metrics
- Users reporting missing alerts or messages

**Investigation Steps:**

1. **Check notification service health:**
   ```bash
   kubectl get pods -l app=notification-service -n tragge
   kubectl logs -l app=notification-service -n tragge --tail=100
   ```

2. **Check external provider status:**
   - Discord API status: https://discordstatus.com/
   - Resend API status: https://status.resend.com/

3. **Verify credentials:**
   ```bash
   # Check if secrets are mounted correctly
   kubectl exec -it deploy/notification-service -n tragge -- cat /run/secrets/discord_webhook_url | head -c 20
   kubectl exec -it deploy/notification-service -n tragge -- cat /run/secrets/resend_api_key | head -c 10
   ```

4. **Check rate limits:**
   - Discord: 30 requests per minute per webhook
   - Resend: Check your plan limits

**Resolution Steps:**

1. If credentials are invalid, rotate them:
   ```bash
   # Update Discord webhook
   kubectl create secret generic discord-webhook \
     --from-literal=url='YOUR_NEW_WEBHOOK_URL' \
     -n tragge --dry-run=client -o yaml | kubectl apply -f -

   # Restart notification service
   kubectl rollout restart deploy/notification-service -n tragge
   ```

2. If rate limited, implement backoff:
   - Configure exponential backoff in notification service
   - Consider batching notifications

### High Failure Rate

**Symptoms:**
- Notification failure rate > 5%
- Intermittent delivery issues
- Some notifications succeed, others fail

**Investigation Steps:**

1. **Analyze failure patterns:**
   ```promql
   # Check failure rate by type
   sum by (notification_type, error) (
     rate(notification_failures_total[5m])
   )
   ```

2. **Check for specific error types:**
   - 429 (Rate Limited): Reduce send rate
   - 401/403 (Auth): Check credentials
   - 5xx (Server Error): External provider issue

**Resolution Steps:**

1. For rate limiting:
   - Increase batch interval
   - Implement priority queues (critical first)

2. For authentication errors:
   - Rotate credentials
   - Verify webhook URLs are correct

3. For server errors:
   - Wait for provider to recover
   - Enable fallback notifications (e.g., Discord fails -> email)

### Queue Backlog

**Symptoms:**
- Notifications delayed
- Queue size growing
- Memory usage increasing in notification service

**Investigation Steps:**

1. **Check queue metrics:**
   ```promql
   notification_queue_size
   notification_queue_age_seconds
   ```

2. **Check processing rate:**
   ```promql
   rate(notifications_processed_total[5m])
   ```

**Resolution Steps:**

1. **Scale notification workers:**
   ```bash
   kubectl scale deploy/notification-service --replicas=3 -n tragge
   ```

2. **Prioritize critical notifications:**
   - Ensure critical alerts are sent first
   - Consider dropping old, non-critical notifications

3. **Increase throughput:**
   - Batch more notifications
   - Increase worker concurrency

## Preventive Measures

1. **Monitor notification metrics:**
   - Add alerts for queue size > threshold
   - Track delivery latency

2. **Implement fallbacks:**
   - If Discord fails, try email
   - Store failed notifications for retry

3. **Rate limit awareness:**
   - Track rate limit headers
   - Implement proactive throttling

## Escalation

If issues persist after following this runbook:

1. Check provider status pages
2. Contact provider support if needed
3. Escalate to platform team for architecture review

## Related Runbooks

- [Incident Response](incident-response.md)
- [Scaling Guide](scaling-guide.md)
