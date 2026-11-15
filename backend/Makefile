# Makefile — backend coffeely (веха M1).
#
# make dev-up     — поднять Postgres (deploy/dev)
# make dev-down   — остановить Postgres
# make migrate    — поднять миграции
# make migrate-down — откатить одну миграцию
# make run        — запустить сервер (PORT=8080)
# make build      — собрать bin/server
# make test       — go test ./...

DATABASE_URL ?= postgres://coffeely:coffeely@localhost:5432/coffeely?sslmode=disable
export DATABASE_URL

.PHONY: dev-up dev-down migrate migrate-down run build test

dev-up:
	cd ../deploy/dev && ([ -f .env ] || cp .env.example .env); \
	docker compose --env-file .env up -d && \
	docker compose --env-file .env exec db sh -c 'until pg_isready -U $${POSTGRES_USER} >/dev/null 2>&1; do sleep 1; done' && \
	echo "Postgres готов на localhost:$${POSTGRES_PORT:-5432}"

dev-down:
	cd ../deploy/dev && docker compose --env-file .env down

migrate:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...
