# Architecture — Redstone

## Tiered layout
1. Presentation (future): web/app UI
2. Edge: API gateway (future; compose exposes services directly for simplicity)
3. Service tier: microservices (order, inventory, payment, notification)
4. Data tier: Postgres per domain, Redis cache
5. Async tier: Kafka-compatible event broker (Redpanda)
6. Ops tier: logs/traces/metrics

## Saga (choreography)
OrderService emits `OrderCreated`
→ InventoryService reserves stock and emits `InventoryReserved` or `InventoryFailed`
→ PaymentService captures and emits `PaymentCaptured` or `PaymentFailed`
→ OrderService consumes and updates order status + emits `OrderConfirmed` or `OrderCancelled`
→ NotificationService consumes and sends notifications

## Reliability patterns
- Transactional Outbox: write domain change + outbox row in the same DB tx
- At-least-once delivery: consumers must be idempotent
- Idempotency keys: create-order endpoint de-duplicates requests

## Data ownership
- order-service: orders DB schema (orders + order_items + order_events + outbox + idempotency)
- inventory-service: inventory DB schema (stock + reservations + outbox)

