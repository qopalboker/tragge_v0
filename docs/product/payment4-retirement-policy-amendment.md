# Payment4 retirement policy amendment

**Decision ID:** `PAYMENT4-RETIREMENT-2026-08-01`

**Status:** Accepted

**Effective date:** 2026-08-01

**Responsible task context:** `SEC-006`

## Decision

Payment4 is retired and is not an active Tragge payment provider. It must not
be selectable by configuration, receive requests or webhooks, load or require
secrets, register routes, participate in startup validation, appear in a user
or Admin option, or remain a runtime-test dependency.

This decision does not approve or add a replacement provider. The fixed policy
continues to name Jibit and Sepal as Rial gateways and Plisio and NOWPayments
as crypto gateways. The current legacy payment-service implementation that
remains executable after this decision contains Jibit and NOWPayments adapters;
Sepal and Plisio remain future roadmap work under `PAY-002` and `PAY-003`.

Paid-production status remains `NO-GO`. Retiring one unavailable provider is
not production-readiness evidence.

## Reason

The Payment4 business/service is no longer available. Retaining dormant code,
configuration, secrets, routes, or an end-to-end gate would create a false
supported-provider contract and an unsafe reactivation path.

## Affected components

- Payment-service provider registry, deposit validation, webhook routing,
  polling, circuit health, and provider-specific metrics.
- User and Admin frontend provider choices and compatibility handling.
- Docker, Kubernetes, Nginx, example environment, and secret initialization.
- Payment-provider runbooks, go-live guidance, and technical documentation.
- SEC-006 validation and its previously blocked provider-specific E2E gate.

## Migration consequences

No migration or target initialization file contains a Payment4 identifier,
provider enum, seed, or reference row. Legacy migration `0004_wallet.up.sql`
stores a generic `payment_intents.provider VARCHAR(50)`; it is immutable
history and is not a provider registry. New Payment4 rows cannot be created
through the active API because crypto deposits accept only NOWPayments and the
retired adapter is absent.

The database is pre-launch and disposable under the FND-004 reset policy.
Existing local rows using the retired provider value are discarded by a clean
reset; no automatic compatibility importer is added. If preservation is ever
explicitly required, it must use the isolated, versioned, reconciled legacy
import process defined by FND-004 and must not reactivate the provider.

## Rollback implications

There is no runtime rollback flag. Reintroducing Payment4 would require a new
explicit product decision, a separately approved roadmap task, commercial and
security review, a new adapter and contracts, configuration and secret review,
webhook verification, migrations if needed, and full runtime evidence. Revert
of the SEC-006 commit is therefore not an approved production rollback because
it would restore a retired and unavailable provider.

## Occurrence inventory before removal

The repository-wide case-insensitive scan found 41 files. Every occurrence was
classified before removal:

| Repository path | Classification | Treatment |
|---|---|---|
| `.env.example` | active configuration | Remove variables, callback, webhook, and secret mapping. |
| `apps/admin-frontend/src/i18n/locales/en.ts` | active frontend option | Remove labels. |
| `apps/admin-frontend/src/i18n/locales/fa.ts` | active frontend option | Remove labels. |
| `apps/admin-frontend/src/modules/admin/views/FinancialPage.vue` | active frontend styling | Remove provider badge style. |
| `apps/gateway/nginx.conf` | active route configuration | Remove webhook location. |
| `apps/gateway/nginx.prod.conf` | active route configuration | Remove webhook location. |
| `apps/payment-service/handlers/deposit.go` | active implementation | Reject every crypto provider except NOWPayments and remove special branches/metrics. |
| `apps/payment-service/handlers/metrics.go` | active implementation | Delete provider-only observer interface. |
| `apps/payment-service/handlers/payment4_e2e_test.go` | obsolete active test | Delete; replace with retirement and remaining-provider evidence. |
| `apps/payment-service/handlers/webhook.go` | active implementation | Remove handler, verification branch, and metrics. |
| `apps/payment-service/handlers/webhook_security.go` | active implementation | Remove provider-specific timestamp alias. |
| `apps/payment-service/handlers/webhook_security_test.go` | active test | Retarget freshness/replay evidence to NOWPayments. |
| `apps/payment-service/providers/payment4.go` | active implementation | Delete adapter. |
| `apps/payment-service/providers/payment4_integration_test.go` | obsolete active test | Delete external integration tests. |
| `apps/payment-service/providers/payment4_test.go` | obsolete active test | Delete provider unit tests. |
| `apps/payment-service/providers/payment4_webhook_test.go` | obsolete active test | Delete provider webhook tests. |
| `apps/payment-service/providers/provider.go` | active provider registry | Remove provider identifier. |
| `apps/payment-service/server/app.go` | active construction and routing | Remove startup requirement, construction, registration, route, and metrics dependency. |
| `apps/payment-service/server/circuits.go` | active runtime health | Remove circuit and health member. |
| `apps/payment-service/server/config.go` | active configuration | Remove accepted environment and secret fields. |
| `apps/payment-service/server/inquiry.go` | active background worker | Remove provider polling path. |
| `apps/payment-service/server/metrics.go` | active observability | Delete provider-specific metrics. |
| `apps/user-frontend/src/i18n/locales/en.ts` | active frontend option | Remove labels. |
| `apps/user-frontend/src/i18n/locales/fa.ts` | active frontend option | Remove labels. |
| `apps/user-frontend/src/modules/user/api/index.ts` | active frontend API | Remove union member and request branch. |
| `apps/user-frontend/src/modules/user/components/wallet/DepositModal.vue` | active frontend flow | Use remaining crypto provider flow. |
| `apps/user-frontend/src/modules/user/stores_wallet.ts` | active frontend flow | Remove provider compatibility arguments. |
| `apps/user-frontend/src/modules/user/views/PaymentResultPage.vue` | active frontend compatibility | Remove retired callback identifier fallback. |
| `CLAUDE.md` | active technical documentation | Remove supported-provider instructions. |
| `docs/codex/reports/SEC-006-git-execution-report.md` | historical evidence | Preserve original failure and add dated remediation. |
| `docs/go-live-checklist.md` | active operational documentation | Replace with remaining-provider checks. |
| `docs/runbook/deployment-procedures.md` | active operational documentation | Replace deployment prerequisites. |
| `docs/SECURE_KEY_MANAGEMENT.md` | active security documentation | Remove secret lifecycle and examples. |
| `docs/security/edge-security-and-abuse-controls.md` | active security documentation | Document only remaining webhook path. |
| `infra/docker/docker-compose.yml` | active deployment configuration | Remove secrets and environment. |
| `infra/k8s/base/configmap.yaml` | active deployment configuration | Remove variables and endpoints. |
| `infra/k8s/base/external-secrets.yaml` | active deployment configuration | Remove provider secret projection. |
| `infra/k8s/base/network-policies.yaml` | active deployment documentation | Remove provider from egress comment. |
| `infra/k8s/base/secrets.yaml` | active deployment configuration | Remove placeholder secret. |
| `scripts/sec-006-edge-security-check.mjs` | active validation | Replace adapter assertion with retirement validation. |
| `scripts/secrets/init-secrets.sh` | active secret initialization | Remove secret creation and operator prompt. |

The scan found no Payment4 occurrence under `packages/db/migrations` or
`packages/db/init`. Test-only retirement fixtures may contain the retired
identifier solely to prove rejection and generic 404 behavior. This decision
record and current SEC-006 report may name it to explain the retirement.

## Validation replacement

The obsolete Payment4 E2E rerun is replaced by:

- structural validation that active source, configuration, deployment, secret,
  frontend, and provider metadata contain no retired support;
- executable route and deposit rejection tests;
- remaining NOWPayments and Jibit provider tests;
- NOWPayments webhook freshness and replay tests, including Redis;
- clean target initialization evidence showing no provider seed or registry;
- full SEC-006 and prerequisite regressions.

No Payment4 mock or external service is contacted.
