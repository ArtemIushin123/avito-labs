ifneq (,$(wildcard .env.example))
	include .env.example
endif
ifneq (,$(wildcard .env))
	include .env
endif
export

COVER_PKGS := ./internal/usecase/...,./internal/domain/...

.PHONY: run build test test-integration lint generate migrate migrate-down migrate-reset

run:
	go run ./cmd/service

build:
	go build -ldflags "-X main.version=$(shell git describe --tags --always)" -o bin/trip-service ./cmd/service

test:
	go test -race -count=1 -coverpkg=$(COVER_PKGS) -coverprofile=coverage.out ./...
	@echo "Покрытие usecase + domain:"
	@go tool cover -func=coverage.out | tail -n 1

test-integration:
	go test -race -count=1 -tags=integration ./...

lint:
	golangci-lint run ./...

generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-package api \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml
	go generate ./...

migrate:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" down

migrate-reset:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" reset