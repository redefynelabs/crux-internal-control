.PHONY: dev-up dev-down dev-template run build test lint fmt web-dev web-build web-test web-lint web-typecheck docker-build

# Loads .env (if present) for run. Copy .env.example to .env first.
ENV_FILE := .env

run:
	@test -f $(ENV_FILE) || (echo "missing $(ENV_FILE): cp .env.example .env"; exit 1)
	set -a; . ./$(ENV_FILE); set +a; go run ./cmd/controlplane

build:
	go build -o bin/controlplane ./cmd/controlplane

# Needs a Postgres the tests can create databases in.
# usage: CP_TEST_DATABASE_URL='postgres://user@localhost:5432/postgres?sslmode=disable' make test
test:
	go test ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)

fmt:
	gofmt -w .

docker-build:
	docker build -t erp-controlplane .

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

web-test:
	cd web && npm test

web-lint:
	cd web && npm run lint

web-typecheck:
	cd web && npm run typecheck

# A whole demo host on a laptop: Postgres, Redis and Traefik in Docker (see dev/docker-compose.yml). Build the images
# first (local/erp-server:local, local/erp-web:local) and set .env as in README "Try it on a laptop".
dev-up:
	docker compose -f dev/docker-compose.yml up -d --wait

dev-down:
	docker compose -f dev/docker-compose.yml down

# Builds erp_demo_seed from the ERP repo. Needs the erp_demo role: see README.
dev-template:
	../erp-software/ops/demo/build-template.sh postgres://postgres:postgres@localhost:5434/postgres?sslmode=disable
