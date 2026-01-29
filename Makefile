.PHONY: up down topics build

up:
	docker compose up --build

down:
	docker compose down -v

topics:
	sh tools/scripts/create-topics.sh

build:
	for s in order-service inventory-service payment-service notification-service; do \
		(cd services/$$s && go build ./...); \
	done
