#!/bin/bash

# Main test runner for E-Goat
# This script runs all verification and testing procedures

set -e  # Exit on any error

echo "🎯 E-Goat Test Runner"
echo "===================="

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

# Make scripts executable
chmod +x scripts/*.sh

# Parse command line arguments
QUICK_MODE=false
SKIP_BUILD=false
SKIP_E2E=false
INTERACTIVE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --quick)
            QUICK_MODE=true
            shift
            ;;
        --skip-build)
            SKIP_BUILD=true
            shift
            ;;
        --skip-e2e)
            SKIP_E2E=true
            shift
            ;;
        --interactive)
            INTERACTIVE=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --quick       Run only quick tests"
            echo "  --skip-build  Skip build verification"
            echo "  --skip-e2e    Skip end-to-end tests"
            echo "  --interactive Run in interactive mode"
            echo "  -h, --help    Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                    # Run all tests"
            echo "  $0 --quick           # Run only quick tests"
            echo "  $0 --skip-e2e        # Run build verification but skip e2e"
            echo "  $0 --interactive     # Run with user prompts"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use -h or --help for usage information"
            exit 1
            ;;
    esac
done

# Print test plan
echo -e "${BLUE}Test Plan:${NC}"
if [ "$SKIP_BUILD" = false ]; then
    echo "  ✓ Build verification"
fi
if [ "$QUICK_MODE" = true ]; then
    echo "  ✓ Quick functionality test"
elif [ "$SKIP_E2E" = false ]; then
    echo "  ✓ End-to-end communication test"
fi
echo ""

# Function to ask for user confirmation in interactive mode
ask_continue() {
    if [ "$INTERACTIVE" = true ]; then
        read -p "Continue? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo "Aborted by user"
            exit 0
        fi
    fi
}

# Function to run a test script with error handling
run_test() {
    local script_name="$1"
    local description="$2"
    
    echo -e "\n${BLUE}🔄 Running: $description${NC}"
    echo "Script: $script_name"
    
    if [ "$INTERACTIVE" = true ]; then
        ask_continue
    fi
    
    start_time=$(date +%s)
    
    if bash "$script_name"; then
        end_time=$(date +%s)
        duration=$((end_time - start_time))
        echo -e "${GREEN}✅ $description completed successfully (${duration}s)${NC}"
        return 0
    else
        end_time=$(date +%s)
        duration=$((end_time - start_time))
        echo -e "${RED}❌ $description failed (${duration}s)${NC}"
        return 1
    fi
}

# Start testing
total_start_time=$(date +%s)
failed_tests=0
total_tests=0

# Cleanup any previous test artifacts
echo -e "${YELLOW}🧹 Initial cleanup...${NC}"
bash "$SCRIPT_DIR/cleanup.sh" || true

# Build verification
if [ "$SKIP_BUILD" = false ]; then
    total_tests=$((total_tests + 1))
    if ! run_test "$SCRIPT_DIR/build-verify.sh" "Build verification"; then
        failed_tests=$((failed_tests + 1))
        if [ "$INTERACTIVE" = false ]; then
            echo -e "${RED}Build verification failed. Cannot continue.${NC}"
            exit 1
        fi
    fi
fi

# Quick test or E2E test
if [ "$QUICK_MODE" = true ]; then
    total_tests=$((total_tests + 1))
    if ! run_test "$SCRIPT_DIR/quick-test.sh" "Quick functionality test"; then
        failed_tests=$((failed_tests + 1))
    fi
elif [ "$SKIP_E2E" = false ]; then
    total_tests=$((total_tests + 1))
    echo -e "\n${YELLOW}⚠️  The E2E test will start two instances and keep them running.${NC}"
    echo -e "${YELLOW}   Press Ctrl+C in the E2E test to stop the instances.${NC}"
    
    if [ "$INTERACTIVE" = true ]; then
        ask_continue
    fi
    
    if ! run_test "$SCRIPT_DIR/test-e2e.sh" "End-to-end communication test"; then
        failed_tests=$((failed_tests + 1))
    fi
fi

# Final cleanup
echo -e "\n${YELLOW}🧹 Final cleanup...${NC}"
bash "$SCRIPT_DIR/cleanup.sh" || true

# Calculate total test time
total_end_time=$(date +%s)
total_duration=$((total_end_time - total_start_time))

# Print final results
echo ""
echo "📊 Test Summary"
echo "==============="
echo "Total tests run: $total_tests"
echo "Tests passed: $((total_tests - failed_tests))"
echo "Tests failed: $failed_tests"
echo "Total time: ${total_duration}s"

if [ $failed_tests -eq 0 ]; then
    echo -e "\n${GREEN}🎉 All tests passed successfully!${NC}"
    echo ""
    echo "Your E-Goat application is working correctly and ready for use."
    echo ""
    echo "To start using E-Goat:"
    echo "  ./e-goat"
    echo ""
    echo "Then open http://localhost:8080 in your browser."
    exit 0
else
    echo -e "\n${RED}❌ Some tests failed.${NC}"
    echo ""
    echo "Please review the test output above and fix any issues."
    echo "You can run individual test scripts for debugging:"
    echo "  ./scripts/build-verify.sh"
    echo "  ./scripts/quick-test.sh"
    echo "  ./scripts/test-e2e.sh"
    exit 1
fi
