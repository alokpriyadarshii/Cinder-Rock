# Runbook — Redstone

## Common commands
- Start:
  - `docker compose up --build`
- Stop:
  - `docker compose down -v`
- Recreate topics:
  - `docker compose exec redpanda rpk topic create redstone.orders redstone.inventory redstone.payments redstone.notifications -p 3`

## Health checks
- order-service: `GET /healthz` on :8081
- inventory-service: `GET /healthz` on :8082
- payment-service: `GET /healthz` on :8083
- notification-service: `GET /healthz` on :8084

## Incident playbooks
### Order creation failing
1) Check order-service logs for DB errors
2) Confirm Postgres is up and `ordersdb` exists
3) Check Kafka broker connectivity (redpanda logs)

### Saga stuck (order not progressing)
1) Check consumer lag: `rpk group list` / `rpk group describe ...`
2) Verify topics exist
3) Look for poison message patterns; consumers are idempotent but may reject invalid events

## SLOs (project targets)
- Create order success rate > 99.9% in steady load tests
- p95 latency under 400ms locally is acceptable as baseline

