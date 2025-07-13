#!/bin/bash

# Demo script for E-Goat E2E communication
# This script demonstrates end-to-end communication between two instances

set -e

echo "🎭 E-Goat Communication Demo"
echo "============================"

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
DEMO_ROOM="demo-room"
DB_PATH_1="demo_chat_1.db"
DB_PATH_2="demo_chat_2.db"

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}🧹 Stopping demo instances...${NC}"
    pkill -f "e-goat.*$HTTP_PORT_1" 2>/dev/null || true
    pkill -f "e-goat.*$HTTP_PORT_2" 2>/dev/null || true
    rm -f "$DB_PATH_1" "$DB_PATH_2"
    sleep 2
    echo -e "${GREEN}✅ Demo cleanup completed${NC}"
}

trap cleanup EXIT

# Check if binary exists
if [ ! -f "e-goat" ]; then
    echo -e "${YELLOW}Building E-Goat first...${NC}"
    make build
fi

# Clean up any existing demo databases
rm -f "$DB_PATH_1" "$DB_PATH_2"

echo -e "${BLUE}🚀 Starting demo instances...${NC}"

# Start first instance
echo "Starting Instance 1 (Alice) on port $HTTP_PORT_1..."
./e-goat -http-port=$HTTP_PORT_1 -ws-port=$WS_PORT_1 -db="$DB_PATH_1" &
PID_1=$!

# Start second instance  
echo "Starting Instance 2 (Bob) on port $HTTP_PORT_2..."
./e-goat -http-port=$HTTP_PORT_2 -ws-port=$WS_PORT_2 -db="$DB_PATH_2" &
PID_2=$!

# Wait for services to start
echo "⏳ Waiting for services to start..."
sleep 5

# Check if processes are running
if ! kill -0 $PID_1 2>/dev/null || ! kill -0 $PID_2 2>/dev/null; then
    echo -e "${RED}❌ Failed to start instances${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Both instances started successfully!${NC}"

# Send some demo messages
echo -e "\n${BLUE}📨 Sending demo messages...${NC}"

# Alice sends a message
echo "Alice: Hello Bob, this is Alice!"
curl -s -X POST "http://localhost:$HTTP_PORT_1/send" \
    -H "Content-Type: application/json" \
    -d "{\"room\":\"$DEMO_ROOM\",\"peer_id\":\"Alice\",\"text\":\"Hello Bob, this is Alice!\"}" > /dev/null

sleep 1

# Bob sends a message
echo "Bob: Hi Alice! Nice to meet you."
curl -s -X POST "http://localhost:$HTTP_PORT_2/send" \
    -H "Content-Type: application/json" \
    -d "{\"room\":\"$DEMO_ROOM\",\"peer_id\":\"Bob\",\"text\":\"Hi Alice! Nice to meet you.\"}" > /dev/null

sleep 1

# Alice sends another message
echo "Alice: How's the weather today?"
curl -s -X POST "http://localhost:$HTTP_PORT_1/send" \
    -H "Content-Type: application/json" \
    -d "{\"room\":\"$DEMO_ROOM\",\"peer_id\":\"Alice\",\"text\":\"How's the weather today?\"}" > /dev/null

sleep 1

# Bob responds
echo "Bob: It's sunny! Perfect for coding."
curl -s -X POST "http://localhost:$HTTP_PORT_2/send" \
    -H "Content-Type: application/json" \
    -d "{\"room\":\"$DEMO_ROOM\",\"peer_id\":\"Bob\",\"text\":\"It's sunny! Perfect for coding.\"}" > /dev/null

sleep 2

# Show conversation history from both perspectives
echo -e "\n${BLUE}📜 Conversation History:${NC}"

echo -e "${YELLOW}From Alice's perspective (Instance 1):${NC}"
ALICE_HISTORY=$(curl -s "http://localhost:$HTTP_PORT_1/history?room=$DEMO_ROOM&since=0")
echo "$ALICE_HISTORY" | jq -r '.[] | "\(.peer_id): \(.text)"' 2>/dev/null || echo "$ALICE_HISTORY"

echo -e "\n${YELLOW}From Bob's perspective (Instance 2):${NC}"
BOB_HISTORY=$(curl -s "http://localhost:$HTTP_PORT_2/history?room=$DEMO_ROOM&since=0")
echo "$BOB_HISTORY" | jq -r '.[] | "\(.peer_id): \(.text)"' 2>/dev/null || echo "$BOB_HISTORY"

# Show browser URLs
echo -e "\n${GREEN}🌐 Open these URLs in your browser for interactive testing:${NC}"
echo ""
echo -e "${BLUE}Alice's Chat (Instance 1):${NC}"
echo "http://localhost:$HTTP_PORT_1/?room=$DEMO_ROOM&peer_id=Alice"
echo ""
echo -e "${BLUE}Bob's Chat (Instance 2):${NC}"
echo "http://localhost:$HTTP_PORT_2/?room=$DEMO_ROOM&peer_id=Bob"
echo ""

echo -e "${YELLOW}💡 Demo Instructions:${NC}"
echo "1. Open both URLs above in separate browser tabs"
echo "2. You'll see the conversation history from the demo"
echo "3. Try typing new messages in either tab"
echo "4. Messages will appear in both instances (via polling)"
echo "5. For WebRTC P2P connection, both browsers need to be on the same network"
echo ""

echo -e "${GREEN}🎉 Demo is running! Press Ctrl+C to stop...${NC}"

# Keep instances running
while true; do
    if ! kill -0 $PID_1 2>/dev/null; then
        echo -e "${RED}❌ Alice's instance stopped${NC}"
        break
    fi
    
    if ! kill -0 $PID_2 2>/dev/null; then
        echo -e "${RED}❌ Bob's instance stopped${NC}"
        break
    fi
    
    sleep 5
done
