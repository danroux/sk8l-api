#!/bin/bash

# Function to check if lines from one file exist in another file
check_strings_in_file() {
    local current_state="$1"
    local expected_output="$2"
    local line_number=1

    while IFS= read -r line || [ -n "$line" ]; do
        [ -z "$line" ] && continue
        if ! grep -F -q "$line" "$current_state"; then
            echo "Error: '$line' (line $line_number) not found in '$current_state'"
            return 1
        fi
        ((line_number++))
    done < "$expected_output"

    echo "All lines were found in $current_state."
    return 0
}

current_state="${1:-current_state.txt}"
expected_output="${2:-expected_output.txt}"

if [ ! -s "$current_state" ]; then
    echo "Error: '$current_state' is missing or empty"
    exit 1
fi

if [ ! -s "$expected_output" ]; then
    echo "Error: '$expected_output' is missing or empty"
    exit 1
fi

echo "Checking if lines in '$expected_output' exist in '$current_state':"

if ! check_strings_in_file "$current_state" "$expected_output"; then
    exit 1
fi

echo "Smoke tests passed successfully."
