.PHONY: dev build check test fmt fmt-check vet vulncheck templ templ-check sqlc sqlc-check migrate docker-build

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

fmt-check:
	@test -z "$$(gofmt -l cmd internal migrations templates web/static)" || (echo "Go files need formatting:"; gofmt -l cmd internal migrations templates web/static; exit 1)
	go run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION) fmt -fail .

vet: templ
	go vet ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

templ:
	go run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION) generate

templ-check:
	@tmp="$$(mktemp -d)"; cp templates/*_templ.go "$$tmp"/; \
	go run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION) generate; \
	status=0; \
	for file in templates/*_templ.go; do cmp -s "$$tmp/$$(basename "$$file")" "$$file" || { echo "$$file is out of date"; status=1; }; done; \
	rm -rf "$$tmp"; exit $$status

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

sqlc-check:
	@tmp="$$(mktemp -d)"; cp -R internal/db/dbgen "$$tmp"/; \
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate; \
	status=0; diff -ru "$$tmp/dbgen" internal/db/dbgen || status=1; \
	rm -rf "$$tmp"; exit $$status

migrate:
	go run github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION) -dir migrations sqlite3 "$${KOALABYE_DATABASE_PATH:-./data/koalabye.db}" up

docker-build:
	docker build --build-arg VERSION=dev --build-arg COMMIT=$$(git rev-parse --short HEAD) --build-arg BUILD_DATE=$$(date -u +%Y-%m-%dT%H:%M:%SZ) -t koalabye:local .

check: fmt-check templ-check sqlc-check test vet
