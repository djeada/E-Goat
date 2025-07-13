#!/bin/bash

# Cleanup script for E-Goat testing
# This script cleans up test artifacts and stops any running instances

echo "🧹 E-Goat Cleanup Script"
echo "======================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Get script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "🔍 Searching for running E-Goat processes..."

# Find and kill any running e-goat processes
PIDS=$(pgrep -f "e-goat" || true)

if [ -n "$PIDS" ]; then
    echo -e "${YELLOW}Found running E-Goat processes: $PIDS${NC}"
    echo "🛑 Stopping E-Goat processes..."
    
    # First try graceful shutdown
    for pid in $PIDS; do
        if kill -0 "$pid" 2>/dev/null; then
            echo "  Sending SIGTERM to process $pid..."
            kill "$pid" 2>/dev/null || true
        fi
    done
    
    # Wait a moment for graceful shutdown
    sleep 3
    
    # Force kill any remaining processes
    REMAINING_PIDS=$(pgrep -f "e-goat" || true)
    if [ -n "$REMAINING_PIDS" ]; then
        echo "  Force killing remaining processes: $REMAINING_PIDS"
        for pid in $REMAINING_PIDS; do
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null || true
            fi
        done
    fi
    
    echo -e "${GREEN}✅ All E-Goat processes stopped${NC}"
else
    echo -e "${GREEN}✅ No running E-Goat processes found${NC}"
fi

echo "🗑️  Cleaning up test artifacts..."

# Remove test databases
TEST_DBS=("test_chat_1.db" "test_chat_2.db" "chat.db" "quick_test.db")
for db in "${TEST_DBS[@]}"; do
    if [ -f "$db" ]; then
        echo "  Removing $db..."
        rm -f "$db"
    fi
done

# Remove any temporary files
if [ -d "tmp" ]; then
    echo "  Removing tmp directory..."
    rm -rf tmp
fi

# Remove any log files (if they exist)
LOG_FILES=(*.log)
for log in "${LOG_FILES[@]}"; do
    if [ -f "$log" ]; then
        echo "  Removing $log..."
        rm -f "$log"
    fi
done

# Clean up any socket files (if they exist)
SOCKET_FILES=(*.sock)
for socket in "${SOCKET_FILES[@]}"; do
    if [ -f "$socket" ]; then
        echo "  Removing $socket..."
        rm -f "$socket"
    fi
done

echo "🔧 Checking for leftover network connections..."

# Check for any leftover connections on test ports
TEST_PORTS=(8080 8081 9000 9001)
for port in "${TEST_PORTS[@]}"; do
    if lsof -ti:$port >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  Port $port might still be in use${NC}"
        PROCESSES=$(lsof -ti:$port)
        echo "  Processes using port $port: $PROCESSES"
        
        # Try to kill processes using the port
        for pid in $PROCESSES; do
            if kill -0 "$pid" 2>/dev/null; then
                echo "  Killing process $pid using port $port..."
                kill "$pid" 2>/dev/null || true
            fi
        done
    fi
done

# Wait a moment for network cleanup
sleep 2

echo "📊 Final cleanup verification..."

# Check if any e-goat processes are still running
FINAL_CHECK=$(pgrep -f "e-goat" || true)
if [ -n "$FINAL_CHECK" ]; then
    echo -e "${RED}❌ Some E-Goat processes are still running: $FINAL_CHECK${NC}"
    echo "   You may need to manually kill them with: kill -9 $FINAL_CHECK"
else
    echo -e "${GREEN}✅ No E-Goat processes running${NC}"
fi

# Check if test databases were removed
DB_CHECK=""
for db in "${TEST_DBS[@]}"; do
    if [ -f "$db" ]; then
        DB_CHECK="$DB_CHECK $db"
    fi
done

if [ -n "$DB_CHECK" ]; then
    echo -e "${YELLOW}⚠️  Some test databases still exist:$DB_CHECK${NC}"
else
    echo -e "${GREEN}✅ All test databases removed${NC}"
fi

# Check if test ports are free
BUSY_PORTS=""
for port in "${TEST_PORTS[@]}"; do
    if lsof -ti:$port >/dev/null 2>&1; then
        BUSY_PORTS="$BUSY_PORTS $port"
    fi
done

if [ -n "$BUSY_PORTS" ]; then
    echo -e "${YELLOW}⚠️  Some test ports are still in use:$BUSY_PORTS${NC}"
else
    echo -e "${GREEN}✅ All test ports are free${NC}"
fi

echo ""
echo -e "${GREEN}🎉 Cleanup completed!${NC}"

# Optional: Show what's left in the project directory
echo ""
echo "📁 Current project contents:"
ls -la
