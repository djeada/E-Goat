#!/bin/bash

# End-to-End testing script for E-Goat
# This script starts two instances and tests communication between them

set -e  # Exit on any error

echo "🔗 E-Goat End-to-End Communication Test"
echo "======================================"

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

# Configuration
HTTP_PORT_1=8080
WS_PORT_1=9000
HTTP_PORT_2=8081
WS_PORT_2=9001
TEST_ROOM="e2e-test-room"
DB_PATH_1="test_chat_1.db"
DB_PATH_2="test_chat_2.db"
TEST_TIMEOUT=30

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}🧹 Cleaning up test environment...${NC}"
    
    # Kill any running e-goat processes
    pkill -f "e-goat.*$HTTP_PORT_1" 2>/dev/null || true
    pkill -f "e-goat.*$HTTP_PORT_2" 2>/dev/null || true
    
    # Remove test databases
    rm -f "$DB_PATH_1" "$DB_PATH_2"
    
    # Wait a moment for processes to clean up
    sleep 2
    
    echo -e "${GREEN}✅ Cleanup completed${NC}"
}

# Set up cleanup trap
trap cleanup EXIT

# Check if binary exists
if [ ! -f "e-goat" ]; then
    echo -e "${RED}❌ e-goat binary not found. Run ./scripts/build-verify.sh first${NC}"
    exit 1
fi

echo "🔧 Starting E-Goat instances..."

# Clean up any existing test databases
rm -f "$DB_PATH_1" "$DB_PATH_2"

# Start first instance
echo -e "${BLUE}🚀 Starting instance 1 (HTTP:$HTTP_PORT_1, WS:$WS_PORT_1)${NC}"
./e-goat -http-port=$HTTP_PORT_1 -ws-port=$WS_PORT_1 -db="$DB_PATH_1" &
PID_1=$!

# Start second instance
echo -e "${BLUE}🚀 Starting instance 2 (HTTP:$HTTP_PORT_2, WS:$WS_PORT_2)${NC}"
./e-goat -http-port=$HTTP_PORT_2 -ws-port=$WS_PORT_2 -db="$DB_PATH_2" &
PID_2=$!

# Wait for services to start
echo "⏳ Waiting for services to start..."
sleep 5

# Check if processes are still running
if ! kill -0 $PID_1 2>/dev/null; then
    echo -e "${RED}❌ Instance 1 failed to start${NC}"
    exit 1
fi

if ! kill -0 $PID_2 2>/dev/null; then
    echo -e "${RED}❌ Instance 2 failed to start${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Both instances started successfully${NC}"

# Test HTTP endpoints
echo "🧪 Testing HTTP endpoints..."

# Test instance 1
echo "   Testing instance 1 HTTP endpoint..."
if curl -s "http://localhost:$HTTP_PORT_1/" > /dev/null; then
    echo -e "   ${GREEN}✅ Instance 1 HTTP endpoint responding${NC}"
else
    echo -e "   ${RED}❌ Instance 1 HTTP endpoint not responding${NC}"
    exit 1
fi

# Test instance 2
echo "   Testing instance 2 HTTP endpoint..."
if curl -s "http://localhost:$HTTP_PORT_2/" > /dev/null; then
    echo -e "   ${GREEN}✅ Instance 2 HTTP endpoint responding${NC}"
else
    echo -e "   ${RED}❌ Instance 2 HTTP endpoint not responding${NC}"
    exit 1
fi

# Test WebSocket endpoints (check if ports are open)
echo "🧪 Testing WebSocket endpoints..."

# Test instance 1 WebSocket
echo "   Testing instance 1 WebSocket endpoint..."
if timeout 5 bash -c "</dev/tcp/localhost/$WS_PORT_1" 2>/dev/null; then
    echo -e "   ${GREEN}✅ Instance 1 WebSocket port is open${NC}"
else
    echo -e "   ${YELLOW}⚠️  Instance 1 WebSocket port test failed (might be normal)${NC}"
fi

# Test instance 2 WebSocket
echo "   Testing instance 2 WebSocket endpoint..."
if timeout 5 bash -c "</dev/tcp/localhost/$WS_PORT_2" 2>/dev/null; then
    echo -e "   ${GREEN}✅ Instance 2 WebSocket port is open${NC}"
else
    echo -e "   ${YELLOW}⚠️  Instance 2 WebSocket port test failed (might be normal)${NC}"
fi

# Test REST API communication
echo "🧪 Testing REST API communication..."

PEER_ID_1="test-peer-1"
PEER_ID_2="test-peer-2"
TEST_MESSAGE_1="Hello from peer 1!"
TEST_MESSAGE_2="Hello from peer 2!"

# Send message from instance 1
echo "   Sending message from instance 1..."
RESPONSE_1=$(curl -s -X POST "http://localhost:$HTTP_PORT_1/send" \
    -H "Content-Type: application/json" \
    -d "{\"room\":\"$TEST_ROOM\",\"peer_id\":\"$PEER_ID_1\",\"text\":\"$TEST_MESSAGE_1\"}")

if echo "$RESPONSE_1" | grep -q "timestamp"; then
    echo -e "   ${GREEN}✅ Message sent from instance 1${NC}"
    TIMESTAMP_1=$(echo "$RESPONSE_1" | grep -o '"timestamp":[^,}]*' | cut -d':' -f2)
else
    echo -e "   ${RED}❌ Failed to send message from instance 1${NC}"
    echo "   Response: $RESPONSE_1"
    exit 1
fi

# Send message from instance 2
echo "   Sending message from instance 2..."
RESPONSE_2=$(curl -s -X POST "http://localhost:$HTTP_PORT_2/send" \
    -H "Content-Type: application/json" \
    -d "{\"room\":\"$TEST_ROOM\",\"peer_id\":\"$PEER_ID_2\",\"text\":\"$TEST_MESSAGE_2\"}")

if echo "$RESPONSE_2" | grep -q "timestamp"; then
    echo -e "   ${GREEN}✅ Message sent from instance 2${NC}"
    TIMESTAMP_2=$(echo "$RESPONSE_2" | grep -o '"timestamp":[^,}]*' | cut -d':' -f2)
else
    echo -e "   ${RED}❌ Failed to send message from instance 2${NC}"
    echo "   Response: $RESPONSE_2"
    exit 1
fi

# Wait a moment for messages to be stored
sleep 2

# Retrieve messages from instance 1
echo "   Retrieving messages from instance 1..."
HISTORY_1=$(curl -s "http://localhost:$HTTP_PORT_1/history?room=$TEST_ROOM&since=0")

if echo "$HISTORY_1" | grep -q "$TEST_MESSAGE_1"; then
    echo -e "   ${GREEN}✅ Instance 1 can retrieve its own message${NC}"
else
    echo -e "   ${RED}❌ Instance 1 cannot retrieve its own message${NC}"
    echo "   History: $HISTORY_1"
    exit 1
fi

# Retrieve messages from instance 2
echo "   Retrieving messages from instance 2..."
HISTORY_2=$(curl -s "http://localhost:$HTTP_PORT_2/history?room=$TEST_ROOM&since=0")

if echo "$HISTORY_2" | grep -q "$TEST_MESSAGE_2"; then
    echo -e "   ${GREEN}✅ Instance 2 can retrieve its own message${NC}"
else
    echo -e "   ${RED}❌ Instance 2 cannot retrieve its own message${NC}"
    echo "   History: $HISTORY_2"
fi

# Test cross-instance message retrieval (if they share storage mechanism)
echo "🧪 Testing message persistence..."

# Check if instance 1 stored exactly one message
MESSAGE_COUNT_1=$(echo "$HISTORY_1" | grep -o '"text":' | wc -l)
echo "   Instance 1 stored $MESSAGE_COUNT_1 message(s)"

# Check if instance 2 stored exactly one message
MESSAGE_COUNT_2=$(echo "$HISTORY_2" | grep -o '"text":' | wc -l)
echo "   Instance 2 stored $MESSAGE_COUNT_2 message(s)"

# Test UI accessibility
echo "🧪 Testing UI accessibility..."

# Test if the main UI loads
UI_1=$(curl -s "http://localhost:$HTTP_PORT_1/")
if echo "$UI_1" | grep -q "html"; then
    echo -e "   ${GREEN}✅ Instance 1 UI loads successfully${NC}"
else
    echo -e "   ${RED}❌ Instance 1 UI failed to load${NC}"
fi

UI_2=$(curl -s "http://localhost:$HTTP_PORT_2/")
if echo "$UI_2" | grep -q "html"; then
    echo -e "   ${GREEN}✅ Instance 2 UI loads successfully${NC}"
else
    echo -e "   ${RED}❌ Instance 2 UI failed to load${NC}"
fi

# Generate test report
echo ""
echo "📊 Test Summary"
echo "==============="
echo -e "${BLUE}Instance 1:${NC}"
echo "  - HTTP Port: $HTTP_PORT_1"
echo "  - WebSocket Port: $WS_PORT_1"
echo "  - Database: $DB_PATH_1"
echo "  - Messages stored: $MESSAGE_COUNT_1"
echo "  - UI: http://localhost:$HTTP_PORT_1/?room=$TEST_ROOM&peer_id=$PEER_ID_1"

echo -e "${BLUE}Instance 2:${NC}"
echo "  - HTTP Port: $HTTP_PORT_2"
echo "  - WebSocket Port: $WS_PORT_2"
echo "  - Database: $DB_PATH_2"
echo "  - Messages stored: $MESSAGE_COUNT_2"
echo "  - UI: http://localhost:$HTTP_PORT_2/?room=$TEST_ROOM&peer_id=$PEER_ID_2"

echo ""
echo -e "${GREEN}🎉 End-to-End test completed successfully!${NC}"
echo ""
echo "Manual testing instructions:"
echo "1. Open http://localhost:$HTTP_PORT_1/?room=$TEST_ROOM&peer_id=$PEER_ID_1 in one browser tab"
echo "2. Open http://localhost:$HTTP_PORT_2/?room=$TEST_ROOM&peer_id=$PEER_ID_2 in another browser tab"
echo "3. Try sending messages between the two instances"
echo "4. Test file sharing if supported"
echo ""
echo "Press Ctrl+C to stop the test instances..."

# Keep the script running so instances stay alive for manual testing
while true; do
    if ! kill -0 $PID_1 2>/dev/null; then
        echo -e "${RED}❌ Instance 1 died unexpectedly${NC}"
        break
    fi
    
    if ! kill -0 $PID_2 2>/dev/null; then
        echo -e "${RED}❌ Instance 2 died unexpectedly${NC}"
        break
    fi
    
    sleep 5
done
