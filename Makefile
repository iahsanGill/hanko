# Copyright 2026 The Hanko Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0

.PHONY: help build test lint fmt check install-hooks

help: ## Show this help.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

build: ## Compile all packages.
	go build ./...

test: ## Run all tests with race detector.
	go test ./... -count=1 -race

lint: ## Run golangci-lint via Docker (matches CI version).
	docker run --rm -v "$$PWD":/src -w /src golangci/golangci-lint:v2.12.2 golangci-lint run --timeout=5m

fmt: ## Run gofmt -w on the tree.
	gofmt -w .

check: fmt build test ## Format, build, test — what CI runs.

install-hooks: ## Symlink hack/pre-commit.sh into .git/hooks/pre-commit.
	@mkdir -p .git/hooks
	@ln -sf ../../hack/pre-commit.sh .git/hooks/pre-commit
	@echo "installed .git/hooks/pre-commit -> hack/pre-commit.sh"
