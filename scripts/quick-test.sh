#!/bin/bash

# Quick test script for E-Goat
# This script performs a fast build and basic functionality test

set -e  # Exit on any error

echo "⚡ E-Goat Quick Test"
echo "=================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

# Quick build test
echo "🔨 Quick build test..."
if [ ! -f "e-goat" ] || [ "cmd/messanger/main.go" -nt "e-goat" ]; then
    echo "   Building E-Goat..."
    go build -o e-goat ./cmd/messanger
    echo -e "   ${GREEN}✅ Build successful${NC}"
else
    echo -e "   ${GREEN}✅ Binary is up to date${NC}"
fi

# Quick functionality test
echo "🧪 Quick functionality test..."

# Clean up any existing test database
rm -f quick_test.db

# Start a single instance for quick test
echo "   Starting test instance..."
./e-goat -http-port=8082 -ws-port=9002 -db=quick_test.db &
PID=$!

# Cleanup function
cleanup() {
    if kill -0 $PID 2>/dev/null; then
        kill $PID 2>/dev/null || true
        wait $PID 2>/dev/null || true
    fi
    rm -f quick_test.db
}

trap cleanup EXIT

# Wait for service to start
sleep 3

# Check if process is still running
if ! kill -0 $PID 2>/dev/null; then
    echo -e "   ${RED}❌ Process failed to start${NC}"
    exit 1
fi

# Test HTTP endpoint
echo "   Testing HTTP endpoint..."
if curl -s "http://localhost:8082/" > /dev/null; then
    echo -e "   ${GREEN}✅ HTTP endpoint responding${NC}"
else
    echo -e "   ${RED}❌ HTTP endpoint not responding${NC}"
    exit 1
fi

# Test REST API
echo "   Testing REST API..."
RESPONSE=$(curl -s -X POST "http://localhost:8082/send" \
    -H "Content-Type: application/json" \
    -d '{"room":"quick-test","peer_id":"test-peer","text":"Quick test message"}')

if echo "$RESPONSE" | grep -q "timestamp"; then
    echo -e "   ${GREEN}✅ REST API working${NC}"
else
    echo -e "   ${RED}❌ REST API failed${NC}"
    echo "   Response: $RESPONSE"
    exit 1
fi

# Test message retrieval
echo "   Testing message retrieval..."
HISTORY=$(curl -s "http://localhost:8082/history?room=quick-test&since=0")

if echo "$HISTORY" | grep -q "Quick test message"; then
    echo -e "   ${GREEN}✅ Message retrieval working${NC}"
else
    echo -e "   ${RED}❌ Message retrieval failed${NC}"
    echo "   History: $HISTORY"
    exit 1
fi

echo ""
echo -e "${GREEN}🎉 Quick test passed!${NC}"
echo ""
echo "All basic functionality is working. For comprehensive testing, run:"
echo "  ./scripts/test-e2e.sh"
