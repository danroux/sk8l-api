#!/bin/bash

set -e

CA_CERT_FILE="ca-cert.pem"
EXPECTED_ANN_COUNT=19
EXPECTED_CRONJOBS_COUNT=3
EXPECTED_JOBS_COUNT=1

echo "Running test..."

kubectl get secrets -n sk8l sk8l-ca-root-cert-secret -o "jsonpath={.data['ca\.crt']}" | base64 --decode > $CA_CERT_FILE

ann=$(grpcurl -cacert $CA_CERT_FILE -import-path protos -proto sk8l.proto -d '{}' localhost:9080 sk8l.Cronjob/GetDashboardAnnotations | jq -rc '.annotations' | jq '[.panels[] | select(.targets != null) | .targets[].expr]')
ann_length=$(echo "$ann" | jq 'length')

if [ "$ann_length" -ne "$EXPECTED_ANN_COUNT" ]; then
  echo "Error: Expected $EXPECTED_ANN_COUNT .targets[].expr values, but got $ann_length"
  exit 1
else
  echo "Success: $ann_length .targets[].expr values found, as expected."
fi

echo "----------------------------"

grpcurl -cacert $CA_CERT_FILE -import-path protos -proto sk8l.proto -d '{}' localhost:9080 sk8l.Cronjob/GetCronjobs > cronjobs.json &
pid=$!
sleep 5
kill $pid

cronjobs_count=$(jq -r '[.cronjobs[].name] | length' cronjobs.json)

if [ "$cronjobs_count" -ne "$EXPECTED_CRONJOBS_COUNT" ]; then
  echo "Error: Expected $EXPECTED_CRONJOBS_COUNT cronjobs, but got $cronjobs_count"
  exit 1
else
  echo "Success: $cronjobs_count cronjobs found, as expected."
fi

echo "----------------------------"

grpcurl -cacert $CA_CERT_FILE -import-path protos -proto sk8l.proto -d '{}' localhost:9080 sk8l.Cronjob/GetJobs > jobs.json &
pid=$!
sleep 5
kill $pid

jq -r '[.jobs[].name]' jobs.json > jobs_names.json
if grep -q "sk8l-demo-job" jobs_names.json; then
  echo "Test passed"
else
  echo "Test failed"
  echo "Error: 'sk8l-demo-job' not found in jobs_names.json"
  exit 1
fi

jobs_count=$(jq length jobs_names.json)

if [ "$jobs_count" -ne "$EXPECTED_JOBS_COUNT" ]; then
  echo "Error: Expected $EXPECTED_JOBS_COUNT jobs, but got $jobs_count"
  exit 1
else
  echo "Success: $jobs_count jobs found, as expected."
fi

echo "----------------------------"
