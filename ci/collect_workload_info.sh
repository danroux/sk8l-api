#!/bin/bash

namespace=$1
cronjobs=$(kubectl get cronjobs -n "$namespace" -o json)
cronjobs_count=$(echo "$cronjobs" | jq -r '.items | length')

echo "# HELP sk8l_cronjobs_count Current count of cronjobs grouped by namespace and status (registered, running, failing, completed)."
echo "# TYPE sk8l_cronjobs_count gauge"
echo "sk8l_cronjobs_count{namespace=\"${namespace}\",status=\"registered\"} ${cronjobs_count}"
echo "sk8l_cronjobs_count{namespace=\"${namespace}\",status=\"failing\"} 0"

echo "# HELP sk8l_cronjob_info Metadata and configuration for a cronjob (constant value of 1)."
echo "# TYPE sk8l_cronjob_info gauge"

echo "# HELP sk8l_cronjob_last_run_duration_seconds Duration in seconds of the latest execution of a cronjob."
echo "# TYPE sk8l_cronjob_last_run_duration_seconds gauge"

echo "# HELP sk8l_cronjob_last_run_status Status of the last execution of a cronjob (1 if true, 0 if false)."
echo "# TYPE sk8l_cronjob_last_run_status gauge"

echo "# HELP sk8l_cronjob_jobs_count Count of retained child jobs for a cronjob by state (succeeded, failed)."
echo "# TYPE sk8l_cronjob_jobs_count gauge"

echo "# HELP sk8l_cronjob_active_jobs Number of currently active child jobs for a cronjob."
echo "# TYPE sk8l_cronjob_active_jobs gauge"

while read -r cronjob_json; do
  cronjob_name=$(echo "$cronjob_json" | jq -r '.metadata.name')
  echo "sk8l_cronjob_info{cronjob=\"${cronjob_name}\",namespace=\"${namespace}\""
  echo "sk8l_cronjob_last_run_status{cronjob=\"${cronjob_name}\",namespace=\"${namespace}\",status=\"failed\"} 0"
  echo "sk8l_cronjob_last_run_status{cronjob=\"${cronjob_name}\",namespace=\"${namespace}\",status=\"succeeded\"} 1"
  echo "sk8l_cronjob_active_jobs{cronjob=\"${cronjob_name}\",namespace=\"${namespace}\"}"
  echo "sk8l_cronjob_jobs_count{cronjob=\"${cronjob_name}\",namespace=\"${namespace}\",status=\"succeeded\"}"
  echo "sk8l_cronjob_jobs_count{cronjob=\"${cronjob_name}\",namespace=\"${namespace}\",status=\"failed\"} 0"
  echo "sk8l_cronjob_last_run_duration_seconds{cronjob=\"${cronjob_name}\",namespace=\"${namespace}\"}"
done < <(echo "$cronjobs" | tr -d '[:cntrl:]' | jq -c '.items[]')

