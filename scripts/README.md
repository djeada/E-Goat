# E-Goat Testing Scripts

This directory contains scripts to verify the build and test end-to-end communication of the E-Goat messaging application.

## Scripts Overview

### 🎯 `run-tests.sh` - Main Test Runner
The primary script that orchestrates all testing procedures.

**Usage:**
```bash
./scripts/run-tests.sh [OPTIONS]
```

**Options:**
- `--quick`: Run only quick tests (faster, basic functionality)
- `--skip-build`: Skip build verification
- `--skip-e2e`: Skip end-to-end tests
- `--interactive`: Run with user prompts between tests
- `-h, --help`: Show help message

**Examples:**
```bash
./scripts/run-tests.sh                    # Run all tests
./scripts/run-tests.sh --quick           # Run only quick tests
./scripts/run-tests.sh --skip-e2e        # Run build verification but skip e2e
./scripts/run-tests.sh --interactive     # Run with user prompts
```

### 🔨 `build-verify.sh` - Build Verification
Verifies that the project builds correctly and performs basic checks.

**What it does:**
- Checks Go installation
- Initializes Go modules if needed
- Downloads dependencies
- Builds the e-goat binary
- Runs static analysis (go vet, go fmt)
- Verifies binary functionality

**Usage:**
```bash
./scripts/build-verify.sh
```

### ⚡ `quick-test.sh` - Quick Functionality Test
Performs a fast build and basic functionality test.

**What it does:**
- Quick build test
- Starts single instance
- Tests HTTP endpoints
- Tests REST API (send/receive messages)
- Minimal verification

**Usage:**
```bash
./scripts/quick-test.sh
```

### 🔗 `test-e2e.sh` - End-to-End Communication Test
Comprehensive test that starts two instances and tests communication between them.

**What it does:**
- Starts two E-Goat instances on different ports
- Tests HTTP endpoints on both instances
- Tests WebSocket connectivity
- Tests REST API communication between instances
- Tests message persistence
- Tests UI accessibility
- Keeps instances running for manual testing

**Usage:**
```bash
./scripts/test-e2e.sh
```

**Instance Configuration:**
- **Instance 1**: HTTP:8080, WebSocket:9000, DB:test_chat_1.db
- **Instance 2**: HTTP:8081, WebSocket:9001, DB:test_chat_2.db

**Manual Testing URLs:**
- Instance 1: http://localhost:8080/?room=e2e-test-room&peer_id=test-peer-1
- Instance 2: http://localhost:8081/?room=e2e-test-room&peer_id=test-peer-2

### 🧹 `cleanup.sh` - Cleanup Script
Cleans up test artifacts and stops any running instances.

**What it does:**
- Stops all running E-Goat processes
- Removes test databases
- Cleans up temporary files
- Frees up test ports
- Verifies cleanup completion

**Usage:**
```bash
./scripts/cleanup.sh
```

## Quick Start

For a complete verification of your E-Goat installation:

```bash
# Make scripts executable (first time only)
chmod +x scripts/*.sh

# Run all tests
./scripts/run-tests.sh

# Or run quick tests only
./scripts/run-tests.sh --quick
```

## Testing Workflow

### 1. Basic Verification
```bash
./scripts/build-verify.sh
./scripts/quick-test.sh
```

### 2. Full End-to-End Testing
```bash
./scripts/test-e2e.sh
```

### 3. Manual Testing
When the E2E test is running, open these URLs in separate browser tabs:
- http://localhost:8080/?room=e2e-test-room&peer_id=test-peer-1
- http://localhost:8081/?room=e2e-test-room&peer_id=test-peer-2

Try sending messages between the instances to verify real-time communication.

### 4. Cleanup
```bash
./scripts/cleanup.sh
```

## Troubleshooting

### Port Already in Use
If you get "port already in use" errors:
```bash
./scripts/cleanup.sh
# Wait a few seconds, then retry
```

### Build Failures
1. Ensure Go is installed and in PATH
2. Check Go version compatibility
3. Run `go mod tidy` manually
4. Check for network connectivity (for downloading dependencies)

### Test Failures
1. Check if required ports (8080, 8081, 9000, 9001) are available
2. Verify no other E-Goat instances are running
3. Check system resources (disk space, memory)
4. Review test output for specific error messages

### WebSocket Test Warnings
The message "WebSocket port test failed (might be normal)" is not necessarily an error. WebSocket ports might not respond to simple TCP connection tests but still work correctly for WebSocket connections.

## Test Ports

The scripts use the following ports for testing:
- **8080**: Production HTTP port / Instance 1 HTTP
- **8081**: Instance 2 HTTP
- **8082**: Quick test HTTP
- **9000**: Production WebSocket port / Instance 1 WebSocket
- **9001**: Instance 2 WebSocket
- **9002**: Quick test WebSocket

## Test Databases

Temporary databases created during testing:
- `test_chat_1.db`: Instance 1 database
- `test_chat_2.db`: Instance 2 database
- `quick_test.db`: Quick test database

These are automatically cleaned up after tests complete.

## Integration with CI/CD

For continuous integration, use:
```bash
./scripts/run-tests.sh --quick
```

This provides fast feedback without the interactive E2E test that requires manual intervention.
