export GOWORK := off

GO_MOD_VERSION := $(shell sed -n 's/^go //p' go.mod | head -1)
GOFMT_CMD      := gofmt
GOFMT_121      := $(shell go version | awk '{print $$3}' | sed 's/go//' | awk -F. '($$1 > 1) || ($$1 == 1 && $$2 >= 21){print 1}')
ifeq ($(GOFMT_121),1)
  GOFMT_CMD := gofmt
endif

APP_NAME = openadrctl

ARCHS = linux/amd64 linux/arm64 linux/arm/v7 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
TAG        := $(shell git describe --tags --exact-match 2>/dev/null || echo "none")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS = -s -w \
  -X main.version=$(VERSION) \
  -X main.tag=$(TAG) \
  -X main.commit=$(COMMIT) \
  -X main.buildDate=$(BUILD_DATE)

# Minimum total statement coverage (percent) enforced by `make coverage`.
COVERAGE_MIN ?= 85

.PHONY: help
help: ## Show this help.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: run
run: ## Run the application.
	go run -ldflags "$(LDFLAGS)" .

.PHONY: check
check: fmt-check vet lint test coverage ## Run all checks (format, vet, lint, test, coverage)

.PHONY: test
test: ## Run tests.
	go test -race -count=1 ./...

.PHONY: stress
stress: ## Run tests with -count=10 for stress testing.
	go test -race -count=10 ./...

.PHONY: coverage
coverage: ## Run tests with whole-module coverage (writes coverage.out); fails below COVERAGE_MIN percent
	@echo "Running tests with coverage"
	@go test -count=1 -race -coverprofile=coverage.out -covermode=atomic -coverpkg=./... ./...
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ { sub("%", "", $$3); print $$3 }'); \
	echo "Total coverage: $$total% (minimum $(COVERAGE_MIN)%)"; \
	awk -v t="$$total" -v m="$(COVERAGE_MIN)" 'BEGIN { exit (t + 0 < m + 0) ? 1 : 0 }' || \
		{ echo "coverage $$total% is below $(COVERAGE_MIN)%"; exit 1; }

.PHONY: cover
cover: coverage ## Run tests with coverage and open HTML report.
	go tool cover -html=coverage.out -o coverage.html

.PHONY: compile
compile: ## Compile without running (syntax check).
	go build ./...

.PHONY: lint
lint: ## Run golangci-lint.
	golangci-lint run ./...

.PHONY: lint-ci
lint-ci: ## Run golangci-lint for CI (stricter).
	golangci-lint run --out-format=colored-line-number ./...

.PHONY: fmt
fmt: ## Format code.
	$(GOFMT_CMD) -w .
	goimports -w .

.PHONY: fmt-check
fmt-check: ## Check code formatting.
	@test -z "$$($(GOFMT_CMD) -l .)" || (echo "Files need formatting:"; $(GOFMT_CMD) -l .; exit 1)

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: build
build: check ## Build the binary (with checks).
	go build -ldflags "$(LDFLAGS)" -o $(APP_NAME) .

.PHONY: build-nocheck
build-nocheck: ## Build the binary (without checks).
	go build -ldflags "$(LDFLAGS)" -o $(APP_NAME) .

.PHONY: build-all
build-all: ## Build for all target architectures.
	@for arch in $(ARCHS); do \
		os=$$(echo $$arch | cut -d/ -f1); \
		cpu=$$(echo $$arch | cut -d/ -f2); \
		arm=$$(echo $$arch | cut -d/ -f3); \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "Building $$os/$$cpu$$arm..."; \
		GOOS=$$os GOARCH=$$cpu GOARM=$${arm#v} go build -ldflags "$(LDFLAGS)" \
			-o release/$(APP_NAME)-$$os-$$cpu$${arm:+-$$arm}$$ext . || exit 1; \
	done

.PHONY: release-all
release-all: check build-all ## Build release binaries for all architectures.

.PHONY: install
install: ## Install the binary to GOPATH/bin.
	go install -ldflags "$(LDFLAGS)" .

.PHONY: clean
clean: ## Remove build artifacts.
	rm -rf $(APP_NAME) release/ bin/ coverage.*
