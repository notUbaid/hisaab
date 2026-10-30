.PHONY: up down migrate migrate-down migrate-status build test test-race test-property \
        test-invariants test-scenarios test-crash bench lint fmt vet check-float clean \
        lint-console typecheck-console

DB_URL      ?= postgres://hisaab:hisaab_dev@localhost:5432/hisaab
TEST_DB_URL ?= postgres://hisaab:hisaab_dev@localhost:5432/hisaab

# ─── Infrastructure ─────────────────────────────────────────────────────────
up:
	docker compose up -d
	@echo "Waiting for Postgres..."
	@until docker compose exec postgres pg_isready -U hisaab 2>/dev/null; do sleep 1; done
	$(MAKE) migrate

down:
	docker compose down

# ─── Database ───────────────────────────────────────────────────────────────
migrate:
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$(DB_URL)" goose -dir db/migrations up

migrate-down:
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$(DB_URL)" goose -dir db/migrations down

migrate-status:
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$(DB_URL)" goose -dir db/migrations status

# ─── Build Binaries ─────────────────────────────────────────────────────────
build:
	go build -o bin/hisaab-api.exe ./cmd/api
	go build -o bin/hisaab-worker.exe ./cmd/worker
	go build -o bin/hisaab-cli.exe ./cmd/cli

# ─── Tests ──────────────────────────────────────────────────────────────────
test:
	go test -count=1 -timeout=120s ./...

test-race:
	go test -race -count=1 -timeout=120s ./...

test-property:
	go test -run "^TestProperty" -rapid.checks=5000 -timeout=120s ./tests/property/...

test-invariants:
	go test -run "^TestInvariant" -v -timeout=120s ./internal/verification/...

test-scenarios:
	go test -run "^TestScenario" -v -timeout=120s ./tests/scenarios/...

test-crash:
	go test -run "^TestCrashPoint" -v -timeout=120s ./tests/crash/...

bench:
	go test ./tests/bench/... -bench=. -benchmem -run='^$$' -count=3

# ─── Code Quality & Precision ───────────────────────────────────────────────
lint:
	golangci-lint run --config=.golangci.yml ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./

check-float:
	@echo "Checking for floats in monetary code..."
	@FOUND=$$(grep -rn "float32\|float64" internal/ledger/ internal/settlement/ internal/recon/ \
	  --include="*.go" 2>/dev/null | grep -v "_test.go" | grep -v "// RAMP-ONLY" || true); \
	if [ -n "$$FOUND" ]; then \
	  echo "CRITICAL: float in monetary code:"; echo "$$FOUND"; exit 1; \
	fi; \
	echo "✓ No floats in monetary code"

clean:
	rm -rf bin/

lint-console:
	cd console && npm run lint

typecheck-console:
	cd console && npx tsc --noEmit
