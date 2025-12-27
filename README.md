# E-Goat

E-Goat is a lightweight P2P messaging application built in Go that supports multiple connection types with automatic fallback. It combines WebRTC peer-to-peer connections, WebSocket communication, and HTTP polling to ensure reliable connectivity across different network environments.

![e_goat](https://github.com/user-attachments/assets/9ac71bdd-1fe3-41b9-89c0-21de1e140ced)

## ✨ Features

- **🔗 Layered Connection Architecture** - Automatic fallback between multiple connection types
- **🌐 WebRTC P2P Mesh** - Direct peer-to-peer connections with minimal latency
- **⚡ Real-time WebSocket** - Bidirectional communication for instant messaging
- **🔄 HTTP Polling Fallback** - Universal compatibility across all network environments
- **💾 Local SQLite Storage** - All messages stored locally with easy backup/migration
- **📱 Single Binary** - No dependencies beyond a web browser
- **🧪 Comprehensive Testing** - Full test suite for build verification and E2E communication

## 🚀 Quick Start

### Build and Run
```bash
# Build the application
make build

# Run all verification tests
make test

# Start E-Goat
make run

# Or manually:
go build -o e-goat ./cmd/messanger
./e-goat -http-port=8080 -ws-port=9000 -db="$HOME/tmp/e-goat/chat.db"
```

### Access the Application
Open [http://localhost:8080](http://localhost:8080) in your browser, create or join a room, and start chatting!

## 🧪 Testing & Verification

E-Goat includes a comprehensive testing suite to ensure reliability and functionality:

### Make Targets
```bash
# Run all tests (recommended)
make test

# Build verification only  
make verify

# Quick functionality test
make quick-test

# End-to-end communication test
make e2e-test

# Run complete verification
make all

# Clean up test artifacts
make clean
```

### Test Scripts
- **`./scripts/build-verify.sh`** - Verifies build process and basic functionality
- **`./scripts/quick-test.sh`** - Fast single-instance functionality test
- **`./scripts/test-e2e.sh`** - Comprehensive two-instance communication test with network scenarios
- **`./scripts/run-tests.sh`** - Main test orchestrator with configurable options
- **`./scripts/cleanup.sh`** - Cleans up test artifacts and processes

### Unit Tests
```bash
# Run unit tests for transport layer
go test -v ./tests/...

# Run with coverage
go test -cover ./tests/...
```

### Script Options
```bash
./scripts/run-tests.sh --quick           # Quick tests only
./scripts/run-tests.sh --skip-e2e        # Skip end-to-end tests  
./scripts/run-tests.sh --interactive     # Interactive mode with prompts
```

### Manual Testing
After running the E2E test, manually verify communication by opening:
- **Instance 1**: http://localhost:8080/?room=test&peer_id=peer1  
- **Instance 2**: http://localhost:8081/?room=test&peer_id=peer2

Send messages between instances to verify real-time communication.

## 🏗️ Architecture Overview

E-Goat implements a **layered connection architecture** that automatically selects the best available connection method and gracefully falls back when connections fail.

### Connection Priority System

E-Goat tries connection types in order of preference and performance:

1. **🚀 WebRTC STUN (Priority 100)** - Direct P2P connections
   - Best performance, lowest latency
   - Works through most NATs
   - No server relay needed

2. **🔄 WebRTC TURN (Priority 80)** - Relayed P2P connections  
   - Works through symmetric NATs
   - Uses TURN server relay
   - Maintains P2P encryption

3. **⚡ WebSocket Direct (Priority 60)** - Real-time TCP connections
   - Bidirectional real-time communication
   - Works in most corporate networks
   - Lower overhead than HTTP polling

4. **🌐 HTTP Polling (Priority 40)** - REST API fallback
   - Works everywhere HTTP works
   - Highest compatibility
   - Higher latency due to polling

5. **📡 LAN Broadcast (Priority 20)** - Local network discovery
   - Direct local network communication
   - No internet required
   - Perfect for local setups

### Network Adaptation

The system automatically adapts to different network conditions:

- **🟢 Perfect LAN**: Uses WebRTC STUN for optimal performance
- **🏢 Corporate Network**: Falls back to WebSocket/HTTP
- **🔒 Symmetric NAT**: Automatically uses TURN relay
- **🚫 Highly Restricted**: HTTP polling ensures connectivity

### Component Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│ E-Goat Application                                          │
├─────────────────────────────────────────────────────────────┤
│ ┌─────────────────┐  ┌─────────────────┐  ┌───────────────┐ │
│ │   HTTP Server   │  │ WebSocket Server│  │ Signaling Hub │ │
│ │   (Port 8080)   │  │   (Port 9000)   │  │   (WebRTC)    │ │
│ └─────────────────┘  └─────────────────┘  └───────────────┘ │
├─────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────┐ │
│ │            Layered Transport Manager                    │ │
│ │  ┌──────────┐ ┌─────────┐ ┌─────────┐ ┌─────────────┐   │ │
│ │  │WebRTC P2P│ │WebSocket│ │HTTP Poll│ │LAN Broadcast│   │ │
│ │  └──────────┘ └─────────┘ └─────────┘ └─────────────┘   │ │
│ └─────────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────────┤
│ ┌─────────────────┐  ┌────────────────────────────────────┐ │
│ │ SQLite Storage  │  │        Web Interface               │ │
│ │ ($HOME/tmp/     │  │ (HTML/CSS/JavaScript)              │ │
│ │  e-goat/chat.db)│  │                                     │ │
│ └─────────────────┘  └────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### Message Flow

1. **Connection Establishment**: Client connects using the highest priority available method
2. **Automatic Failover**: If connection fails, automatically tries next priority method  
3. **Quality Monitoring**: Continuously monitors connection quality and switches if needed
4. **Message Delivery**: All connection types use the same message interface
5. **Persistent Storage**: Messages stored locally in SQLite regardless of connection type

## 🌐 Network Configuration

### Automatic Connection Selection

E-Goat automatically handles network configuration challenges:

- **No Configuration Needed**: Works out-of-the-box on most networks
- **Smart Fallbacks**: Automatically tries different connection methods  
- **Quality Monitoring**: Continuously optimizes connection performance
- **Universal Compatibility**: HTTP polling ensures connectivity in any environment

### NAT Traversal & Port Forwarding

By default the HTTP server (`:8080`) and signaling WS (`:9000`) listen on all interfaces, but most home networks sit behind NAT. To allow external peers to reach you without WebRTC STUN/TURN:

#### Quick Linux Setup (Firewall + Checks)
```bash
./scripts/setup-network.sh
sudo ./scripts/setup-network.sh --apply
./scripts/setup-network.sh --port-map
```

#### Router Port-Forwarding
```bash
# Forward these ports on your router:
# HTTP: 8080 → your-machine:8080
# WebSocket: 9000 → your-machine:9000

# External access URL format:
# http://YOUR_WAN_IP:8080/?room=myroom&peer_id=XYZ
```

#### Public Invite Base (Optional)
If you're running locally but want invite links to use a public IP or domain:
```bash
./e-goat -public-base="http://YOUR_WAN_IP:8080"
# or set EGOAT_PUBLIC_BASE=http://YOUR_WAN_IP:8080
```

#### Firewall Configuration
```bash
# UFW (Ubuntu/Debian)
sudo ufw allow 8080/tcp
sudo ufw allow 9000/tcp
sudo ufw reload

# firewalld (RHEL/CentOS)  
sudo firewall-cmd --add-port=8080/tcp --permanent
sudo firewall-cmd --add-port=9000/tcp --permanent
sudo firewall-cmd --reload

# iptables
sudo iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 9000 -j ACCEPT
sudo apt install iptables-persistent
sudo netfilter-persistent save
```

### STUN/TURN Server Configuration

For WebRTC connections in complex network environments:

```bash
# Built-in STUN servers (Google)
# stun:stun.l.google.com:19302
# stun:stun1.l.google.com:19302

# Custom TURN server (optional)
# turn:your-turn-server.com:3478
```

## 🔌 API Reference

### HTTP Endpoints (Port 8080)

#### Web Interface
- **`GET /`** - Serves the chat UI (index.html, styles.css, chat.js)
- **`GET /static/*`** - Serves frontend assets

#### REST API
- **`GET /history?room=<name>&since=<timestamp>`**  
  Returns JSON array of messages in room since timestamp
  ```json
  [
    {
      "peer_id": "user123",
      "text": "Hello everyone!",
      "timestamp": 1625097600
    }
  ]
  ```

- **`POST /send`**  
  Accepts JSON message and returns timestamp
  ```json
  // Request
  {
    "room": "general",
    "peer_id": "user123", 
    "text": "Hello world!"
  }
  
  // Response
  {
    "timestamp": 1625097600
  }
  ```

### WebSocket Signaling (Port 9000)

- **`ws://host:9000/signal?room=<name>&peer_id=<id>`**  
  WebRTC signaling hub for SDP offers, answers, and ICE candidates

Signal message format:
```json
{
  "peer_id": "user123",
  "type": "offer|answer|ice",
  "payload": "<SDP_or_ICE_data>"
}
```

## ⚙️ Technologies & Dependencies

### Backend (Go)
- **Core Language**: Go 1.21+ for single-binary builds
- **WebRTC**: `github.com/pion/webrtc/v3` - P2P connections and DataChannels
- **WebSockets**: `github.com/gorilla/websocket` - Real-time communication
- **Database**: `github.com/mattn/go-sqlite3` - Embedded local storage
- **Standard Library**: `net/http`, `database/sql`, `embed`, `flag`

### Frontend
- **Vanilla JavaScript**: WebRTC and WebSocket APIs
- **HTML/CSS**: Minimal responsive design
- **No Frameworks**: Zero dependencies, works in any modern browser

### Transport Layer
- **Layered Architecture**: Multiple connection types with automatic fallback
- **Quality Monitoring**: Continuous connection performance assessment
- **Priority-Based Selection**: Intelligent connection type prioritization

## 💻 Frontend Implementation (`web/chat.js`)

The frontend implements automatic connection management and fallback strategies:

### Core Functions
- **`joinRoom()`**: Hides init UI, shows chat interface, displays HTTP deep-link invite
- **`pollHistory()`**: Fetches new messages via REST API and appends to chat view
- **`sendMessage()`**: Posts messages to `/send` endpoint with optimistic rendering

### Connection Strategy
- Automatically attempts WebRTC connections first for optimal performance
- Falls back to HTTP polling if P2P connections fail
- Generates shareable deep-link invites: `http://host:8080/?room=name&peer_id=id`

## 📁 Project Structure

```
E-Goat/
├── cmd/
│   └── messanger/
│       ├── main.go                    # Application entry point
│       ├── transport_integration.go   # Transport layer integration
│       └── web/
│           ├── index.html             # Chat UI
│           ├── chat.js                # Frontend logic
│           └── styles.css             # Styling
├── internal/
│   ├── signaling/
│   │   └── server.go                  # WebSocket signaling hub
│   ├── storage/
│   │   └── sqlite.go                  # SQLite database management
│   ├── transport/                     # Layered connection architecture
│   │   ├── connection.go              # Core interfaces and manager
│   │   ├── webrtc_stun.go            # WebRTC STUN connections
│   │   ├── webrtc_turn.go            # WebRTC TURN connections
│   │   ├── websocket_direct.go       # Direct WebSocket connections
│   │   ├── http_polling.go           # HTTP polling fallback
│   │   ├── lan_broadcast.go          # LAN broadcast discovery
│   │   └── manager.go                # Transport manager wrapper
│   └── webrtc/
│       └── peer.go                   # WebRTC peer connection logic
├── scripts/                          # Testing and automation scripts
│   ├── build-verify.sh               # Build verification
│   ├── quick-test.sh                 # Quick functionality test
│   ├── test-e2e.sh                   # End-to-end communication test
│   ├── run-tests.sh                  # Main test orchestrator
│   ├── cleanup.sh                    # Test cleanup
│   └── README.md                     # Testing documentation
├── tests/
│   └── transport_test.go             # Unit tests for transport layer
├── .github/
│   └── workflows/
│       └── ci.yml                    # GitHub Actions CI/CD
├── Makefile                          # Build automation
├── go.mod                            # Go module definition
├── go.sum                            # Go module checksums
└── README.md                         # This file
```

## 🤝 Contributing

We welcome contributions! Please follow these guidelines:

1. **Open an Issue**: For significant changes, open an issue first to discuss
2. **Fork the Repository**: Create your own fork of the project
3. **Create a Branch**: Use a descriptive branch name for your changes
4. **Write Tests**: Ensure your changes include appropriate tests
5. **Update Documentation**: Update README and comments as needed
6. **Submit a Pull Request**: Include a clear description of your changes

### Development Workflow
```bash
# Fork and clone the repository
git clone https://github.com/your-username/E-Goat.git
cd E-Goat

# Create a feature branch
git checkout -b feature/your-feature-name

# Make your changes and test them
make test

# Commit and push your changes
git commit -m "Add your feature description"
git push origin feature/your-feature-name

# Create a pull request on GitHub
```

## 📄 License

E-Goat is licensed under the [MIT License](LICENSE).

## 🎯 Roadmap

### Current Focus
- [ ] **Frontend WebRTC Integration** - Complete JavaScript WebRTC implementation
- [ ] **Transport Layer Integration** - Connect layered architecture to main application

### Near-Term Goals  
- [ ] **Mobile-Responsive UI** - Improved mobile experience
- [ ] **File Sharing** - Binary file transfer over DataChannels
- [ ] **End-to-End Encryption** - Additional security layer

### Future Vision
- [ ] **Voice/Video Chat** - WebRTC media channels
- [ ] **Docker Support** - Containerized deployment
- [ ] **Plugin System** - Extensible architecture
- [ ] **Federation** - Cross-instance communication

## 🔗 External Access & Sharing

Once port-forwarding and firewall rules are configured, you can share external links:

```
http://YOUR_WAN_IP:8080/?room=myroom&peer_id=XYZ
```

This allows external users to join your chat rooms directly through your public IP address.

---

**Made with ❤️ in Go** - E-Goat provides reliable peer-to-peer communication with automatic network adaptation, ensuring your messages get through no matter the network conditions.
