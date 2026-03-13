#!/usr/bin/env bash
# scripts/check_k8s_mapper_drift.sh

set -euo pipefail

REPORT_REMOVED=""
REPORT_ADDED=""
DRIFT=0

k8s_version=$(go list -m -json k8s.io/api | jq -r '.Version')
apimachinery_version=$(go list -m -json k8s.io/apimachinery | jq -r '.Version')

echo "Checking k8s.io/api@${k8s_version} and k8s.io/apimachinery@${apimachinery_version}"
echo ""

get_upstream_fields() {
    local type_path=$1
    go doc "${type_path}" 2>/dev/null \
        | grep -E '^\s+[A-Z][A-Za-z0-9]+\s' \
        | awk '{print $1}' \
        | sort
}

check_type() {
    local type_path=$1
    local mapper=$2
    shift 2
    local mapped_fields=("$@")

    if ! go doc "${type_path}" &>/dev/null; then
        DRIFT=1
        REPORT_REMOVED+="❌ REMOVED TYPE: ${type_path} — update ${mapper}()\n"
        return
    fi

    upstream_fields=$(get_upstream_fields "${type_path}")

    for field in "${mapped_fields[@]}"; do
        if ! echo "${upstream_fields}" | grep -qw "${field}"; then
            DRIFT=1
            REPORT_REMOVED+="❌ REMOVED FIELD: ${type_path}.${field} — update ${mapper}()\n"
        fi
    done

    while IFS= read -r upstream_field; do
        [ -z "${upstream_field}" ] && continue
        found=0
        for mapped in "${mapped_fields[@]}"; do
            if [ "${mapped}" = "${upstream_field}" ]; then
                found=1
                break
            fi
        done
        if [ $found -eq 0 ]; then
            REPORT_ADDED+="➕ NEW FIELD: ${type_path}.${upstream_field} — consider adding to ${mapper}()\n"
        fi
    done <<< "${upstream_fields}"
}

echo "=== Checking metav1 types ==="
check_type "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta" "mapObjectMeta" \
    Name Namespace UID Labels Annotations Generation OwnerReferences CreationTimestamp

check_type "k8s.io/apimachinery/pkg/apis/meta/v1.OwnerReference" "mapOwnerReferences" \
    APIVersion Kind Name UID Controller

echo "=== Checking corev1 types ==="
check_type "k8s.io/api/core/v1.PodStatus" "mapPodStatus" \
    Phase Conditions Message Reason HostIP PodIP StartTime \
    ContainerStatuses InitContainerStatuses EphemeralContainerStatuses QOSClass PodIPs

check_type "k8s.io/api/core/v1.PodCondition" "mapPodConditions" \
    Type Status LastProbeTime LastTransitionTime Reason Message

check_type "k8s.io/api/core/v1.ContainerStatus" "mapContainerStatus" \
    Name State LastTerminationState Ready RestartCount Image ImageID ContainerID Started

check_type "k8s.io/api/core/v1.ContainerStateTerminated" "mapContainerStateTerminated" \
    ExitCode Signal Reason Message StartedAt FinishedAt ContainerID

check_type "k8s.io/api/core/v1.ContainerStateWaiting" "mapContainerState" \
    Reason Message

check_type "k8s.io/api/core/v1.ContainerStateRunning" "mapContainerState" \
    StartedAt

check_type "k8s.io/api/core/v1.PodSpec" "mapPodSpec" \
    Containers InitContainers EphemeralContainers RestartPolicy \
    ServiceAccountName NodeName NodeSelector TerminationGracePeriodSeconds

check_type "k8s.io/api/core/v1.Container" "mapContainer" \
    Name Image Command Args Ports Env Resources VolumeMounts ImagePullPolicy WorkingDir

check_type "k8s.io/api/core/v1.EphemeralContainerCommon" "mapEphemeralContainer" \
    Name Image Command Args ImagePullPolicy WorkingDir

check_type "k8s.io/api/core/v1.ContainerPort" "mapContainerPorts" \
    Name ContainerPort Protocol

check_type "k8s.io/api/core/v1.EnvVar" "mapEnvVars" \
    Name Value

check_type "k8s.io/api/core/v1.VolumeMount" "mapVolumeMounts" \
    Name ReadOnly MountPath

check_type "k8s.io/api/core/v1.ResourceRequirements" "mapResources" \
    Limits Requests

echo "=== Checking batchv1 types ==="
check_type "k8s.io/api/batch/v1.JobStatus" "mapJobStatus" \
    Active Succeeded Failed Ready StartTime CompletionTime Conditions

check_type "k8s.io/api/batch/v1.JobCondition" "mapJobCondition" \
    Type Status LastProbeTime LastTransitionTime Reason Message

check_type "k8s.io/api/batch/v1.JobSpec" "mapJobSpec" \
    Parallelism Completions ActiveDeadlineSeconds BackoffLimit CompletionMode Suspend

check_type "k8s.io/api/batch/v1.CronJobSpec" "mapCronJobSpec" \
    Schedule TimeZone ConcurrencyPolicy Suspend \
    SuccessfulJobsHistoryLimit FailedJobsHistoryLimit StartingDeadlineSeconds

echo ""

if [ -n "$REPORT_REMOVED" ]; then
    echo "❌ REMOVED FIELDS — update mappers.go immediately:"
    echo ""
    echo -e "$REPORT_REMOVED"
fi

if [ -n "$REPORT_ADDED" ]; then
    echo "➕ NEW UPSTREAM FIELDS — consider exposing in your proto/mappers:"
    echo ""
    echo -e "$REPORT_ADDED"
fi

if [ $DRIFT -eq 1 ]; then
    echo "⚠️  Mapper drift detected. See above."
    exit 1
else
    echo "✅ All mapped fields present in k8s@${k8s_version} — no removals detected."
    if [ -n "$REPORT_ADDED" ]; then
        echo "   (new upstream fields found — review additions above)"
    fi
    exit 0
fi
