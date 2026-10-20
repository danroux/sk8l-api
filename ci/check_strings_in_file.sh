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

current_state="current_state.txt"
expected_output="expected_output.txt"

echo "Checking if lines in '$expected_output' exist in '$current_state':"

if ! check_strings_in_file "$current_state" "$expected_output"; then
    exit 1
fi

pattern='sk8l_cronjob_last_run_duration_seconds{cronjob="process-videos",namespace="sk8l"}'

count=$(grep -c "$pattern" "$current_state")

if [ "$count" -lt 1 ]; then
    echo "Error: Pattern '$pattern' not found in $current_state"
    exit 1
fi

echo "Smoke tests passed successfully."
