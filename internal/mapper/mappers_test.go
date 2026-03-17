package mapper

import (
	"testing"
	"time"

	"github.com/danroux/sk8l/protos"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestTimeToString(t *testing.T) {
	tests := []struct {
		name     string
		input    *metav1.Time
		expected string
	}{
		{
			name:     "nil time returns empty string",
			input:    nil,
			expected: "",
		},
		{
			name:     "zero time returns empty string",
			input:    &metav1.Time{},
			expected: "",
		},
		{
			name:     "valid time returns RFC3339",
			input:    &metav1.Time{Time: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)},
			expected: "2024-01-15T10:30:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TimeToString(tt.input)
			if got != tt.expected {
				t.Errorf("TimeToString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestMapOwnerReferences(t *testing.T) {
	ctrl := true
	refs := []metav1.OwnerReference{
		{
			APIVersion: "batch/v1",
			Kind:       "CronJob",
			Name:       "my-cronjob",
			UID:        types.UID("uid-123"),
			Controller: &ctrl,
		},
	}

	result := MapOwnerReferences(refs)

	if len(result) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(result))
	}
	if result[0].ApiVersion != "batch/v1" {
		t.Errorf("expected ApiVersion %q, got %q", "batch/v1", result[0].ApiVersion)
	}
	if result[0].Kind != "CronJob" {
		t.Errorf("expected Kind %q, got %q", "CronJob", result[0].Kind)
	}
	if result[0].Name != "my-cronjob" {
		t.Errorf("expected Name %q, got %q", "my-cronjob", result[0].Name)
	}
	if result[0].Uid != "uid-123" {
		t.Errorf("expected Uid %q, got %q", "uid-123", result[0].Uid)
	}
	if !result[0].Controller {
		t.Error("expected Controller to be true")
	}
}

func TestMapOwnerReferences_NilController(t *testing.T) {
	refs := []metav1.OwnerReference{
		{
			APIVersion: "batch/v1",
			Kind:       "CronJob",
			Name:       "my-cronjob",
			UID:        types.UID("uid-123"),
			Controller: nil,
		},
	}

	result := MapOwnerReferences(refs)

	if result[0].Controller {
		t.Error("expected Controller to be false when nil")
	}
}

func TestMapObjectMeta(t *testing.T) {
	ts := metav1.NewTime(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))
	ctrl := true
	meta := metav1.ObjectMeta{
		Name:              "my-job",
		Namespace:         "default",
		UID:               types.UID("uid-abc"),
		Labels:            map[string]string{"app": "sk8l"},
		Annotations:       map[string]string{"note": "test"},
		CreationTimestamp: ts,
		Generation:        3,
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: "batch/v1",
				Kind:       "CronJob",
				Name:       "my-cronjob",
				UID:        types.UID("uid-123"),
				Controller: &ctrl,
			},
		},
	}

	result := MapObjectMeta(meta)

	if result.Name != "my-job" {
		t.Errorf("expected Name %q, got %q", "my-job", result.Name)
	}
	if result.Namespace != "default" {
		t.Errorf("expected Namespace %q, got %q", "default", result.Namespace)
	}
	if result.Uid != "uid-abc" {
		t.Errorf("expected Uid %q, got %q", "uid-abc", result.Uid)
	}
	if result.Labels["app"] != "sk8l" {
		t.Errorf("expected Label app=sk8l, got %q", result.Labels["app"])
	}
	if result.Annotations["note"] != "test" {
		t.Errorf("expected Annotation note=test, got %q", result.Annotations["note"])
	}
	if result.CreationTimestamp != "2024-01-15T10:30:00Z" {
		t.Errorf("expected CreationTimestamp %q, got %q", "2024-01-15T10:30:00Z", result.CreationTimestamp)
	}
	if result.Generation != 3 {
		t.Errorf("expected Generation 3, got %d", result.Generation)
	}
	if len(result.OwnerReferences) != 1 {
		t.Fatalf("expected 1 OwnerReference, got %d", len(result.OwnerReferences))
	}
}

func TestMapContainerStateTerminated(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		result := MapContainerStateTerminated(nil)
		if result != nil {
			t.Error("expected nil result for nil input")
		}
	})

	t.Run("maps all fields", func(t *testing.T) {
		startedAt := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
		finishedAt := metav1.NewTime(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))
		state := &corev1.ContainerStateTerminated{
			ExitCode:    1,
			Signal:      9,
			Reason:      "Error",
			Message:     "OOM killed",
			StartedAt:   startedAt,
			FinishedAt:  finishedAt,
			ContainerID: "docker://abc123",
		}

		result := MapContainerStateTerminated(state)

		if result.ExitCode != 1 {
			t.Errorf("expected ExitCode 1, got %d", result.ExitCode)
		}
		if result.Signal != 9 {
			t.Errorf("expected Signal 9, got %d", result.Signal)
		}
		if result.Reason != "Error" {
			t.Errorf("expected Reason %q, got %q", "Error", result.Reason)
		}
		if result.Message != "OOM killed" {
			t.Errorf("expected Message %q, got %q", "OOM killed", result.Message)
		}
		if result.StartedAt != "2024-01-15T10:00:00Z" {
			t.Errorf("expected StartedAt %q, got %q", "2024-01-15T10:00:00Z", result.StartedAt)
		}
		if result.FinishedAt != "2024-01-15T10:30:00Z" {
			t.Errorf("expected FinishedAt %q, got %q", "2024-01-15T10:30:00Z", result.FinishedAt)
		}
		if result.ContainerID != "docker://abc123" {
			t.Errorf("expected ContainerID %q, got %q", "docker://abc123", result.ContainerID)
		}
	})
}

func TestMapContainerState(t *testing.T) {
	t.Run("waiting state", func(t *testing.T) {
		state := corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  "CrashLoopBackOff",
				Message: "back-off restarting failed container",
			},
		}
		result := MapContainerState(state)
		if result.Waiting == nil {
			t.Fatal("expected Waiting to be set")
		}
		if result.Waiting.Reason != "CrashLoopBackOff" {
			t.Errorf("expected Reason %q, got %q", "CrashLoopBackOff", result.Waiting.Reason)
		}
		if result.Running != nil {
			t.Error("expected Running to be nil")
		}
		if result.Terminated != nil {
			t.Error("expected Terminated to be nil")
		}
	})

	t.Run("running state", func(t *testing.T) {
		startedAt := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
		state := corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{
				StartedAt: startedAt,
			},
		}
		result := MapContainerState(state)
		if result.Running == nil {
			t.Fatal("expected Running to be set")
		}
		if result.Running.StartedAt != "2024-01-15T10:00:00Z" {
			t.Errorf("expected StartedAt %q, got %q", "2024-01-15T10:00:00Z", result.Running.StartedAt)
		}
	})

	t.Run("terminated state", func(t *testing.T) {
		state := corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 0,
				Reason:   "Completed",
			},
		}
		result := MapContainerState(state)
		if result.Terminated == nil {
			t.Fatal("expected Terminated to be set")
		}
		if result.Terminated.Reason != "Completed" {
			t.Errorf("expected Reason %q, got %q", "Completed", result.Terminated.Reason)
		}
	})
}

func TestMapContainerStatus(t *testing.T) {
	started := true
	cs := corev1.ContainerStatus{
		Name:         "my-container",
		Ready:        true,
		RestartCount: 3,
		Image:        "nginx:latest",
		ImageID:      "sha256:abc",
		ContainerID:  "docker://xyz",
		Started:      &started,
		State: corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{},
		},
	}

	result := MapContainerStatus(cs)

	if result.Name != "my-container" {
		t.Errorf("expected Name %q, got %q", "my-container", result.Name)
	}
	if !result.Ready {
		t.Error("expected Ready to be true")
	}
	if result.RestartCount != 3 {
		t.Errorf("expected RestartCount 3, got %d", result.RestartCount)
	}
	if result.Image != "nginx:latest" {
		t.Errorf("expected Image %q, got %q", "nginx:latest", result.Image)
	}
	if !result.Started {
		t.Error("expected Started to be true")
	}
}

func TestMapContainerStatus_NilStarted(t *testing.T) {
	cs := corev1.ContainerStatus{
		Name:    "my-container",
		Started: nil,
	}
	result := MapContainerStatus(cs)
	if result.Started {
		t.Error("expected Started to be false when nil")
	}
}

func TestMapPodConditions(t *testing.T) {
	lastProbe := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	lastTransition := metav1.NewTime(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))
	conditions := []corev1.PodCondition{
		{
			Type:               corev1.PodReady,
			Status:             corev1.ConditionTrue,
			LastProbeTime:      lastProbe,
			LastTransitionTime: lastTransition,
			Reason:             "PodCompleted",
			Message:            "pod has completed",
		},
	}

	result := MapPodConditions(conditions)

	if len(result) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(result))
	}
	if result[0].Type != "Ready" {
		t.Errorf("expected Type %q, got %q", "Ready", result[0].Type)
	}
	if result[0].Status != "True" {
		t.Errorf("expected Status %q, got %q", "True", result[0].Status)
	}
	if result[0].LastProbeTime != "2024-01-15T10:00:00Z" {
		t.Errorf("expected LastProbeTime %q, got %q", "2024-01-15T10:00:00Z", result[0].LastProbeTime)
	}
	if result[0].LastTransitionTime != "2024-01-15T10:30:00Z" {
		t.Errorf("expected LastTransitionTime %q, got %q", "2024-01-15T10:30:00Z", result[0].LastTransitionTime)
	}
	if result[0].Reason != "PodCompleted" {
		t.Errorf("expected Reason %q, got %q", "PodCompleted", result[0].Reason)
	}
}

func TestMapPodStatus(t *testing.T) {
	startTime := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	status := corev1.PodStatus{
		Phase:   corev1.PodRunning,
		Message: "running fine",
		Reason:  "scheduled",
		HostIP:  "192.168.1.1",
		PodIP:   "10.0.0.1",
		PodIPs: []corev1.PodIP{
			{IP: "10.0.0.1"},
			{IP: "fd00::1"},
		},
		StartTime: &startTime,
		QOSClass:  corev1.PodQOSBurstable,
	}

	result := MapPodStatus(status)

	if result.Phase != "Running" {
		t.Errorf("expected Phase %q, got %q", "Running", result.Phase)
	}
	if result.HostIP != "192.168.1.1" {
		t.Errorf("expected HostIP %q, got %q", "192.168.1.1", result.HostIP)
	}
	if result.PodIP != "10.0.0.1" {
		t.Errorf("expected PodIP %q, got %q", "10.0.0.1", result.PodIP)
	}
	if result.StartTime != "2024-01-15T10:00:00Z" {
		t.Errorf("expected StartTime %q, got %q", "2024-01-15T10:00:00Z", result.StartTime)
	}
	if result.QosClass != "Burstable" {
		t.Errorf("expected QosClass %q, got %q", "Burstable", result.QosClass)
	}
	if len(result.PodIPs) != 2 {
		t.Fatalf("expected 2 PodIPs, got %d", len(result.PodIPs))
	}
	if result.PodIPs[0] != "10.0.0.1" {
		t.Errorf("expected PodIPs[0] %q, got %q", "10.0.0.1", result.PodIPs[0])
	}
	if result.PodIPs[1] != "fd00::1" {
		t.Errorf("expected PodIPs[1] %q, got %q", "fd00::1", result.PodIPs[1])
	}
}

func TestMapResources(t *testing.T) {
	r := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		},
	}

	result := MapResources(r)

	if result.Limits["cpu"] == "" {
		t.Error("expected cpu limit to be set")
	}
	if result.Limits["memory"] == "" {
		t.Error("expected memory limit to be set")
	}
	if result.Requests["cpu"] == "" {
		t.Error("expected cpu request to be set")
	}
	if result.Requests["memory"] == "" {
		t.Error("expected memory request to be set")
	}
}

func TestMapContainer(t *testing.T) {
	c := corev1.Container{
		Name:            "my-container",
		Image:           "nginx:latest",
		Command:         []string{"/bin/sh"},
		Args:            []string{"-c", "echo hello"},
		ImagePullPolicy: corev1.PullAlways,
		WorkingDir:      "/app",
		Ports: []corev1.ContainerPort{
			{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
		},
		Env: []corev1.EnvVar{
			{Name: "ENV", Value: "production"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "data", ReadOnly: true, MountPath: "/data"},
		},
	}

	result := MapContainer(c)

	if result.Name != "my-container" {
		t.Errorf("expected Name %q, got %q", "my-container", result.Name)
	}
	if result.Image != "nginx:latest" {
		t.Errorf("expected Image %q, got %q", "nginx:latest", result.Image)
	}
	if len(result.Command) != 1 || result.Command[0] != "/bin/sh" {
		t.Errorf("expected Command [/bin/sh], got %v", result.Command)
	}
	if result.ImagePullPolicy != "Always" {
		t.Errorf("expected ImagePullPolicy %q, got %q", "Always", result.ImagePullPolicy)
	}
	if len(result.Ports) != 1 || result.Ports[0].ContainerPort != 8080 {
		t.Errorf("expected port 8080, got %v", result.Ports)
	}
	if len(result.Env) != 1 || result.Env[0].Name != "ENV" {
		t.Errorf("expected env ENV, got %v", result.Env)
	}
	if len(result.VolumeMounts) != 1 || result.VolumeMounts[0].MountPath != "/data" {
		t.Errorf("expected mount /data, got %v", result.VolumeMounts)
	}
}

func TestMapPodSpec(t *testing.T) {
	tgps := int64(30)
	spec := corev1.PodSpec{
		RestartPolicy:                 corev1.RestartPolicyNever,
		ServiceAccountName:            "my-sa",
		NodeName:                      "node-1",
		NodeSelector:                  map[string]string{"zone": "us-east-1"},
		TerminationGracePeriodSeconds: &tgps,
		Containers: []corev1.Container{
			{Name: "main", Image: "nginx:latest"},
		},
		InitContainers: []corev1.Container{
			{Name: "init", Image: "busybox"},
		},
	}

	result := MapPodSpec(spec)

	if result.RestartPolicy != "Never" {
		t.Errorf("expected RestartPolicy %q, got %q", "Never", result.RestartPolicy)
	}
	if result.ServiceAccountName != "my-sa" {
		t.Errorf("expected ServiceAccountName %q, got %q", "my-sa", result.ServiceAccountName)
	}
	if result.NodeName != "node-1" {
		t.Errorf("expected NodeName %q, got %q", "node-1", result.NodeName)
	}
	if result.TerminationGracePeriodSeconds != 30 {
		t.Errorf("expected TerminationGracePeriodSeconds 30, got %d", result.TerminationGracePeriodSeconds)
	}
	if len(result.Containers) != 1 {
		t.Errorf("expected 1 container, got %d", len(result.Containers))
	}
	if len(result.InitContainers) != 1 {
		t.Errorf("expected 1 init container, got %d", len(result.InitContainers))
	}
}

func TestMapPodSpec_NilTerminationGracePeriod(t *testing.T) {
	spec := corev1.PodSpec{
		TerminationGracePeriodSeconds: nil,
	}
	result := MapPodSpec(spec)
	if result.TerminationGracePeriodSeconds != 0 {
		t.Errorf("expected 0 for nil TerminationGracePeriodSeconds, got %d", result.TerminationGracePeriodSeconds)
	}
}

func TestMapJobCondition(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		result := MapJobCondition(nil)
		if result != nil {
			t.Error("expected nil result for nil input")
		}
	})

	t.Run("maps all fields", func(t *testing.T) {
		lastProbe := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
		lastTransition := metav1.NewTime(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))
		c := &batchv1.JobCondition{
			Type:               batchv1.JobFailed,
			Status:             "False",
			LastProbeTime:      lastProbe,
			LastTransitionTime: lastTransition,
			Reason:             "BackoffLimitExceeded",
			Message:            "Job has reached the specified backoff limit",
		}

		result := MapJobCondition(c)

		if result.Type != "Failed" {
			t.Errorf("expected Type %q, got %q", "Failed", result.Type)
		}
		if result.Status != "False" {
			t.Errorf("expected Status %q, got %q", "False", result.Status)
		}
		if result.Reason != "BackoffLimitExceeded" {
			t.Errorf("expected Reason %q, got %q", "BackoffLimitExceeded", result.Reason)
		}
		if result.LastProbeTime != "2024-01-15T10:00:00Z" {
			t.Errorf("expected LastProbeTime %q, got %q", "2024-01-15T10:00:00Z", result.LastProbeTime)
		}
		if result.LastTransitionTime != "2024-01-15T10:30:00Z" {
			t.Errorf("expected LastTransitionTime %q, got %q", "2024-01-15T10:30:00Z", result.LastTransitionTime)
		}
	})
}

func TestMapJobStatus(t *testing.T) {
	startTime := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	completionTime := metav1.NewTime(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))
	ready := int32(2)
	s := batchv1.JobStatus{
		Active:         1,
		Succeeded:      3,
		Failed:         0,
		StartTime:      &startTime,
		CompletionTime: &completionTime,
		Ready:          &ready,
	}

	result := MapJobStatus(s)

	if result.Active != 1 {
		t.Errorf("expected Active 1, got %d", result.Active)
	}
	if result.Succeeded != 3 {
		t.Errorf("expected Succeeded 3, got %d", result.Succeeded)
	}
	if result.Failed != 0 {
		t.Errorf("expected Failed 0, got %d", result.Failed)
	}
	if result.StartTime != "2024-01-15T10:00:00Z" {
		t.Errorf("expected StartTime %q, got %q", "2024-01-15T10:00:00Z", result.StartTime)
	}
	if result.CompletionTime != "2024-01-15T10:30:00Z" {
		t.Errorf("expected CompletionTime %q, got %q", "2024-01-15T10:30:00Z", result.CompletionTime)
	}
	if result.Ready != 2 {
		t.Errorf("expected Ready 2, got %d", result.Ready)
	}
}

func TestMapJobSpec(t *testing.T) {
	parallelism := int32(2)
	completions := int32(5)
	activeDeadlineSeconds := int64(300)
	backoffLimit := int32(3)
	suspend := true
	completionMode := batchv1.CompletionMode(batchv1.IndexedCompletion)

	s := batchv1.JobSpec{
		Parallelism:           &parallelism,
		Completions:           &completions,
		ActiveDeadlineSeconds: &activeDeadlineSeconds,
		BackoffLimit:          &backoffLimit,
		Suspend:               &suspend,
		CompletionMode:        &completionMode,
	}

	result := MapJobSpec(s)

	if result.Parallelism != 2 {
		t.Errorf("expected Parallelism 2, got %d", result.Parallelism)
	}
	if result.Completions != 5 {
		t.Errorf("expected Completions 5, got %d", result.Completions)
	}
	if result.ActiveDeadlineSeconds != 300 {
		t.Errorf("expected ActiveDeadlineSeconds 300, got %d", result.ActiveDeadlineSeconds)
	}
	if result.BackoffLimit != 3 {
		t.Errorf("expected BackoffLimit 3, got %d", result.BackoffLimit)
	}
	if !result.Suspend {
		t.Error("expected Suspend to be true")
	}
	if result.CompletionMode != "Indexed" {
		t.Errorf("expected CompletionMode %q, got %q", "Indexed", result.CompletionMode)
	}
}

func TestMapJobSpec_NilPointers(t *testing.T) {
	s := batchv1.JobSpec{}
	result := MapJobSpec(s)

	if result.Parallelism != 0 {
		t.Errorf("expected Parallelism 0, got %d", result.Parallelism)
	}
	if result.Completions != 0 {
		t.Errorf("expected Completions 0, got %d", result.Completions)
	}
	if result.ActiveDeadlineSeconds != 0 {
		t.Errorf("expected ActiveDeadlineSeconds 0, got %d", result.ActiveDeadlineSeconds)
	}
	if result.BackoffLimit != 0 {
		t.Errorf("expected BackoffLimit 0, got %d", result.BackoffLimit)
	}
	if result.Suspend {
		t.Error("expected Suspend to be false")
	}
	if result.CompletionMode != "" {
		t.Errorf("expected empty CompletionMode, got %q", result.CompletionMode)
	}
}

func TestMapCronJobSpec(t *testing.T) {
	suspend := true
	successfulJobsHistoryLimit := int32(5)
	failedJobsHistoryLimit := int32(3)
	timezone := "Europe/Berlin"
	startingDeadlineSeconds := int64(60)

	s := batchv1.CronJobSpec{
		Schedule:                   "*/5 * * * *",
		TimeZone:                   &timezone,
		ConcurrencyPolicy:          batchv1.ForbidConcurrent,
		Suspend:                    &suspend,
		SuccessfulJobsHistoryLimit: &successfulJobsHistoryLimit,
		FailedJobsHistoryLimit:     &failedJobsHistoryLimit,
		StartingDeadlineSeconds:    &startingDeadlineSeconds,
	}

	result := MapCronJobSpec(s)

	if result.Schedule != "*/5 * * * *" {
		t.Errorf("expected Schedule %q, got %q", "*/5 * * * *", result.Schedule)
	}
	if result.Timezone != "Europe/Berlin" {
		t.Errorf("expected Timezone %q, got %q", "Europe/Berlin", result.Timezone)
	}
	if result.ConcurrencyPolicy != "Forbid" {
		t.Errorf("expected ConcurrencyPolicy %q, got %q", "Forbid", result.ConcurrencyPolicy)
	}
	if !result.Suspend {
		t.Error("expected Suspend to be true")
	}
	if result.SuccessfulJobsHistoryLimit != 5 {
		t.Errorf("expected SuccessfulJobsHistoryLimit 5, got %d", result.SuccessfulJobsHistoryLimit)
	}
	if result.FailedJobsHistoryLimit != 3 {
		t.Errorf("expected FailedJobsHistoryLimit 3, got %d", result.FailedJobsHistoryLimit)
	}
	if result.StartingDeadlineSeconds != 60 {
		t.Errorf("expected StartingDeadlineSeconds 60, got %d", result.StartingDeadlineSeconds)
	}
}

func TestMapCronJobSpec_NilPointers(t *testing.T) {
	s := batchv1.CronJobSpec{
		Schedule: "0 * * * *",
	}
	result := MapCronJobSpec(s)

	if result.Schedule != "0 * * * *" {
		t.Errorf("expected Schedule %q, got %q", "0 * * * *", result.Schedule)
	}
	if result.Timezone != "" {
		t.Errorf("expected empty Timezone, got %q", result.Timezone)
	}
	if result.Suspend {
		t.Error("expected Suspend to be false")
	}
	if result.SuccessfulJobsHistoryLimit != 0 {
		t.Errorf("expected SuccessfulJobsHistoryLimit 0, got %d", result.SuccessfulJobsHistoryLimit)
	}
	if result.FailedJobsHistoryLimit != 0 {
		t.Errorf("expected FailedJobsHistoryLimit 0, got %d", result.FailedJobsHistoryLimit)
	}
	if result.StartingDeadlineSeconds != 0 {
		t.Errorf("expected StartingDeadlineSeconds 0, got %d", result.StartingDeadlineSeconds)
	}
}

func TestMapUncountedTerminatedPods(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		result := MapUncountedTerminatedPods(nil)
		if result != nil {
			t.Error("expected nil result for nil input")
		}
	})

	t.Run("maps succeeded and failed", func(t *testing.T) {
		u := &batchv1.UncountedTerminatedPods{
			Succeeded: []types.UID{"uid-1", "uid-2"},
			Failed:    []types.UID{"uid-3"},
		}

		result := MapUncountedTerminatedPods(u)

		if len(result.Succeeded) != 2 {
			t.Fatalf("expected 2 succeeded, got %d", len(result.Succeeded))
		}
		if result.Succeeded[0] != "uid-1" {
			t.Errorf("expected Succeeded[0] %q, got %q", "uid-1", result.Succeeded[0])
		}
		if len(result.Failed) != 1 {
			t.Fatalf("expected 1 failed, got %d", len(result.Failed))
		}
		if result.Failed[0] != "uid-3" {
			t.Errorf("expected Failed[0] %q, got %q", "uid-3", result.Failed[0])
		}
	})
}

func TestMapCustomJobStatus(t *testing.T) {
	startTime := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	completionTime := metav1.NewTime(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))
	terminating := int32(1)
	ready := int32(2)

	s := batchv1.JobStatus{
		Active:           3,
		Succeeded:        10,
		Failed:           1,
		StartTime:        &startTime,
		CompletionTime:   &completionTime,
		Terminating:      &terminating,
		CompletedIndexes: "0-9",
		Ready:            &ready,
		UncountedTerminatedPods: &batchv1.UncountedTerminatedPods{
			Succeeded: []types.UID{"uid-1"},
			Failed:    []types.UID{"uid-2"},
		},
	}

	result := MapCustomJobStatus(s)

	if result.Active != 3 {
		t.Errorf("expected Active 3, got %d", result.Active)
	}
	if result.Succeeded != 10 {
		t.Errorf("expected Succeeded 10, got %d", result.Succeeded)
	}
	if result.Failed != 1 {
		t.Errorf("expected Failed 1, got %d", result.Failed)
	}
	if result.StartTime != "2024-01-15T10:00:00Z" {
		t.Errorf("expected StartTime %q, got %q", "2024-01-15T10:00:00Z", result.StartTime)
	}
	if result.CompletionTime != "2024-01-15T10:30:00Z" {
		t.Errorf("expected CompletionTime %q, got %q", "2024-01-15T10:30:00Z", result.CompletionTime)
	}
	if result.Terminating != 1 {
		t.Errorf("expected Terminating 1, got %d", result.Terminating)
	}
	if result.CompletedIndexes != "0-9" {
		t.Errorf("expected CompletedIndexes %q, got %q", "0-9", result.CompletedIndexes)
	}
	if result.Ready != 2 {
		t.Errorf("expected Ready 2, got %d", result.Ready)
	}
	if result.UncountedTerminatedPods == nil {
		t.Fatal("expected UncountedTerminatedPods to be set")
	}
	if len(result.UncountedTerminatedPods.Succeeded) != 1 {
		t.Errorf("expected 1 succeeded pod, got %d", len(result.UncountedTerminatedPods.Succeeded))
	}
}

func TestMapCustomJobStatus_NilPointers(t *testing.T) {
	s := batchv1.JobStatus{}
	result := MapCustomJobStatus(s)

	if result.Terminating != 0 {
		t.Errorf("expected Terminating 0, got %d", result.Terminating)
	}
	if result.Ready != 0 {
		t.Errorf("expected Ready 0, got %d", result.Ready)
	}
	if result.UncountedTerminatedPods != nil {
		t.Error("expected UncountedTerminatedPods to be nil")
	}
}

func TestMapCustomJobConditions(t *testing.T) {
	lastProbe := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	lastTransition := metav1.NewTime(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))
	conditions := []batchv1.JobCondition{
		{
			Type:               batchv1.JobComplete,
			Status:             "True",
			LastProbeTime:      lastProbe,
			LastTransitionTime: lastTransition,
			Reason:             "Completed",
			Message:            "job completed successfully",
		},
	}

	result := MapCustomJobConditions(conditions)

	if len(result) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(result))
	}

	c := result[0]
	if c.Type != "Complete" {
		t.Errorf("expected Type %q, got %q", "Complete", c.Type)
	}
	if c.Status != "True" {
		t.Errorf("expected Status %q, got %q", "True", c.Status)
	}
	if c.Reason != "Completed" {
		t.Errorf("expected Reason %q, got %q", "Completed", c.Reason)
	}
	if c.LastProbeTime != "2024-01-15T10:00:00Z" {
		t.Errorf("expected LastProbeTime %q, got %q", "2024-01-15T10:00:00Z", c.LastProbeTime)
	}
	if c.LastTransitionTime != "2024-01-15T10:30:00Z" {
		t.Errorf("expected LastTransitionTime %q, got %q", "2024-01-15T10:30:00Z", c.LastTransitionTime)
	}
}

func TestMapContainerStatuses_Empty(t *testing.T) {
	result := MapContainerStatuses([]corev1.ContainerStatus{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestMapContainers_Empty(t *testing.T) {
	result := MapContainers([]corev1.Container{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestMapPodConditions_Empty(t *testing.T) {
	result := MapPodConditions([]corev1.PodCondition{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestMapJobConditions_Empty(t *testing.T) {
	result := MapJobConditions([]batchv1.JobCondition{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestMapOwnerReferences_Empty(t *testing.T) {
	result := MapOwnerReferences([]metav1.OwnerReference{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

// ensure protos package is used to avoid unused import
var _ *protos.CronJobSpecResponse
