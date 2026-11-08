#!/bin/bash

set -e

CA_CERT_FILE="ca-cert.pem"
EXPECTED_ANN_COUNT=13
EXPECTED_CRONJOBS_COUNT=3
EXPECTED_JOBS_COUNT=1

echo "Running test..."

kubectl get secrets -n sk8l sk8l-ca-root-cert-secret -o "jsonpath={.data['ca\.crt']}" | base64 --decode > $CA_CERT_FILE

ann=$(grpcurl -cacert $CA_CERT_FILE -import-path protos -proto sk8l.proto -d '{}' localhost:8590 sk8l.Cronjob/GetDashboardAnnotations | jq -rc '.annotations' | jq '[.panels[] | select(.targets != null) | .targets[].expr]')
ann_length=$(echo "$ann" | jq 'length')

if [ "$ann_length" -ne "$EXPECTED_ANN_COUNT" ]; then
  echo "Error: Expected $EXPECTED_ANN_COUNT .targets[].expr values, but got $ann_length"
  exit 1
else
  echo "Success: $ann_length .targets[].expr values found, as expected."
fi

echo "----------------------------"

grpcurl -cacert $CA_CERT_FILE -import-path protos -proto sk8l.proto -max-time 2 -d '{}' localhost:8590 sk8l.Cronjob/GetCronjobs > cronjobs.json 2>/dev/null || true

cronjobs_count=$(jq -s 'last | [.cronjobs[].name] | length' cronjobs.json)

if [ "$cronjobs_count" -ne "$EXPECTED_CRONJOBS_COUNT" ]; then
  echo "Error: Expected $EXPECTED_CRONJOBS_COUNT cronjobs, but got $cronjobs_count"
  exit 1
else
  echo "Success: $cronjobs_count cronjobs found, as expected."
fi

echo "----------------------------"

grpcurl -cacert $CA_CERT_FILE -import-path protos -proto sk8l.proto -max-time 2 -d '{}' localhost:8590 sk8l.Cronjob/GetJobs > jobs.json 2>/dev/null || true

jq -s 'last | [.jobs[].name]' jobs.json > jobs_names.json
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

connect_ann=$(curl --cacert $CA_CERT_FILE -s -f -X POST -H "Content-Type: application/json" -d '{}' https://localhost:8590/sk8l.Cronjob/GetDashboardAnnotations | jq -rc '.annotations' | jq '[.panels[] | select(.targets != null) | .targets[].expr]')
connect_ann_length=$(echo "$connect_ann" | jq 'length')

if [ "$connect_ann_length" -ne "$EXPECTED_ANN_COUNT" ]; then
  echo "Error: ConnectRPC expected $EXPECTED_ANN_COUNT .targets[].expr values, but got $connect_ann_length"
  exit 1
else
  echo "Success: ConnectRPC on :8590 returned $connect_ann_length .targets[].expr values, as expected."
fi

echo "----------------------------"

ui_html=$(curl --cacert $CA_CERT_FILE -s -f https://localhost:8590/)
if ! echo "$ui_html" | grep -q '<div id="app"></div>'; then
  echo "Error: Embedded UI index.html not served on https://localhost:8590/"
  exit 1
fi

spa_html=$(curl --cacert $CA_CERT_FILE -s -f https://localhost:8590/jobs/sk8l)
if ! echo "$spa_html" | grep -q '<div id="app"></div>'; then
  echo "Error: Embedded UI SPA fallback failed on https://localhost:8590/jobs/sk8l"
  exit 1
fi

js_asset=$(echo "$ui_html" | grep -oE '/assets/index-[^"]+\.js' | head -n 1)
if [ -z "$js_asset" ]; then
  echo "Error: Could not find /assets/index-*.js reference in index.html"
  exit 1
fi

js_body=$(curl --cacert $CA_CERT_FILE -s -f "https://localhost:8590${js_asset}")
if echo "$js_body" | grep -qE 'VITE_SK8L_K8_NAMESPACE|VITE_SK8L_API_URL'; then
  echo "Error: Unreplaced VITE_SK8L_* placeholder found in ${js_asset}"
  exit 1
fi

echo "Success: Embedded UI and runtime config injection verified on :8590."
echo "----------------------------"
