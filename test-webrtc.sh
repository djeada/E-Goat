#!/bin/bash

# E-Goat WebRTC Testing Script
# This script helps you test the complete WebRTC functionality

set -e

echo "🚀 E-Goat WebRTC Testing Script"
echo "==============================="

# Configuration
SERVER_PORT=8080
SIGNALING_PORT=9000
ROOM_NAME="test-webrtc-$(date +%s)"

echo "📋 Test Configuration:"
echo "   Server Port: $SERVER_PORT"
echo "   Signaling Port: $SIGNALING_PORT"
echo "   Test Room: $ROOM_NAME"
echo ""

# Function to check if port is available
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        echo "❌ Port $port is already in use"
        return 1
    fi
    return 0
}

# Function to wait for server to be ready
wait_for_server() {
    local port=$1
    local max_attempts=30
    local attempt=1
    
    echo "⏳ Waiting for server on port $port..."
    while [ $attempt -le $max_attempts ]; do
        if curl -s "http://localhost:$port" >/dev/null 2>&1; then
            echo "✅ Server ready on port $port"
            return 0
        fi
        echo "   Attempt $attempt/$max_attempts..."
        sleep 1
        attempt=$((attempt + 1))
    done
    
    echo "❌ Server failed to start on port $port after $max_attempts attempts"
    return 1
}

# Check if required ports are available
echo "🔍 Checking port availability..."
if ! check_port $SERVER_PORT; then
    echo "💡 Tip: Stop existing server with: pkill -f 'go run.*main.go'"
    exit 1
fi

if ! check_port $SIGNALING_PORT; then
    echo "💡 Tip: Stop existing signaling server with: pkill -f 'go run.*main.go'"
    exit 1
fi

# Start the server in background
echo "🔧 Starting E-Goat server..."
cd /workspaces/E-Goat
go run cmd/messanger/main.go &
SERVER_PID=$!

echo "📝 Server PID: $SERVER_PID"

# Wait for server to be ready
if ! wait_for_server $SERVER_PORT; then
    echo "❌ Failed to start server"
    kill $SERVER_PID 2>/dev/null || true
    exit 1
fi

# Create test URLs
BASE_URL="http://localhost:$SERVER_PORT"
ROOM_URL="$BASE_URL/?room=$ROOM_NAME"

echo ""
echo "🎯 WebRTC Test Instructions:"
echo "============================"
echo ""
echo "1. 📱 **First Browser Tab**"
echo "   Open: $ROOM_URL&peer_id=peer1"
echo ""
echo "2. 📱 **Second Browser Tab**"
echo "   Open: $ROOM_URL&peer_id=peer2"
echo ""
echo "3. 🔍 **What to Look For:**"
echo "   • Both tabs should auto-connect via WebRTC signaling"
echo "   • Watch for peer discovery: '👋 peer2 joined the room'"
echo "   • Look for WebRTC connection states: 'connecting' → 'connected'"
echo "   • Data channels should show as 'open' in debug panel"
echo "   • Connection Quality should show 'P2P Active'"
echo ""
echo "4. 💬 **Test Messages:**"
echo "   • Send messages from either tab"
echo "   • Should see: '🚀 ✅ Sent via WebRTC to 1 peer(s)'"
echo "   • NOT: 'HTTP Polling Only' fallback"
echo ""
echo "5. 🐛 **Debug Information:**"
echo "   • Open Browser Console (F12) for detailed logs"
echo "   • Check the Debug Information panel on each page"
echo "   • Monitor connection states and channel counts"
echo ""

echo "🔗 **Quick Test URLs:**"
echo "   Tab 1: $ROOM_URL&peer_id=peer1"
echo "   Tab 2: $ROOM_URL&peer_id=peer2"
echo ""

# Interactive mode
echo "Press Enter to continue, or Ctrl+C to stop the server..."
read -r

echo ""
echo "🔧 **Common Issues & Solutions:**"
echo "================================"
echo ""
echo "❌ **WebRTC Falls Back to HTTP Polling:**"
echo "   • Check browser console for ICE connection errors"
echo "   • Try refreshing both tabs and waiting 10-15 seconds"
echo "   • Localhost WebRTC can be flaky - try 127.0.0.1 instead"
echo ""
echo "❌ **'No open data channels' Error:**"
echo "   • WebRTC signaling worked but data channels didn't open"
echo "   • Check ICE connection state in console logs"
echo "   • May need TURN server for some network configurations"
echo ""
echo "❌ **Connection Timeout:**"
echo "   • WebRTC has 15-second timeout for connections"
echo "   • Try manual connection with 'Connect to Peer' button"
echo "   • Use peer IDs from debug panel"
echo ""
echo "✅ **Success Indicators:**"
echo "   • 'Data channel ready with [peer] - WebRTC P2P active!'"
echo "   • 'P2P Active (1 WebRTC channels)' in Connection Quality"
echo "   • Messages show '📨 ✅ Message via WebRTC from [peer]'"
echo ""

echo "Press Enter to stop the server..."
read -r

# Cleanup
echo ""
echo "🧹 Cleaning up..."
echo "Stopping server (PID: $SERVER_PID)..."
kill $SERVER_PID 2>/dev/null || true

# Wait a moment for cleanup
sleep 2

# Double-check cleanup
if kill -0 $SERVER_PID 2>/dev/null; then
    echo "⚠️  Server still running, force killing..."
    kill -9 $SERVER_PID 2>/dev/null || true
fi

echo "✅ Server stopped"
echo ""
echo "📊 **Test Summary:**"
echo "=================="
echo "If you saw:"
echo "✅ Both peers auto-connected"
echo "✅ Data channels opened successfully" 
echo "✅ Messages sent via WebRTC (not HTTP polling)"
echo "✅ Connection Quality showed 'P2P Active'"
echo ""
echo "Then WebRTC is working correctly! 🎉"
echo ""
echo "If messages fell back to HTTP polling, check the debug"
echo "information in the browser console for specific errors."
echo ""
echo "For testing on different machines:"
echo "💡 Replace 'localhost' with your machine's IP address"
echo "💡 Ensure firewall allows ports $SERVER_PORT and $SIGNALING_PORT"
echo "💡 For internet connections, you'll need a TURN server"
