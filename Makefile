ifneq (,$(wildcard .env.example))
	include .env.example
endif
ifneq (,$(wildcard .env))
	include .env
endif
export

.PHONY: run build test lint generate migrate migrate-down migrate-reset

run:
	go run ./cmd/trip-service

build:
	go build -o bin/trip-service ./cmd/trip-service

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-package api \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml

migrate:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" down

migrate-reset:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" reset