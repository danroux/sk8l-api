#!/bin/bash

# Check if the argument is provided
if [ -z "$1" ]; then
  echo "Usage: $0 <pr_image_tag>"
  exit 1
fi

pr_image_tag="$1"
current_version=$(awk -F'"' '/\.Values\.sk8lApi\.imageTag/ {print $2}' charts/sk8l/templates/deployment.yaml)
sed -i.bak -E "s/.Values.sk8lApi.imageTag \| default \"$current_version\"/.Values.sk8lApi.imageTag | default \"$pr_image_tag\"/" charts/sk8l/templates/deployment.yaml
cp charts/sk8l/values.yaml charts/sk8l/values.yaml.bak
echo $current_version
yq e -i ".sk8lApi.imageTag = \"$pr_image_tag\"" charts/sk8l/values.yaml
set +e
echo "after"
diff -C3 charts/sk8l/templates/deployment.yaml.bak charts/sk8l/templates/deployment.yaml
diff_status=$?
set -e
if [ $diff_status -eq 0 ]; then
  echo "No differences found between the files."
  exit 1
fi
set +e
diff -C3 charts/sk8l/values.yaml.bak charts/sk8l/values.yaml
diff_status=$?
set -e
if [ $diff_status -eq 0 ]; then
  echo "No differences found between the files."
  exit 1
fi
