GOCMD ?= go
NPM   ?= npm
LINT   = golangci-lint

APP      := sentinel
BIN_DIR  := bin
BIN      := $(BIN_DIR)/$(APP)
ENTRY    := ./cmd/sentinel
PKG_LIST := ./...
FRONTEND := frontend
DOCS_CHECK := ./scripts/docs-check.sh
WEB_URL    := http://127.0.0.1:4040

# System scope under root, user scope otherwise (see CLAUDE.md).
PREFIX        ?= $(shell [ "$$(id -u)" -eq 0 ] && echo /usr/local || echo "$(HOME)/.local")
BINDIR         = $(PREFIX)/bin
INSTALLED_BIN  = $(BINDIR)/$(APP)
XDG_CONFIG_HOME ?= $(HOME)/.config

VERSION ?= dev
LDFLAGS ?= -s -w -X github.com/opus-domini/sentinel/pkg/sentinel.version=$(VERSION)
COVERAGE_PROFILE ?= coverage.txt
COVERAGE_PKGS    ?= ./...
COVERAGE_CHECK    = ./scripts/coverage-check.sh
COVERAGE_MIN     ?= 80

.DEFAULT_GOAL := help

# --- Development -----------------------------------------------

.PHONY: run
run: check-go build-frontend ## Run the server with go run
	$(GOCMD) run $(ENTRY)

.PHONY: dev
dev: check-go check-npm ## Run Go server and Vite dev server concurrently
	@test -d "$(FRONTEND)/node_modules" || $(NPM) --prefix "$(FRONTEND)" install
	@$(GOCMD) run $(ENTRY) & GO_PID=$$!; \
	$(NPM) --prefix "$(FRONTEND)" run dev & NPM_PID=$$!; \
	trap 'kill $$GO_PID $$NPM_PID 2>/dev/null; wait' INT TERM; \
	wait

.PHONY: dev-frontend
dev-frontend: check-npm ## Run the Vite dev server only
	@test -d "$(FRONTEND)/node_modules" || $(NPM) --prefix "$(FRONTEND)" install
	$(NPM) --prefix "$(FRONTEND)" run dev

# --- Build -----------------------------------------------------

.PHONY: all
all: build ## Build the frontend and binary

.PHONY: build
build: build-server ## Build the frontend and binary

.PHONY: build-server
build-server: check-go build-frontend ## Build the binary into $(BIN)
	@mkdir -p "$(BIN_DIR)"
	$(GOCMD) build -trimpath -ldflags="$(LDFLAGS)" -o "$(BIN)" $(ENTRY)

.PHONY: build-frontend
build-frontend: frontend-install ## Build embedded frontend assets
	$(NPM) --prefix "$(FRONTEND)" run build

.PHONY: frontend-install
frontend-install: check-npm ## Install frontend dependencies reproducibly
	$(NPM) ci --prefix "$(FRONTEND)"

# --- Quality ---------------------------------------------------

.PHONY: test
test: check-go ## Run Go tests with race detection and shuffle
	$(GOCMD) test -race -shuffle=on $(PKG_LIST)

.PHONY: test-unit
test-unit: check-go check-npm ## Run fast unit test layer (Go + frontend)
	$(GOCMD) test $(PKG_LIST)
	@test -d "$(FRONTEND)/node_modules" || $(NPM) --prefix "$(FRONTEND)" install
	$(NPM) --prefix "$(FRONTEND)" run test:unit

.PHONY: test-contract
test-contract: check-go ## Run API contract tests
	$(GOCMD) test -tags=contract -run '^TestContract' $(PKG_LIST)

.PHONY: test-integration
test-integration: check-go ## Run integration tests
	$(GOCMD) test -tags=integration -run '^TestIntegration' $(PKG_LIST)

.PHONY: test-coverage
test-coverage: check-go ## Run tests with race detection and the coverage gate
	$(GOCMD) test -race -shuffle=on -covermode=atomic -coverpkg=$(COVERAGE_PKGS) -coverprofile="$(COVERAGE_PROFILE)" $(PKG_LIST)
	COVERAGE_MIN=$(COVERAGE_MIN) $(COVERAGE_CHECK) "$(COVERAGE_PROFILE)"

.PHONY: test-cover
test-cover: test-coverage ## Alias for test-coverage

.PHONY: coverage-check
coverage-check: check-go ## Validate an existing coverage profile against COVERAGE_MIN
	COVERAGE_MIN=$(COVERAGE_MIN) $(COVERAGE_CHECK) "$(COVERAGE_PROFILE)"

.PHONY: test-frontend
test-frontend: check-npm ## Run frontend tests
	@test -d "$(FRONTEND)/node_modules" || $(NPM) --prefix "$(FRONTEND)" install
	$(NPM) --prefix "$(FRONTEND)" test

.PHONY: test-e2e
test-e2e: check-npm ## Run frontend end-to-end component flows
	@test -d "$(FRONTEND)/node_modules" || $(NPM) --prefix "$(FRONTEND)" install
	$(NPM) --prefix "$(FRONTEND)" run test:e2e

.PHONY: benchmark
benchmark: check-go ## Run Go benchmarks
	$(GOCMD) test -run=^$$ -bench=. -benchmem $(PKG_LIST)

.PHONY: test-perf
test-perf: benchmark ## Run performance benchmark suite

.PHONY: fmt
fmt: check-go check-lint ## Format Go code
	$(LINT) fmt

.PHONY: fmt-check
fmt-check: check-go ## Verify Go files are gofmt-clean
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
	  echo "These files are not gofmt-clean:"; \
	  echo "$$unformatted"; \
	  exit 1; \
	fi

.PHONY: vet
vet: check-go ## Run go vet
	$(GOCMD) vet $(PKG_LIST)

.PHONY: lint
lint: check-go check-lint ## Run golangci-lint
	$(LINT) run

.PHONY: lint-frontend
lint-frontend: check-npm ## Lint frontend code
	@test -d "$(FRONTEND)/node_modules" || $(NPM) --prefix "$(FRONTEND)" install
	$(NPM) --prefix "$(FRONTEND)" run lint

.PHONY: typecheck-frontend
typecheck-frontend: check-npm ## Typecheck frontend code
	@test -d "$(FRONTEND)/node_modules" || $(NPM) --prefix "$(FRONTEND)" install
	$(NPM) --prefix "$(FRONTEND)" run typecheck

.PHONY: tidy
tidy: check-go ## Tidy go.mod and go.sum
	$(GOCMD) mod tidy

.PHONY: tidy-check
tidy-check: check-go ## Verify go.mod and go.sum are tidy
	$(GOCMD) mod tidy
	git diff --exit-code go.mod go.sum

.PHONY: vuln
vuln: check-go check-govulncheck ## Run vulnerability scanner
	govulncheck $(PKG_LIST)

.PHONY: docs-check
docs-check: ## Validate docs navigation and file references
	$(DOCS_CHECK)

.PHONY: smoke-frontend-terminal
smoke-frontend-terminal: check-go check-npm ## Run browser smoke for tmux terminal rendering
	@test -d "$(FRONTEND)/node_modules" || $(NPM) --prefix "$(FRONTEND)" install
	./scripts/frontend-terminal-smoke.sh

.PHONY: smoke-frontend-terminal-soak
smoke-frontend-terminal-soak: check-go check-npm ## Run heavier browser soak for tmux terminal rendering
	@test -d "$(FRONTEND)/node_modules" || $(NPM) --prefix "$(FRONTEND)" install
	SENTINEL_SMOKE_INITIAL_LINES=4000 SENTINEL_SMOKE_LIVE_LINES=12000 ./scripts/frontend-terminal-smoke.sh

.PHONY: ci-fast
ci-fast: tidy-check fmt-check vet lint lint-frontend typecheck-frontend test-coverage docs-check build ## Fast CI gate for pull requests

.PHONY: ci-full
ci-full: ci-fast test-frontend test-contract test-integration test-e2e test-perf vuln ## Full CI gate for mainline

.PHONY: ci
ci: ci-full ## Run the full CI pipeline

.PHONY: check
check: fmt tidy vet lint lint-frontend typecheck-frontend test-coverage test-frontend vuln docs-check ## Run local validation and apply formatting

# --- Install ---------------------------------------------------

.PHONY: install
install: install-binary install-service install-completion ## Install binary, service and shell completion (user scope; system scope when run as root)
	@printf '\n  \033[1;32m✓\033[0m \033[1m%s installed\033[0m\n\n' "$(APP)"
	@printf '  \033[36m%-12s\033[0m %s\n' 'Binary' "$(INSTALLED_BIN)"
	@printf '  \033[36m%-12s\033[0m %s\n' 'Service' "$(APP).service"
	@printf '  \033[36m%-12s\033[0m %s\n' 'Completion' 'shell completion (best effort)'
	@printf '  \033[36m%-12s\033[0m \033[1;36m%s\033[0m\n' 'Web UI' "$(WEB_URL)"
	@printf '\n  \033[1mNext steps\033[0m\n'
	@printf '    \033[2m1.\033[0m %-28s\033[2m%s\033[0m\n' "$(APP) doctor" 'verify the environment'
	@printf '    \033[2m2.\033[0m %-28s\033[2m%s\033[0m\n' "$(APP) service status" 'check the service'
	@printf '    \033[2m3.\033[0m %-28s\033[2m%s\033[0m\n' "open $(WEB_URL)" 'launch the web UI'
	@printf '\n'

.PHONY: install-binary
install-binary: build ## Install the binary
	install -Dm755 "$(BIN)" "$(INSTALLED_BIN)"
	@echo "Installed $(APP) to $(INSTALLED_BIN)"

.PHONY: install-completion
install-completion: install-binary ## Install shell completion for the installed binary
	@"$(INSTALLED_BIN)" completion install --shell auto \
		&& echo "Shell completion installed (open a new shell to use it)" || true

.PHONY: install-service
install-service: install-binary ## Install and restart the service (system scope when run as root)
	"$(INSTALLED_BIN)" service install --exec "$(INSTALLED_BIN)"

.PHONY: uninstall
uninstall: ## Remove service, binary and shell completion
	-"$(INSTALLED_BIN)" service uninstall --purge
	rm -f "$(HOME)/.local/share/bash-completion/completions/$(APP)"
	rm -f "$(HOME)/.local/share/zsh/site-functions/_$(APP)"
	rm -f "$(XDG_CONFIG_HOME)/fish/completions/$(APP).fish"
	rm -f "$(INSTALLED_BIN)"
	@echo "Sentinel uninstalled."

# --- Release ---------------------------------------------------

# Real releases go through .github/workflows/release-please.yml only: the
# workflow runs the ci-full gate against the tagged commit, pins GoReleaser,
# and signs artifacts with keyless cosign via GitHub OIDC. Use
# release-snapshot/release-check for local validation.
.PHONY: release
release:
	@printf '%s\n' 'Releases are published by the Release Please workflow (gated, pinned, cosign-signed).' \
		'Run it from the GitHub Actions tab; use `make release-snapshot` or `make release-check` locally.'
	@exit 1

.PHONY: release-check
release-check: check-goreleaser ## Validate the GoReleaser config
	goreleaser check

.PHONY: release-snapshot
release-snapshot: check-goreleaser ## Build a local snapshot release (no publish)
	goreleaser release --snapshot --clean

# --- Maintenance -----------------------------------------------

.PHONY: clean
clean: ## Remove build, frontend and coverage artifacts
	$(GOCMD) clean
	rm -rf "$(BIN_DIR)" dist "$(COVERAGE_PROFILE)" "$(FRONTEND)/dist"
	find internal/ui/dist -mindepth 1 -maxdepth 1 ! -name .gitkeep -exec rm -rf {} +

.PHONY: help
help: ## Show available targets
	@awk '\
		/^# --- / { printf "\n\033[1m%s\033[0m\n", substr($$0, 7) } \
		/^[a-zA-Z_-]+:.*## / { \
			target = $$0; \
			sub(/:.*/, "", target); \
			desc = $$0; \
			sub(/.*## /, "", desc); \
			printf "  \033[36m%-24s\033[0m %s\n", target, desc; \
		}' $(MAKEFILE_LIST)
	@echo

# Dependency checks (internal)

.PHONY: check-go check-npm check-lint check-govulncheck check-goreleaser

check-go:
	@command -v $(GOCMD) >/dev/null 2>&1 || { echo "error: go is not installed - https://go.dev/doc/install"; exit 1; }

check-npm:
	@command -v $(NPM) >/dev/null 2>&1 || { echo "error: npm is not installed - https://nodejs.org"; exit 1; }

check-lint:
	@command -v $(LINT) >/dev/null 2>&1 || { echo "error: $(LINT) is not installed - go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; exit 1; }

check-govulncheck:
	@command -v govulncheck >/dev/null 2>&1 || { echo "error: govulncheck is not installed - go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 1; }

check-goreleaser:
	@command -v goreleaser >/dev/null 2>&1 || { echo "error: goreleaser is not installed - https://goreleaser.com/install"; exit 1; }
