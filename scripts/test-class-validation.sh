#!/bin/bash

# Test class validation for all D&D 5e classes
# This helps verify validation logic when adding new classes

set -e

CLIENT="${CLIENT:-./bin/rpg-client}"
SERVER="${SERVER:-localhost:50051}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "Testing class validation..."
echo "=========================="
echo ""

# List of all classes to test
CLASSES=(
    "fighter"
    "wizard"
    "cleric"
    "rogue"
    "ranger"
    "paladin"
    "barbarian"
    "bard"
    "druid"
    "monk"
    "sorcerer"
    "warlock"
)

# Test each class
for CLASS in "${CLASSES[@]}"; do
    echo -n "Testing $CLASS... "
    
    # Run validation and capture warnings
    RESULT=$($CLIENT character validate \
        --server="$SERVER" \
        --class="$CLASS" \
        --race="human" \
        --format=json 2>/dev/null)
    
    # Check if there are warnings
    WARNINGS=$(echo "$RESULT" | jq -r '.warnings')
    
    if [ "$WARNINGS" == "null" ]; then
        echo -e "${YELLOW}⚠${NC}  No validation implemented"
    else
        WARNING_COUNT=$(echo "$RESULT" | jq '.warnings | length')
        if [ "$WARNING_COUNT" -gt 0 ]; then
            echo -e "${GREEN}✓${NC} Validation active ($WARNING_COUNT warnings)"
            
            # Show warnings if verbose mode
            if [ "$1" == "-v" ] || [ "$1" == "--verbose" ]; then
                echo "$RESULT" | jq -r '.warnings[] | "    • [\(.field)] \(.message)"'
            fi
        else
            echo -e "${GREEN}✓${NC} Fully configured (no warnings)"
        fi
    fi
done

echo ""
echo "Summary:"
echo "--------"

# Count classes with validation
VALIDATED=0
for CLASS in "${CLASSES[@]}"; do
    RESULT=$($CLIENT character validate \
        --server="$SERVER" \
        --class="$CLASS" \
        --race="human" \
        --format=json 2>/dev/null)
    
    WARNINGS=$(echo "$RESULT" | jq -r '.warnings')
    if [ "$WARNINGS" != "null" ]; then
        VALIDATED=$((VALIDATED + 1))
    fi
done

echo "Classes with validation: $VALIDATED/${#CLASSES[@]}"

# Special test: Fighter with invalid fighting style
echo ""
echo "Special Tests:"
echo "--------------"
echo -n "Fighter with invalid data... "

# This would require extending the client to pass choices, so just note it
echo -e "${YELLOW}⚠${NC}  Requires client extension to test invalid choices"