# reducarr Makefile
# Build and test commands for the reducarr project

# =============================================================================
# Configuration
# =============================================================================

BINARY_NAME ?= reducarr
BINARY_DIR ?= ./cmd/reducarr

# Flags for static builds (no libc)
STATIC_FLAGS := -ldflags="-s -w"

# Default environment (can be overridden)
GOOS ?= 
GOARCH ?= 

# =============================================================================
# Main Targets
# =============================================================================

.PHONY: all build build-static build-docker test coverage coverage-summary coverage-ci generate clean help

# Default target: classic build (with libc)
all: build

# Classic build for local development (WITH libc)
build: generate
	go build -o $(BINARY_NAME) $(BINARY_DIR)

# Static build (WITHOUT libc, portable)
build-static: generate
	CGO_ENABLED=0 go build $(STATIC_FLAGS) -o $(BINARY_NAME) $(BINARY_DIR)

# Docker build (matches Dockerfile: static, linux/amd64)
build-docker: generate
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(STATIC_FLAGS) -o $(BINARY_NAME) $(BINARY_DIR)

# =============================================================================
# Code Generation
# =============================================================================

generate:
	go generate ./...

# =============================================================================
# Testing
# =============================================================================

test: generate
	go test ./...

# =============================================================================
# Coverage
# =============================================================================

# User's exact coverage command (go build -cover is kept for compatibility)
coverage: generate
	go build -cover $(BINARY_DIR) || true
	go test ./... -coverprofile=cover.out -coverpkg=./...
	go tool cover -html cover.out

# Alternative: Better coverage implementation (recommended)
coverage-summary: generate
	go test ./... -coverprofile=cover.out -covermode=atomic
	@echo "\n=== Coverage Summary ==="
	go tool cover -func=cover.out | tail -1

# CI-friendly coverage: generates cover.out + coverage.xml (Cobertura format, no browser)
# Requires: go install github.com/boumenot/gocover-cobertura@latest
coverage-ci: generate
	go test -race ./... -coverprofile=cover.out -covermode=atomic
	gocover-cobertura < cover.out > coverage.xml
	@echo "\n=== Coverage Summary ==="
	go tool cover -func=cover.out | tail -1

# =============================================================================
# Utilities
# =============================================================================

fmt:
	go fmt ./...

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)-* cover.out

install: generate
	go install ./cmd/reducarr

run: build
	./$(BINARY_NAME) serve

tidy:
	go mod tidy

version:
	@./$(BINARY_NAME) version 2>/dev/null || echo "Run 'make build' first"

# =============================================================================
# Cross-compilation
# =============================================================================

# Pattern: make build-linux-amd64, make build-windows-amd64, etc.
build-%:
	@echo "Building for: $*"
	CGO_ENABLED=0 GOOS=$(word 1, $(subst -, ,$*)) GOARCH=$(word 2, $(subst -, ,$*)) \
		go build $(STATIC_FLAGS) -o $(BINARY_NAME)-$* $(BINARY_DIR)

build-linux-amd64: 
build-linux-arm64: 
build-windows-amd64: 
build-darwin-amd64: 
build-darwin-arm64: 

# =============================================================================
# Help
# =============================================================================

help:
	@echo "reducarr - Build System"
	@echo ""
	@echo "Main Targets:"
	@echo "  build           - Build classic binary (with libc) [DEFAULT]"
	@echo "  build-static    - Build static binary (no libc)"
	@echo "  build-docker    - Build Docker-optimized binary"
	@echo ""
	@echo "Testing:"
	@echo "  test            - Run all tests"
	@echo ""
	@echo "Coverage:"
	@echo "  coverage        - Coverage report (user's command)"
	@echo "  coverage-summary - Coverage summary (recommended)"
	@echo "  coverage-ci     - Coverage + Cobertura XML (mirrors CI, requires gocover-cobertura)"
	@echo ""
	@echo "Utilities:"
	@echo "  generate        - Generate code (templ, etc.)"
	@echo "  fmt             - Format code"
	@echo "  clean           - Clean build artifacts"
	@echo "  install         - Install binary to GOPATH"
	@echo "  run             - Build and run the application"
	@echo "  tidy            - Clean up go.mod and go.sum"
	@echo "  version         - Show binary version"
	@echo "  help            - Show this help message"
	@echo ""
	@echo "Cross-compilation:"
	@echo "  build-linux-amd64"
	@echo "  build-linux-arm64"
	@echo "  build-windows-amd64"
	@echo "  build-darwin-amd64"
	@echo "  build-darwin-arm64"
