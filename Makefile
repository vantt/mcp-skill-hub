GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
# Lint only issues introduced after this revision; existing findings are tracked debt.
LINT_BASE ?= origin/main

.PHONY: test test-race test-perf lint lint-all fmt fmt-check vet check web-install web-build web-test web-check web-e2e web-ux web-dev

## test: full suite, same as CI
test:
	go test -count=1 ./...

## test-race: full suite with the race detector
test-race:
	go test -race -count=1 ./...

## test-perf: timing-budget tests, run serially as in CI
test-perf:
	SKILLHUB_PERF=1 go test -p 1 -count=1 -run 'Performance' ./...

## lint: new issues since LINT_BASE (override: make lint LINT_BASE=<rev>)
lint:
	$(GOLANGCI_LINT) run --new-from-rev=$(LINT_BASE) ./...

## lint-all: every issue, including existing debt
lint-all:
	$(GOLANGCI_LINT) run --max-issues-per-linter=0 --max-same-issues=0 ./...

GO_DIRS := cmd internal schemas

fmt:
	gofmt -w $(GO_DIRS)

## fmt-check: fail when any Go file is not gofmt-formatted, same as CI
fmt-check:
	@unformatted="$$(gofmt -l $(GO_DIRS))"; \
	if [ -n "$$unformatted" ]; then echo "gofmt needed (run make fmt):"; echo "$$unformatted"; exit 1; fi

vet:
	go vet ./...

## check: what to run before committing
check: fmt-check vet lint test

web-install:
	cd web && npm ci

web-build: web-install
	find internal/delivery/web/dist -mindepth 1 ! -name .gitkeep -exec rm -rf {} +
	cd web && npm run build

web-test:
	cd web && npm run typecheck && npm run lint && npm test

## web-check: frontend typecheck, lint, unit tests and build
web-check: web-install web-test web-build

## web-e2e: build the UI and binary, then run Playwright against the real server
web-e2e: web-build
	go build -o web/.e2e/skillhub ./cmd/skillhub
	cd web && npx playwright install chromium && npm run e2e

## web-ux: capture every web flow (screenshots, text, axe) on a hub clone with isolated HOME/XDG
## UX_HUB_SOURCE: Git workspace to clone (default ~/skill-hub, only read); empty uses a small fixture.
UX_HUB_SOURCE ?= $(if $(wildcard $(HOME)/skill-hub/.git),$(HOME)/skill-hub,)
UX_OUT_DIR ?= $(CURDIR)/web/test-results/ux
web-ux: web-build
	go build -o web/.e2e/skillhub ./cmd/skillhub
	cd web && npx playwright install chromium && UX_CAPTURE=1 UX_HUB_SOURCE='$(UX_HUB_SOURCE)' UX_OUT_DIR='$(UX_OUT_DIR)' npx playwright test --project=ux
	@echo "UX capture output: $(UX_OUT_DIR)"
	@ls $(UX_OUT_DIR) | grep -c '\.png$$' | xargs -I{} echo "{} screenshots"

web-dev:
	bash scripts/web-dev.sh
