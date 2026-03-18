// Package mapper provides functions for converting Kubernetes API types to protobuf response types.
package mapper

import (
	"time"

	"github.com/danroux/sk8l/protos"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TimeToString(t *metav1.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func MapOwnerReferences(refs []metav1.OwnerReference) []*protos.OwnerReferenceResponse {
	result := make([]*protos.OwnerReferenceResponse, 0, len(refs))
	for _, r := range refs {
		ctrl := r.Controller != nil && *r.Controller
		result = append(result, &protos.OwnerReferenceResponse{
			ApiVersion: r.APIVersion,
			Kind:       r.Kind,
			Name:       r.Name,
			Uid:        string(r.UID),
			Controller: ctrl,
		})
	}
	return result
}

func MapObjectMeta(m metav1.ObjectMeta) *protos.ObjectMetaResponse {
	return &protos.ObjectMetaResponse{
		Name:              m.Name,
		Namespace:         m.Namespace,
		Uid:               string(m.UID),
		Labels:            m.Labels,
		Annotations:       m.Annotations,
		CreationTimestamp: m.CreationTimestamp.UTC().Format(time.RFC3339),
		Generation:        m.Generation,
		OwnerReferences:   MapOwnerReferences(m.OwnerReferences),
	}
}

func MapContainerStateTerminated(t *corev1.ContainerStateTerminated) *protos.ContainerStateTerminatedResponse {
	if t == nil {
		return nil
	}
	return &protos.ContainerStateTerminatedResponse{
		ExitCode:    t.ExitCode,
		Signal:      t.Signal,
		Reason:      t.Reason,
		Message:     t.Message,
		StartedAt:   TimeToString(&t.StartedAt),
		FinishedAt:  TimeToString(&t.FinishedAt),
		ContainerID: t.ContainerID,
	}
}

func MapContainerState(s corev1.ContainerState) *protos.ContainerStateResponse {
	r := &protos.ContainerStateResponse{}
	if s.Waiting != nil {
		r.Waiting = &protos.ContainerStateWaitingResponse{
			Reason:  s.Waiting.Reason,
			Message: s.Waiting.Message,
		}
	}
	if s.Running != nil {
		r.Running = &protos.ContainerStateRunningResponse{
			StartedAt: TimeToString(&s.Running.StartedAt),
		}
	}
	if s.Terminated != nil {
		r.Terminated = MapContainerStateTerminated(s.Terminated)
	}
	return r
}

func MapContainerStatus(cs corev1.ContainerStatus) *protos.ContainerStatusResponse {
	started := cs.Started != nil && *cs.Started
	return &protos.ContainerStatusResponse{
		Name:         cs.Name,
		State:        MapContainerState(cs.State),
		LastState:    MapContainerState(cs.LastTerminationState),
		Ready:        cs.Ready,
		RestartCount: cs.RestartCount,
		Image:        cs.Image,
		ImageID:      cs.ImageID,
		ContainerID:  cs.ContainerID,
		Started:      started,
	}
}

func MapContainerStatuses(css []corev1.ContainerStatus) []*protos.ContainerStatusResponse {
	result := make([]*protos.ContainerStatusResponse, 0, len(css))
	for _, cs := range css {
		result = append(result, MapContainerStatus(cs))
	}
	return result
}

func MapPodConditions(conditions []corev1.PodCondition) []*protos.PodConditionResponse {
	result := make([]*protos.PodConditionResponse, 0, len(conditions))
	for _, c := range conditions {
		result = append(result, &protos.PodConditionResponse{
			Type:               string(c.Type),
			Status:             string(c.Status),
			LastProbeTime:      TimeToString(&c.LastProbeTime),
			LastTransitionTime: TimeToString(&c.LastTransitionTime),
			Reason:             c.Reason,
			Message:            c.Message,
		})
	}
	return result
}

func MapPodStatus(s corev1.PodStatus) *protos.PodStatusResponse {
	ips := make([]string, 0, len(s.PodIPs))
	for _, ip := range s.PodIPs {
		ips = append(ips, ip.IP)
	}
	return &protos.PodStatusResponse{
		Phase:                      string(s.Phase),
		Conditions:                 MapPodConditions(s.Conditions),
		Message:                    s.Message,
		Reason:                     s.Reason,
		HostIP:                     s.HostIP,
		PodIP:                      s.PodIP,
		StartTime:                  TimeToString(s.StartTime),
		ContainerStatuses:          MapContainerStatuses(s.ContainerStatuses),
		InitContainerStatuses:      MapContainerStatuses(s.InitContainerStatuses),
		EphemeralContainerStatuses: MapContainerStatuses(s.EphemeralContainerStatuses),
		QosClass:                   string(s.QOSClass),
		PodIPs:                     ips,
	}
}

func MapEnvVars(envs []corev1.EnvVar) []*protos.EnvVarResponse {
	result := make([]*protos.EnvVarResponse, 0, len(envs))
	for _, e := range envs {
		result = append(result, &protos.EnvVarResponse{
			Name:  e.Name,
			Value: e.Value,
		})
	}
	return result
}

func MapVolumeMounts(mounts []corev1.VolumeMount) []*protos.VolumeMountResponse {
	result := make([]*protos.VolumeMountResponse, 0, len(mounts))
	for _, m := range mounts {
		result = append(result, &protos.VolumeMountResponse{
			Name:      m.Name,
			ReadOnly:  m.ReadOnly,
			MountPath: m.MountPath,
		})
	}
	return result
}

func MapContainerPorts(ports []corev1.ContainerPort) []*protos.ContainerPortResponse {
	result := make([]*protos.ContainerPortResponse, 0, len(ports))
	for _, p := range ports {
		result = append(result, &protos.ContainerPortResponse{
			Name:          p.Name,
			ContainerPort: p.ContainerPort,
			Protocol:      string(p.Protocol),
		})
	}
	return result
}

func MapResources(r corev1.ResourceRequirements) *protos.ResourcesResponse {
	limits := make(map[string]string)
	requests := make(map[string]string)
	for k, v := range r.Limits {
		limits[string(k)] = v.String()
	}
	for k, v := range r.Requests {
		requests[string(k)] = v.String()
	}
	return &protos.ResourcesResponse{
		Limits:   limits,
		Requests: requests,
	}
}

func MapContainer(c corev1.Container) *protos.ContainerSpecResponse {
	return &protos.ContainerSpecResponse{
		Name:            c.Name,
		Image:           c.Image,
		Command:         c.Command,
		Args:            c.Args,
		Ports:           MapContainerPorts(c.Ports),
		Env:             MapEnvVars(c.Env),
		Resources:       MapResources(c.Resources),
		VolumeMounts:    MapVolumeMounts(c.VolumeMounts),
		ImagePullPolicy: string(c.ImagePullPolicy),
		WorkingDir:      c.WorkingDir,
	}
}

func MapEphemeralContainer(c corev1.EphemeralContainer) *protos.ContainerSpecResponse {
	return &protos.ContainerSpecResponse{
		Name:            c.Name,
		Image:           c.Image,
		Command:         c.Command,
		Args:            c.Args,
		ImagePullPolicy: string(c.ImagePullPolicy),
		WorkingDir:      c.WorkingDir,
	}
}

func MapContainers(containers []corev1.Container) []*protos.ContainerSpecResponse {
	result := make([]*protos.ContainerSpecResponse, 0, len(containers))
	for _, c := range containers {
		result = append(result, MapContainer(c))
	}
	return result
}

func MapEphemeralContainers(containers []corev1.EphemeralContainer) []*protos.ContainerSpecResponse {
	result := make([]*protos.ContainerSpecResponse, 0, len(containers))
	for _, c := range containers {
		result = append(result, MapEphemeralContainer(c))
	}
	return result
}

func MapPodSpec(s corev1.PodSpec) *protos.PodSpecResponse {
	tgps := int64(0)
	if s.TerminationGracePeriodSeconds != nil {
		tgps = *s.TerminationGracePeriodSeconds
	}
	return &protos.PodSpecResponse{
		Containers:                    MapContainers(s.Containers),
		InitContainers:                MapContainers(s.InitContainers),
		EphemeralContainers:           MapEphemeralContainers(s.EphemeralContainers),
		RestartPolicy:                 string(s.RestartPolicy),
		ServiceAccountName:            s.ServiceAccountName,
		NodeName:                      s.NodeName,
		NodeSelector:                  s.NodeSelector,
		TerminationGracePeriodSeconds: tgps,
	}
}

func MapJobConditions(conditions []batchv1.JobCondition) []*protos.JobConditionResponse {
	result := make([]*protos.JobConditionResponse, 0, len(conditions))
	for _, c := range conditions {
		result = append(result, &protos.JobConditionResponse{
			Type:               string(c.Type),
			Status:             string(c.Status),
			LastProbeTime:      TimeToString(&c.LastProbeTime),
			LastTransitionTime: TimeToString(&c.LastTransitionTime),
			Reason:             c.Reason,
			Message:            c.Message,
		})
	}
	return result
}

func MapJobStatus(s batchv1.JobStatus) *protos.JobStatusResponse {
	ready := int32(0)
	if s.Ready != nil {
		ready = *s.Ready
	}
	return &protos.JobStatusResponse{
		Active:         s.Active,
		Succeeded:      s.Succeeded,
		Failed:         s.Failed,
		StartTime:      TimeToString(s.StartTime),
		CompletionTime: TimeToString(s.CompletionTime),
		Conditions:     MapJobConditions(s.Conditions),
		Ready:          ready,
	}
}

func MapJobSpec(s batchv1.JobSpec) *protos.JobSpecResponse {
	parallelism := int32(0)
	if s.Parallelism != nil {
		parallelism = *s.Parallelism
	}
	completions := int32(0)
	if s.Completions != nil {
		completions = *s.Completions
	}
	activeDeadlineSeconds := int64(0)
	if s.ActiveDeadlineSeconds != nil {
		activeDeadlineSeconds = *s.ActiveDeadlineSeconds
	}
	backoffLimit := int32(0)
	if s.BackoffLimit != nil {
		backoffLimit = *s.BackoffLimit
	}
	suspend := false
	if s.Suspend != nil {
		suspend = *s.Suspend
	}
	completionMode := ""
	if s.CompletionMode != nil {
		completionMode = string(*s.CompletionMode)
	}
	return &protos.JobSpecResponse{
		Parallelism:           parallelism,
		Completions:           completions,
		ActiveDeadlineSeconds: activeDeadlineSeconds,
		BackoffLimit:          backoffLimit,
		Suspend:               suspend,
		CompletionMode:        completionMode,
	}
}

func MapJobCondition(c *batchv1.JobCondition) *protos.JobConditionResponse {
	if c == nil {
		return nil
	}
	return &protos.JobConditionResponse{
		Type:               string(c.Type),
		Status:             string(c.Status),
		LastProbeTime:      TimeToString(&c.LastProbeTime),
		LastTransitionTime: TimeToString(&c.LastTransitionTime),
		Reason:             c.Reason,
		Message:            c.Message,
	}
}

func MapCronJobSpec(s batchv1.CronJobSpec) *protos.CronJobSpecResponse {
	suspend := false
	if s.Suspend != nil {
		suspend = *s.Suspend
	}
	successfulJobsHistoryLimit := int32(0)
	if s.SuccessfulJobsHistoryLimit != nil {
		successfulJobsHistoryLimit = *s.SuccessfulJobsHistoryLimit
	}
	failedJobsHistoryLimit := int32(0)
	if s.FailedJobsHistoryLimit != nil {
		failedJobsHistoryLimit = *s.FailedJobsHistoryLimit
	}
	timezone := ""
	if s.TimeZone != nil {
		timezone = *s.TimeZone
	}
	startingDeadlineSeconds := int64(0)
	if s.StartingDeadlineSeconds != nil {
		startingDeadlineSeconds = *s.StartingDeadlineSeconds
	}
	return &protos.CronJobSpecResponse{
		Schedule:                   s.Schedule,
		Timezone:                   timezone,
		ConcurrencyPolicy:          string(s.ConcurrencyPolicy),
		Suspend:                    suspend,
		SuccessfulJobsHistoryLimit: successfulJobsHistoryLimit,
		FailedJobsHistoryLimit:     failedJobsHistoryLimit,
		StartingDeadlineSeconds:    startingDeadlineSeconds,
	}
}

func MapUncountedTerminatedPods(u *batchv1.UncountedTerminatedPods) *protos.UncountedTerminatedPods {
	if u == nil {
		return nil
	}
	succeeded := make([]string, 0, len(u.Succeeded))
	for _, uid := range u.Succeeded {
		succeeded = append(succeeded, string(uid))
	}
	failed := make([]string, 0, len(u.Failed))
	for _, uid := range u.Failed {
		failed = append(failed, string(uid))
	}
	return &protos.UncountedTerminatedPods{
		Succeeded: succeeded,
		Failed:    failed,
	}
}

func MapCustomJobConditions(conditions []batchv1.JobCondition) []*protos.JobCondition {
	result := make([]*protos.JobCondition, 0, len(conditions))
	for _, c := range conditions {
		result = append(result, &protos.JobCondition{
			Type:               string(c.Type),
			Status:             string(c.Status),
			LastProbeTime:      TimeToString(&c.LastProbeTime),
			LastTransitionTime: TimeToString(&c.LastTransitionTime),
			Reason:             c.Reason,
			Message:            c.Message,
		})
	}
	return result
}

func MapCustomJobStatus(s batchv1.JobStatus) *protos.JobStatus {
	terminating := int32(0)
	if s.Terminating != nil {
		terminating = *s.Terminating
	}
	ready := int32(0)
	if s.Ready != nil {
		ready = *s.Ready
	}
	return &protos.JobStatus{
		Conditions:              MapCustomJobConditions(s.Conditions),
		StartTime:               TimeToString(s.StartTime),
		CompletionTime:          TimeToString(s.CompletionTime),
		Active:                  s.Active,
		Succeeded:               s.Succeeded,
		Failed:                  s.Failed,
		Terminating:             terminating,
		CompletedIndexes:        s.CompletedIndexes,
		UncountedTerminatedPods: MapUncountedTerminatedPods(s.UncountedTerminatedPods),
		Ready:                   ready,
	}
}
