GOCMD      ?= go
NPM        ?= npm
LINT        = golangci-lint
BINARY      = build/sentinel
ENTRY       = ./cmd/sentinel
CLIENT      = client
DOCS_CHECK  = ./scripts/docs-check.sh
LINT_GOCACHE ?= /tmp/go-cache
LINT_CACHE   ?= /tmp/golangci-lint-cache

.DEFAULT_GOAL := help

# ─── Development ──────────────────────────────────────────────

.PHONY: run
run: check-go build-client ## Run Sentinel server (go run)
	$(GOCMD) run $(ENTRY)

.PHONY: dev
dev: check-go check-npm ## Run Go server + Vite dev server concurrently
	@test -d $(CLIENT)/node_modules || $(NPM) --prefix $(CLIENT) install
	@$(GOCMD) run $(ENTRY) & GO_PID=$$!; \
	$(NPM) --prefix $(CLIENT) run dev & NPM_PID=$$!; \
	trap 'kill $$GO_PID $$NPM_PID 2>/dev/null; wait' INT TERM; \
	wait

.PHONY: dev-client
dev-client: check-npm ## Run Vite dev server only (proxy to :4040)
	@test -d $(CLIENT)/node_modules || $(NPM) --prefix $(CLIENT) install
	$(NPM) --prefix $(CLIENT) run dev

# ─── Build ────────────────────────────────────────────────────

.PHONY: build
build: build-server ## Build frontend + Go binary

.PHONY: build-server
build-server: check-go build-client ## Compile Go binary to build/sentinel
	@mkdir -p build
	$(GOCMD) build -o $(BINARY) $(ENTRY)

.PHONY: build-client
build-client: check-npm ## Build frontend to client/dist/assets
	@test -d $(CLIENT)/node_modules || $(NPM) --prefix $(CLIENT) install
	$(NPM) --prefix $(CLIENT) run build

.PHONY: client-install
client-install: check-npm ## Install frontend dependencies
	$(NPM) --prefix $(CLIENT) install

# ─── Quality ──────────────────────────────────────────────────

.PHONY: test
test: check-go ## Run Go tests
	$(GOCMD) test ./...

.PHONY: test-unit
test-unit: check-go check-npm ## Run fast unit test layer (Go + client)
	$(GOCMD) test ./...
	@test -d $(CLIENT)/node_modules || $(NPM) --prefix $(CLIENT) install
	$(NPM) --prefix $(CLIENT) run test:unit

.PHONY: test-contract
test-contract: check-go ## Run API contract tests
	$(GOCMD) test -tags=contract -run '^TestContract' ./...

.PHONY: test-integration
test-integration: check-go ## Run integration test layer
	$(GOCMD) test -tags=integration -run '^TestIntegration' ./...

.PHONY: test-client
test-client: check-npm ## Run frontend tests
	@test -d $(CLIENT)/node_modules || $(NPM) --prefix $(CLIENT) install
	$(NPM) --prefix $(CLIENT) test

.PHONY: test-e2e
test-e2e: check-npm ## Run frontend end-to-end component flows
	@test -d $(CLIENT)/node_modules || $(NPM) --prefix $(CLIENT) install
	$(NPM) --prefix $(CLIENT) run test:e2e

.PHONY: test-coverage
test-coverage: check-go ## Run tests with race detection and coverage
	$(GOCMD) test -race -covermode=atomic -coverprofile=coverage.txt ./...

.PHONY: benchmark
benchmark: check-go ## Run Go benchmarks
	$(GOCMD) test -run=^$$ -bench=. -benchmem ./...

.PHONY: test-perf
test-perf: benchmark ## Run performance benchmark suite

.PHONY: fmt
fmt: check-go check-lint ## Format Go code
	GOCACHE=$(LINT_GOCACHE) GOLANGCI_LINT_CACHE=$(LINT_CACHE) $(LINT) fmt

.PHONY: lint
lint: check-go check-lint ## Lint Go code
	GOCACHE=$(LINT_GOCACHE) GOLANGCI_LINT_CACHE=$(LINT_CACHE) $(LINT) run

.PHONY: lint-client
lint-client: check-npm ## Lint frontend code
	@test -d $(CLIENT)/node_modules || $(NPM) --prefix $(CLIENT) install
	$(NPM) --prefix $(CLIENT) run lint

.PHONY: tidy
tidy: check-go ## Tidy go.mod and go.sum
	$(GOCMD) mod tidy

.PHONY: vuln
vuln: check-go ## Run vulnerability scanner
	govulncheck ./...

.PHONY: check-docs
check-docs: ## Validate docs navigation and file references
	$(DOCS_CHECK)

.PHONY: ci-fast
ci-fast: fmt lint lint-client test-unit test-contract build-server check-docs ## Fast PR gate

.PHONY: ci-full
ci-full: ci-fast test-integration test-e2e test-coverage test-perf ## Full mainline gate

.PHONY: ci
ci: ci-full ## Run full CI pipeline

# ─── Install ─────────────────────────────────────────────────

PREFIX     ?= $(HOME)/.local
BINDIR      = $(PREFIX)/bin
SERVICEDIR  = $(HOME)/.config/systemd/user

.PHONY: install
install: build ## Install binary and systemd user service
	install -Dm755 $(BINARY) $(BINDIR)/sentinel
	@echo "Installed sentinel to $(BINDIR)/sentinel"
	@if [ "$$(uname)" = "Linux" ] && command -v systemctl >/dev/null 2>&1; then \
		mkdir -p $(SERVICEDIR); \
		sed 's|ExecStart=.*|ExecStart=$(BINDIR)/sentinel|' contrib/sentinel.service \
			> $(SERVICEDIR)/sentinel.service; \
		if systemctl --user daemon-reload; then \
			echo "systemd user service installed."; \
			echo "  Start:   systemctl --user start sentinel"; \
			echo "  Enable:  systemctl --user enable sentinel"; \
			echo "  Logs:    journalctl --user -u sentinel -f"; \
		else \
			echo "warning: failed to run 'systemctl --user daemon-reload' (no active user bus?)"; \
			echo "service file written to $(SERVICEDIR)/sentinel.service"; \
		fi; \
	fi

.PHONY: uninstall
uninstall: ## Remove binary and systemd user service
	-systemctl --user disable --now sentinel 2>/dev/null
	rm -f $(BINDIR)/sentinel
	rm -f $(SERVICEDIR)/sentinel.service
	@if command -v systemctl >/dev/null 2>&1; then \
		systemctl --user daemon-reload 2>/dev/null; \
	fi
	@echo "Sentinel uninstalled."

# ─── Maintenance ──────────────────────────────────────────────

.PHONY: clean
clean: ## Remove build artifacts
	$(GOCMD) clean
	rm -rf build coverage.txt

.PHONY: help
help: ## Show available targets
	@awk '\
		/^# ─── / { printf "\n\033[1m%s\033[0m\n", substr($$0, 7) } \
		/^[a-zA-Z_-]+:.*## / { \
			target = $$0; \
			sub(/:.*/, "", target); \
			desc = $$0; \
			sub(/.*## /, "", desc); \
			printf "  \033[36m%-18s\033[0m %s\n", target, desc; \
		}' $(MAKEFILE_LIST)
	@echo

# Dependency checks (internal)

.PHONY: check-go check-npm check-lint

check-go:
	@command -v $(GOCMD) >/dev/null 2>&1 || { echo "error: go is not installed — https://golang.org/doc/install"; exit 1; }

check-npm:
	@command -v $(NPM) >/dev/null 2>&1 || { echo "error: npm is not installed — https://nodejs.org"; exit 1; }

check-lint:
	@command -v $(LINT) >/dev/null 2>&1 || { echo "error: $(LINT) is not installed — go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; exit 1; }
