.PHONY: dev build test fmt vet templ sqlc migrate docker-build

GOCACHE ?= /tmp/koalabye-go-cache
TEMPL_VERSION := v0.3.960
SQLC_VERSION := v1.29.0
GOOSE_VERSION := v3.26.0

dev: templ
	go run ./cmd/koalabye

build: templ
	go build -trimpath -o koalabye ./cmd/koalabye

test: templ
	go test ./...

fmt:
	go fmt ./...
	go run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION) fmt .

vet: templ
	go vet ./...

templ:
	go run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION) generate

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

migrate:
	go run github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION) -dir migrations sqlite3 "$${KOALABYE_DATABASE_PATH:-./data/koalabye.db}" up

docker-build:
	docker build -t koalabye:local .
