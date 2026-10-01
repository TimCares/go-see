# Pinned to the version CI uses. `go run` caches the binary, so only the first run builds it.
GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

.DEFAULT_GOAL := check

# Checks never modify files. They are what pre-commit and CI run.

.PHONY: check
check: check-format check-lint check-tidy test ## Run every check, same as CI.

.PHONY: check-format
check-format: ## Fail if a file is not formatted.
	$(GOLANGCI_LINT) fmt --diff

.PHONY: check-lint
check-lint: ## Fail on any lint issue.
	$(GOLANGCI_LINT) run

.PHONY: check-tidy
check-tidy: ## Fail if go.mod or go.sum is not tidy.
	go mod tidy -diff

.PHONY: test
test: ## Run the tests, including the examples, with the race detector.
	go test -race ./...

# Fixes modify files. They are only ever run by hand.

.PHONY: fix
fix: format lint tidy repo-fix ## Apply every fix.

.PHONY: format
format: ## Format every file.
	$(GOLANGCI_LINT) fmt

.PHONY: lint
lint: ## Apply every lint fix that can be applied automatically.
	$(GOLANGCI_LINT) run --fix

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum.
	go mod tidy

# The fixers exit non zero whenever they changed a file, which is success here.
.PHONY: repo-fix
repo-fix: ## Strip trailing whitespace and fix missing final newlines.
	pre-commit run trailing-whitespace --hook-stage manual --all-files || true
	pre-commit run end-of-file-fixer --hook-stage manual --all-files || true

.PHONY: help
help: ## List every target.
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
