# E-Goat Makefile
# Simple automation for building and testing E-Goat

.PHONY: help build test quick-test e2e-test e2e-interactive clean install run dev verify all demo

# Default target
help:
	@echo "E-Goat Build and Test Automation"
	@echo "================================"
	@echo ""
	@echo "Available targets:"
	@echo "  build        - Build the E-Goat binary"
	@echo "  verify       - Verify build and basic functionality"
	@echo "  test         - Run all tests (build + quick test)"
	@echo "  quick-test   - Run quick functionality test"
	@echo "  e2e-test     - Run end-to-end communication test (automated)"
	@echo "  e2e-interactive - Run end-to-end test with manual testing"
	@echo "  clean        - Clean up build artifacts and test files"
	@echo "  install      - Install dependencies and build"
	@echo "  run          - Build and run E-Goat"
	@echo "  dev          - Run in development mode (with auto-restart)"
	@echo "  demo         - Run interactive demo with two instances"
	@echo "  all          - Run complete verification (build + e2e test)"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make test"
	@echo "  make e2e-test"
	@echo "  make clean"

# Build the binary
build:
	@echo "🔨 Building E-Goat..."
	go mod tidy
	go build -o e-goat ./cmd/messanger
	@echo "✅ Build complete: e-goat"

# Verify build and run basic checks
verify:
	@echo "🔍 Running build verification..."
	./scripts/build-verify.sh

# Run quick tests (build + basic functionality)
test: 
	@echo "⚡ Running quick tests..."
	./scripts/run-tests.sh --quick

# Run quick functionality test only
quick-test:
	@echo "⚡ Running quick functionality test..."
	./scripts/quick-test.sh

# Run end-to-end communication test (automated)
e2e-test:
	@echo "🔗 Running end-to-end test (automated)..."
	./scripts/test-e2e.sh

# Run end-to-end test with interactive manual testing
e2e-interactive:
	@echo "🔗 Running end-to-end test (interactive)..."
	./scripts/test-e2e.sh --interactive

# Clean up everything
clean:
	@echo "🧹 Cleaning up..."
	./scripts/cleanup.sh
	rm -f e-goat
	go clean

# Install dependencies and build
install:
	@echo "📦 Installing dependencies..."
	go mod init github.com/djeada/E-Goat 2>/dev/null || true
	go mod tidy
	$(MAKE) build

# Build and run
run: build
	@echo "🚀 Starting E-Goat..."
	./e-goat

# Development mode (requires entr or similar for auto-restart)
dev:
	@echo "🔧 Starting development mode..."
	@echo "Note: This requires 'entr' to be installed for auto-restart"
	@echo "Install with: sudo apt-get install entr (Ubuntu/Debian) or brew install entr (macOS)"
	@if command -v entr >/dev/null 2>&1; then \
		find . -name "*.go" | entr -r make run; \
	else \
		echo "entr not found. Running once..."; \
		$(MAKE) run; \
	fi

# Run complete verification (build + e2e test)
all:
	@echo "🎯 Running complete verification..."
	./scripts/run-tests.sh

# Run interactive demo
demo:
	@echo "🎭 Starting interactive demo..."
	./scripts/demo.sh

# Check if scripts are executable
check-scripts:
	@if [ ! -x scripts/build-verify.sh ]; then \
		echo "Making scripts executable..."; \
		chmod +x scripts/*.sh; \
	fi

# Ensure scripts are executable before running them
scripts/build-verify.sh: check-scripts
scripts/quick-test.sh: check-scripts
scripts/test-e2e.sh: check-scripts
scripts/cleanup.sh: check-scripts
scripts/run-tests.sh: check-scripts
