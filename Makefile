.PHONY: up down migrate migrate-down migrate-status test test-race test-property \
        test-invariants test-scenarios test-crash test-browser bench lint install-hooks check-real-data \
        check-spec-coverage sqlc seed-calendar verify-merkle security-scan setup \
        fmt vet check-float generate-report check-spec-drift

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

sqlc:
	sqlc generate

# ─── Tests ──────────────────────────────────────────────────────────────────
test:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test -count=1 -timeout=120s ./...

test-race:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test -race -count=1 -timeout=120s ./...

test-property:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test -run "^TestProperty" \
	  -rapid.checks=50000 -timeout=300s ./tests/property/...

test-invariants:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test -run "^TestInvariant" \
	  -v -timeout=300s ./tests/...

test-scenarios:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test -run "^TestScenario" \
	  -v -timeout=300s ./tests/scenarios/...

test-crash:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test -run "^TestCrashPoint" \
	  -v -timeout=300s ./tests/crash/...

test-browser:
	python scripts/physical_browser_check.py

bench:
	HISAAB_TEST_DB="$(TEST_DB_URL)" go test ./tests/bench/... \
	  -bench=. -benchmem -run='^$$' -count=3

# ─── Code quality ───────────────────────────────────────────────────────────
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

# ─── Security & compliance ──────────────────────────────────────────────────
security-scan:
	@echo "=== gosec ===" && gosec ./...
	@echo "=== real data check ===" && python3 scripts/check_real_data.py
	@echo "=== spec coverage ===" && python3 scripts/check_spec_coverage.py

check-real-data:
	python3 scripts/check_real_data.py

check-spec-coverage:
	python3 scripts/check_spec_coverage.py

check-spec-drift:
	python3 scripts/check_ondc_spec_drift.py

bin/cli.exe: cmd/cli/main.go
	go build -o bin/cli.exe ./cmd/cli/main.go

generate-report: bin/cli.exe
	./bin/cli.exe generate-report --output docs/CORRECTNESS-REPORT.md

# ─── Developer setup ────────────────────────────────────────────────────────
install-hooks:
	bash scripts/install-hooks.sh

setup: install-hooks
	@echo ""
	@echo "✓ Git hooks installed"
	@echo "  Next: docker compose up -d && make migrate"
	@echo "  Then: make test-race"
	@echo ""
	@echo "  Read CLAUDE.md before writing any code."

# ─── CLI commands ───────────────────────────────────────────────────────────
seed-calendar: bin/cli.exe
	./bin/cli.exe seed-calendar --year 2026

verify-merkle: bin/cli.exe
	@[ "$(BATCH_ID)" ] || (echo "Usage: make verify-merkle BATCH_ID=<id>"; exit 1)
	./bin/cli.exe verify-merkle --batch-id $(BATCH_ID)
