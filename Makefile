GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
# Lint only issues introduced after this revision; existing findings are tracked debt.
LINT_BASE ?= origin/main

.PHONY: test test-race test-perf lint lint-all fmt vet check

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

fmt:
	gofmt -w cmd internal

vet:
	go vet ./...

## check: what to run before committing
check: vet lint test
