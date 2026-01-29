# PRD — Project Redstone

## Goal
Build a distributed multi-tier commerce/fulfillment platform demonstrating real industry patterns:
microservices + event streaming + reliability + security + observability.

## Key User Stories (MVP)
- Customer: create order, view order status timeline
- System: reserve inventory, capture payment, confirm order
- Customer: receive notification when order confirmed/failed

## Non-functional Requirements (targets)
- Availability: 99.9% for create-order path
- Latency: p95 < 400ms for create-order at steady load
- Resilience: retryable failures, idempotency, outbox for reliable events
- Security: JWT auth ready (Keycloak), rate limiting recommended at gateway
- Observability: structured logs, traces (optional), metrics endpoints

## Out of Scope (for MVP scaffold)
- Full UI/admin portal, search, promotions, shipping carriers
- PCI compliance (payment is mocked)

