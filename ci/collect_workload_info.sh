#!/bin/bash

namespace=$1
jobs=$(kubectl get jobs -n "$namespace" -o json)
cronjobs=$(kubectl get cronjobs -n "$namespace" -o json)

while read -r cronjob_json; do
  cronjob_name=$(echo "$cronjob_json" | jq -r '.metadata.name' | tr "-" "_")
  echo "# HELP sk8l_${namespace}_${cronjob_name}_duration_seconds Duration of ${cronjob_name} in seconds"
  echo "# TYPE sk8l_${namespace}_${cronjob_name}_duration_seconds gauge"
  echo "${namespace}_${cronjob_name}_duration_seconds"
  echo "# HELP sk8l_${namespace}_${cronjob_name}_failure_total ${cronjob_name} failure total"
  echo "# TYPE sk8l_${namespace}_${cronjob_name}_failure_total gauge"
  echo "${namespace}_${cronjob_name}_failure_total"
  echo "# HELP sk8l_${namespace}_${cronjob_name}_completion_total ${cronjob_name} completion total"
  echo "# TYPE sk8l_${namespace}_${cronjob_name}_completion_total gauge"
  echo "${namespace}_${cronjob_name}_completion_total"
  echo "# HELP sk8l_${namespace}_registered_cronjobs_total "
  echo "# TYPE sk8l_${namespace}_registered_cronjobs_total gauge"
  echo "sk8l_${namespace}_registered_cronjobs_total 3"
  echo "# HELP sk8l_${namespace}_running_cronjobs_total"
  echo "# TYPE sk8l_${namespace}_running_cronjobs_total gauge"
  echo "sk8l_${namespace}_running_cronjobs_total"
  echo "# HELP sk8l_${namespace}_failing_cronjobs_total"
  echo "# TYPE sk8l_${namespace}_failing_cronjobs_total gauge"
  echo "sk8l_${namespace}_failing_cronjobs_total 0"
done < <(echo "$cronjobs" | tr -d '[:cntrl:]' | jq -c '.items[]')

echo "sk8l_${namespace}_download_report_files_duration_seconds{job_name=\"download-report-files-"
echo "sk8l_${namespace}_process_csv_files_duration_seconds{job_name=\"process-csv-files-"
echo "sk8l_${namespace}_process_videos_duration_seconds{job_name=\"process-videos-"

while read -r job_json; do
  job_name=$(echo "$job_json" | jq -r '.metadata.name')
  cronjob_name=$(echo "$job_json" | jq -r '.metadata.ownerReferences[0].name' | tr "-" "_")

  echo "sk8l_${namespace}_${cronjob_name}_duration_seconds{job_name=\"${job_name}\"}"
done < <(echo "$jobs" | jq -c '.items[]')
