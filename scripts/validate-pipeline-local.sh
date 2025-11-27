#!/bin/bash
# validate-pipeline-local.sh
# Mirrors CI/CD pipeline validation exactly for local testing
# CNPG Storage Manager - Kubebuilder Project

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters for summary
TOTAL_CHECKS=0
PASSED_CHECKS=0
FAILED_CHECKS=0

# Components to validate
COMPONENTS=("api" "controllers" "pkg")

# Parse command line arguments
SPECIFIC_COMPONENT=""
QUICK_MODE=false
SKIP_TESTS=false

usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Validates the CNPG Storage Manager using the same checks as CI/CD pipeline.

OPTIONS:
    --component=NAME    Validate only specific component (api, controllers, pkg)
    --quick             Skip tests and gosec (format, vet, staticcheck only)
    --skip-tests        Skip tests but run all other checks
    -h, --help          Show this help message

EXAMPLES:
    $0                              # Full validation
    $0 --component=controllers      # Validate only controllers
    $0 --quick                      # Quick validation (no tests)

EOF
}

# Parse arguments
for arg in "$@"; do
    case $arg in
        --component=*)
            SPECIFIC_COMPONENT="${arg#*=}"
            ;;
        --quick)
            QUICK_MODE=true
            ;;
        --skip-tests)
            SKIP_TESTS=true
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $arg${NC}"
            usage
            exit 1
            ;;
    esac
done

# Validate specific component if provided
if [ -n "$SPECIFIC_COMPONENT" ]; then
    VALID_COMPONENT=false
    for comp in "${COMPONENTS[@]}"; do
        if [ "$comp" = "$SPECIFIC_COMPONENT" ]; then
            VALID_COMPONENT=true
            break
        fi
    done

    if [ "$VALID_COMPONENT" = false ]; then
        echo -e "${RED}Error: Invalid component '$SPECIFIC_COMPONENT'${NC}"
        echo "Valid components: ${COMPONENTS[*]}"
        exit 1
    fi

    COMPONENTS=("$SPECIFIC_COMPONENT")
fi

# Print header
echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║       CNPG Storage Manager - Local Pipeline Validation         ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${BLUE}Components:${NC} ${COMPONENTS[*]}"
echo -e "${BLUE}Quick Mode:${NC} $QUICK_MODE"
echo -e "${BLUE}Skip Tests:${NC} $SKIP_TESTS"
echo ""

# Check for required tools
echo -e "${BLUE}Checking required tools...${NC}"
MISSING_TOOLS=()

if ! command -v gofmt &> /dev/null; then
    MISSING_TOOLS+=("gofmt")
fi

if ! command -v go &> /dev/null; then
    MISSING_TOOLS+=("go")
else
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    REQUIRED_VERSION="1.21"
    if [[ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]]; then
        echo -e "${YELLOW}Warning: Go version $GO_VERSION detected. Requires Go $REQUIRED_VERSION+${NC}"
    fi
fi

if ! command -v staticcheck &> /dev/null; then
    echo -e "${YELLOW}Installing staticcheck...${NC}"
    go install honnef.co/go/tools/cmd/staticcheck@latest
fi

if [ "$QUICK_MODE" = false ] && ! command -v gosec &> /dev/null; then
    echo -e "${YELLOW}Installing gosec...${NC}"
    go install github.com/securego/gosec/v2/cmd/gosec@latest
fi

if [ ${#MISSING_TOOLS[@]} -gt 0 ]; then
    echo -e "${RED}Error: Missing required tools: ${MISSING_TOOLS[*]}${NC}"
    exit 1
fi

echo -e "${GREEN}✓ All required tools available${NC}"
echo ""

# Function to run a validation check
run_check() {
    local check_name=$1
    local command=$2

    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))

    echo -e "${BLUE}Running $check_name...${NC}"

    if eval "$command" > /tmp/validate_${check_name}.log 2>&1; then
        echo -e "${GREEN}✓ $check_name passed${NC}"
        PASSED_CHECKS=$((PASSED_CHECKS + 1))
        return 0
    else
        echo -e "${RED}✗ $check_name FAILED${NC}"
        cat /tmp/validate_${check_name}.log
        FAILED_CHECKS=$((FAILED_CHECKS + 1))
        return 1
    fi
}

# Main validation
echo -e "${BLUE}════════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}Running Validation Checks${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════════════${NC}"
echo ""

# Step 1: Generate manifests and code (kubebuilder)
echo -e "${BLUE}Step 1: Checking generated code...${NC}"
run_check "manifests" "make manifests" || true
run_check "generate" "make generate" || true

# Step 2: Check for uncommitted generated changes
echo -e "${BLUE}Step 2: Checking for uncommitted changes...${NC}"
if git diff --quiet 2>/dev/null; then
    echo -e "${GREEN}✓ No uncommitted changes in generated files${NC}"
    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
    PASSED_CHECKS=$((PASSED_CHECKS + 1))
else
    echo -e "${YELLOW}⚠ Generated files have uncommitted changes${NC}"
    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
fi
echo ""

# Step 3: Format check
echo -e "${BLUE}Step 3: Format check...${NC}"
run_check "gofmt" "test -z \"\$(gofmt -l . | grep -v vendor)\"" || true
echo ""

# Step 4: Go vet
echo -e "${BLUE}Step 4: Go vet...${NC}"
run_check "go-vet" "go vet ./..." || true
echo ""

# Step 5: Staticcheck
echo -e "${BLUE}Step 5: Staticcheck...${NC}"
run_check "staticcheck" "staticcheck ./..." || true
echo ""

# Step 6: Gosec (skip in quick mode)
if [ "$QUICK_MODE" = false ]; then
    echo -e "${BLUE}Step 6: Security scan (gosec)...${NC}"
    run_check "gosec" "gosec -quiet -exclude=G104,G304 ./..." || true
    echo ""
fi

# Step 7: Unit tests (skip in quick mode or skip-tests)
if [ "$QUICK_MODE" = false ] && [ "$SKIP_TESTS" = false ]; then
    echo -e "${BLUE}Step 7: Unit tests...${NC}"
    run_check "unit-tests" "go test -v -race -coverprofile=coverage-unit.out ./pkg/..." || true
    echo ""

    # Step 8: Integration tests with envtest
    echo -e "${BLUE}Step 8: Integration tests (envtest)...${NC}"
    run_check "integration-tests" "make test-integration" || true
    echo ""
fi

# Step 9: Build check
echo -e "${BLUE}Step 9: Build check...${NC}"
run_check "build" "go build -o /dev/null ./cmd/..." || true
echo ""

# Cleanup temp files
rm -f /tmp/validate_*.log

# Print summary
echo -e "${BLUE}════════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}Validation Summary${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "Total checks:  $TOTAL_CHECKS"
echo -e "${GREEN}Passed:        $PASSED_CHECKS${NC}"
echo -e "${RED}Failed:        $FAILED_CHECKS${NC}"
echo ""

# Exit with appropriate code
if [ $FAILED_CHECKS -eq 0 ]; then
    echo -e "${GREEN}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║  ✓ All validations passed! Safe to push.                      ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════════════════════╝${NC}"
    exit 0
else
    echo -e "${RED}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║  ✗ Validation failed! Fix issues before pushing.              ║${NC}"
    echo -e "${RED}╚════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${YELLOW}See logs above for details.${NC}"
    echo -e "${YELLOW}Run with --quick for faster feedback (skips tests).${NC}"
    exit 1
fi
