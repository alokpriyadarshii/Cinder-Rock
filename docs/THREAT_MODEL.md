# Threat Model (STRIDE-lite) — Redstone

## Assets
- Order and user identifiers (PII-light)
- Payment status (sensitive business data)
- Inventory counts

## Entry points
- Public HTTP APIs
- Kafka topics
- Admin endpoints (future)

## Threats & mitigations
- Spoofing: JWT verification (Keycloak), mTLS in-cluster (future)
- Tampering: schema validation + signed JWTs + least-privilege DB users
- Repudiation: append-only order_events table for audit
- Information disclosure: avoid storing PII, encrypt in transit (TLS), secrets management
- Denial of service: rate limiting at gateway, bounded queues, consumer backpressure
- Elevation of privilege: RBAC claims in JWT, admin endpoints separated

