#!/bin/bash

# Build verification script for E-Goat
# This script builds the project and verifies the binary works correctly

set -e  # Exit on any error

echo "🔨 E-Goat Build Verification Script"
echo "=================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Get script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "📂 Project root: $PROJECT_ROOT"

# Clean up any existing binary
echo "🧹 Cleaning up existing binary..."
rm -f e-goat

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed or not in PATH${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Go is available:${NC} $(go version)"

# Initialize Go modules if go.mod doesn't exist
if [ ! -f "go.mod" ]; then
    echo "🔧 Initializing Go modules..."
    go mod init github.com/djeada/E-Goat
fi

# Download and tidy dependencies
echo "📦 Downloading dependencies..."
go mod tidy

# Build the project
echo "🔨 Building E-Goat..."
start_time=$(date +%s)
go build -o e-goat ./cmd/messanger
end_time=$(date +%s)
build_time=$((end_time - start_time))

# Verify binary exists and is executable
if [ ! -f "e-goat" ]; then
    echo -e "${RED}❌ Build failed: e-goat binary not found${NC}"
    exit 1
fi

if [ ! -x "e-goat" ]; then
    echo -e "${RED}❌ Build failed: e-goat binary is not executable${NC}"
    exit 1
fi

binary_size=$(du -h e-goat | cut -f1)
echo -e "${GREEN}✅ Build successful!${NC}"
echo "📊 Binary size: $binary_size"
echo "⏱️  Build time: ${build_time}s"

# Test basic functionality (help/version check)
echo "🧪 Testing basic functionality..."
if ./e-goat -h > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Binary responds to help flag${NC}"
else
    echo -e "${YELLOW}⚠️  Binary doesn't respond to help flag (this might be normal)${NC}"
fi

# Run static analysis
echo "🔍 Running static analysis..."
if command -v go &> /dev/null; then
    echo "   Running go vet..."
    if go vet ./...; then
        echo -e "${GREEN}✅ go vet passed${NC}"
    else
        echo -e "${YELLOW}⚠️  go vet found issues${NC}"
    fi
    
    echo "   Running go fmt check..."
    if [ -z "$(gofmt -l .)" ]; then
        echo -e "${GREEN}✅ Code is properly formatted${NC}"
    else
        echo -e "${YELLOW}⚠️  Code formatting issues found:${NC}"
        gofmt -l .
    fi
fi

echo ""
echo -e "${GREEN}🎉 Build verification completed successfully!${NC}"
echo "Binary location: $PROJECT_ROOT/e-goat"
echo ""
echo "Next steps:"
echo "  - Run './scripts/test-e2e.sh' to test end-to-end communication"
echo "  - Run './e-goat' to start the application"
