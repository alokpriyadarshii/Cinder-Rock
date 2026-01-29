# Cinder Rock

---

set -euo pipefail

## 1) Go to project folder (adjust if you're already there)

---

cd "Cinder Rock"

## 2) Start stack in background

---

docker compose up --build -d

## 3) Wait until services are ready

---

HOST=127.0.0.1 ORDER="http://${HOST}:8081" INVENTORY="http://${HOST}:8082" PAYMENT="http://${HOST}:8083" NOTIFY="http://${HOST}:8084"
for S in "$ORDER" "$INVENTORY" "$PAYMENT" "$NOTIFY"; do until curl -sf "$S/healthz" >/dev/null; do sleep 0.2; done; done

## 4) Demo API calls

---

echo "== create order =="; curl -s -X POST "${ORDER}/v1/orders" -H 'Content-Type: application/json' -H 'Idempotency-Key: demo-123' -d '{"user_id":"u_100","items":[{"sku":"SKU-RED-1","qty":1,"unit_price":1999}]}' | python -m json.tool
echo "== replay (same key) =="; curl -s -X POST "${ORDER}/v1/orders" -H 'Content-Type: application/json' -H 'Idempotency-Key: demo-123' -d '{"user_id":"u_100","items":[{"sku":"SKU-RED-1","qty":1,"unit_price":1999}]}' | python -m json.tool

## 5) Watch saga logs

---

docker compose logs -f order-service inventory-service payment-service notification-service

## 6) Optional UIs

---

Keycloak: http://localhost:8080  (admin/admin)
Jaeger:   http://localhost:16686
Redpanda: http://localhost:9644

## 7) Stop + clean up

---

docker compose down -v

## 8) All commands at once (one paste)

---

cd "Cinder Rock" 2>/dev/null || true && docker compose up --build -d && HOST=127.0.0.1 ORDER="http://${HOST}:8081" INVENTORY="http://${HOST}:8082" PAYMENT="http://${HOST}:8083" NOTIFY="http://${HOST}:8084" && for S in "$ORDER"

