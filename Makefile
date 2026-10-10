SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.ONESHELL:
.DEFAULT_GOAL := help

ENV_FILE ?= .env
MIGRATIONS_DIR ?= migrations
ARGS ?=
NAME ?=
GOCACHE ?= /tmp/payment-service-go-cache
# Only explicit make arguments override .env; inherited shell values do not.
ENV_OVERRIDE_KEYS := $(strip $(foreach key,$(.VARIABLES),$(if $(filter command line,$(origin $(key))),$(key))))
export ENV_FILE NAME GOCACHE ENV_OVERRIDE_KEYS

WITH_ENV := bash scripts/with-env.sh

# Use DATABASE_URL from the loaded environment without printing credentials.
define run_goose
@$(WITH_ENV) bash -eu -o pipefail -c \
	'export GOOSE_DRIVER=postgres GOOSE_DBSTRING="$${DATABASE_URL:?DATABASE_URL harus diisi di .env}"; exec goose -env= -dir "$$1" "$$2"' \
	-- "$(MIGRATIONS_DIR)" "$(1)"
endef

.PHONY: help dev run build deps fmt vet test test-race test-local channels sandbox sandbox-setup \
	db-up db-down db-logs migrate-up migrate-down migrate-status migrate-version \
	migrate-create migrate-validate

help: ## Tampilkan perintah development
	@awk 'BEGIN { print "Perintah development:" } /^[a-zA-Z0-9_-]+:.*## / { split($$0, parts, ":.*## "); printf "  %-20s %s\n", parts[1], parts[2] }' $(MAKEFILE_LIST)

dev: ## Jalankan PostgreSQL lokal, migrasi, lalu API
	@$(MAKE) db-up
	$(MAKE) migrate-up
	$(MAKE) run

run: ## Jalankan API dengan environment dari .env
	@$(WITH_ENV) go run ./cmd/api $(ARGS)

build: ## Build binary API ke bin/payment-api
	@mkdir -p bin
	$(WITH_ENV) go build -o bin/payment-api ./cmd/api

deps: ## Unduh dependency Go
	@$(WITH_ENV) go mod download

fmt: ## Format kode Go
	@$(WITH_ENV) go fmt ./...

vet: ## Jalankan pemeriksaan go vet
	@$(WITH_ENV) go vet ./...

test: ## Jalankan unit test dengan coverage
	@$(WITH_ENV) go test -cover ./... $(ARGS)

test-race: ## Jalankan unit test dengan race detector (butuh compiler C)
	@$(WITH_ENV) env CGO_ENABLED=1 go test -race -cover ./... $(ARGS)

test-local: ## Jalankan unit dan integration test pada database terpisah
	@$(WITH_ENV) env CGO_ENABLED=1 bash scripts/test-payment.sh local $(ARGS)

channels: ## Tampilkan katalog channel pembayaran
	@$(WITH_ENV) go run ./cmd/payment-test -list

sandbox-setup: ## Siapkan API dan tunnel untuk webhook Xendit TEST mode
	@$(WITH_ENV) bash scripts/test-payment.sh sandbox-setup $(ARGS)

sandbox: ## Jalankan pengujian Xendit TEST mode
	@$(WITH_ENV) bash scripts/test-payment.sh sandbox $(ARGS)

db-up: ## Jalankan PostgreSQL lokal dan tunggu sampai siap
	@$(WITH_ENV) docker compose up -d postgres
	$(WITH_ENV) docker compose exec -T postgres sh -c \
		'for attempt in $$(seq 1 30); do pg_isready -h 127.0.0.1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" >/dev/null 2>&1 && exit 0; sleep 1; done; printf "PostgreSQL belum siap setelah 30 detik.\n" >&2; exit 1'

db-down: ## Hentikan layanan Compose tanpa menghapus volume database
	@$(WITH_ENV) docker compose down

db-logs: ## Ikuti log PostgreSQL lokal
	@$(WITH_ENV) docker compose logs -f postgres

migrate-up: ## Terapkan semua migrasi goose yang belum dijalankan
	$(call run_goose,up)

migrate-down: ## Rollback satu migrasi goose terakhir
	$(call run_goose,down)

migrate-status: ## Tampilkan status migrasi goose
	$(call run_goose,status)

migrate-version: ## Tampilkan versi migrasi database
	$(call run_goose,version)

migrate-create: ## Buat migrasi SQL berurutan: make migrate-create NAME=add_table
	@$(WITH_ENV) bash -eu -o pipefail -c \
		'if [[ ! "$${NAME:-}" =~ ^[a-z][a-z0-9_]*$$ ]]; then printf "Gunakan NAME dengan huruf kecil, angka, dan underscore; contoh: make migrate-create NAME=add_table\n" >&2; exit 2; fi; exec goose -env= -dir "$$1" -s create "$$NAME" sql' \
		-- "$(MIGRATIONS_DIR)"

migrate-validate: ## Validasi file migrasi tanpa mengubah database
	@$(WITH_ENV) goose -env= -dir "$(MIGRATIONS_DIR)" validate
