#!/usr/bin/env sh
set -e
docker compose exec -T redpanda rpk topic create   redstone.orders redstone.inventory redstone.payments redstone.notifications   -p 3 || true
echo "Topics ready."
