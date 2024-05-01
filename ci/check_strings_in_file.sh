#!/bin/bash

# Function to check if lines from one file exist in another file
check_strings_in_file() {
    local current_state="$1"
    local expected_output="$2"
    local line_number=1

    while read -r line; do
        if ! grep -q "$line" "$current_state"; then
            echo "Error: '$line' (line $line_number) not found in the file '$expected_output'"
            return 1
        fi
        ((line_number++))
    done < "$expected_output"

    echo "All lines were found in the expected output file."
    return 0
}

current_state="current_state.txt"
expected_output="expected_output.txt"

echo "Checking if lines in '$expected_output' exist in '$current_state':"

if ! check_strings_in_file "$current_state" "$expected_output"; then
    exit 1
fi

pattern="sk8l_sk8l_process_videos_duration_seconds{job_name=\"process-videos-[0-9]*\"\\}"

count=$(cat "$current_state" | grep -c "$pattern")

if [ $count -le 1 ]; then
    echo "Error: Found only one or fewer matches($count) for the pattern: $pattern"
    exit 1
fi
