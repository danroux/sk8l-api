package mapper

import (
	"testing"
	"time"

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
	tests := []struct {
		name     string
		input    []metav1.OwnerReference
		wantLen  int
		wantCtrl bool
	}{
		{
			name:    "empty returns empty",
			input:   []metav1.OwnerReference{},
			wantLen: 0,
		},
		{
			name: "nil controller is false",
			input: []metav1.OwnerReference{
				{APIVersion: "batch/v1", Kind: "CronJob", Name: "my-cronjob", UID: "uid-123", Controller: nil},
			},
			wantLen:  1,
			wantCtrl: false,
		},
		{
			name: "true controller is true",
			input: []metav1.OwnerReference{
				{APIVersion: "batch/v1", Kind: "CronJob", Name: "my-cronjob", UID: "uid-123", Controller: &ctrl},
			},
			wantLen:  1,
			wantCtrl: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapOwnerReferences(tt.input)
			if len(result) != tt.wantLen {
				t.Fatalf("expected %d owner references, got %d", tt.wantLen, len(result))
			}
			if tt.wantLen > 0 && result[0].Controller != tt.wantCtrl {
				t.Errorf("expected Controller %v, got %v", tt.wantCtrl, result[0].Controller)
			}
		})
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
			{APIVersion: "batch/v1", Kind: "CronJob", Name: "my-cronjob", UID: "uid-123", Controller: &ctrl},
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
	startedAt := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	finishedAt := metav1.NewTime(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))

	tests := []struct {
		name     string
		input    *corev1.ContainerStateTerminated
		wantNil  bool
		wantExit int32
	}{
		{
			name:    "nil returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "maps all fields",
			input: &corev1.ContainerStateTerminated{
				ExitCode:    1,
				Signal:      9,
				Reason:      "Error",
				Message:     "OOM killed",
				StartedAt:   startedAt,
				FinishedAt:  finishedAt,
				ContainerID: "docker://abc123",
			},
			wantNil:  false,
			wantExit: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapContainerStateTerminated(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Error("expected nil result")
				}
				return
			}
			if result.ExitCode != tt.wantExit {
				t.Errorf("expected ExitCode %d, got %d", tt.wantExit, result.ExitCode)
			}
			if result.Signal != 9 {
				t.Errorf("expected Signal 9, got %d", result.Signal)
			}
			if result.Reason != "Error" {
				t.Errorf("expected Reason %q, got %q", "Error", result.Reason)
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
}

func TestMapContainerState(t *testing.T) {
	startedAt := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))

	tests := []struct {
		name          string
		input         corev1.ContainerState
		wantWaiting   bool
		wantRunning   bool
		wantTerminated bool
	}{
		{
			name: "waiting state",
			input: corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
			},
			wantWaiting: true,
		},
		{
			name: "running state",
			input: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{StartedAt: startedAt},
			},
			wantRunning: true,
		},
		{
			name: "terminated state",
			input: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{ExitCode: 0, Reason: "Completed"},
			},
			wantTerminated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapContainerState(tt.input)
			if tt.wantWaiting && result.Waiting == nil {
				t.Error("expected Waiting to be set")
			}
			if !tt.wantWaiting && result.Waiting != nil {
				t.Error("expected Waiting to be nil")
			}
			if tt.wantRunning && result.Running == nil {
				t.Error("expected Running to be set")
			}
			if !tt.wantRunning && result.Running != nil {
				t.Error("expected Running to be nil")
			}
			if tt.wantTerminated && result.Terminated == nil {
				t.Error("expected Terminated to be set")
			}
			if !tt.wantTerminated && result.Terminated != nil {
				t.Error("expected Terminated to be nil")
			}
		})
	}
}

func TestMapContainerStatus(t *testing.T) {
	started := true

	tests := []struct {
		name        string
		input       corev1.ContainerStatus
		wantStarted bool
	}{
		{
			name: "nil started is false",
			input: corev1.ContainerStatus{
				Name:    "my-container",
				Started: nil,
			},
			wantStarted: false,
		},
		{
			name: "true started is true",
			input: corev1.ContainerStatus{
				Name:         "my-container",
				Ready:        true,
				RestartCount: 3,
				Image:        "nginx:latest",
				ImageID:      "sha256:abc",
				ContainerID:  "docker://xyz",
				Started:      &started,
			},
			wantStarted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapContainerStatus(tt.input)
			if result.Started != tt.wantStarted {
				t.Errorf("expected Started %v, got %v", tt.wantStarted, result.Started)
			}
			if result.Name != tt.input.Name {
				t.Errorf("expected Name %q, got %q", tt.input.Name, result.Name)
			}
		})
	}
}

func TestMapPodStatus(t *testing.T) {
	startTime := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	s := corev1.PodStatus{
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

	result := MapPodStatus(s)

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

	tests := []struct {
		name    string
		input   corev1.PodSpec
		wantTGP int64
	}{
		{
			name: "nil TerminationGracePeriodSeconds defaults to 0",
			input: corev1.PodSpec{
				TerminationGracePeriodSeconds: nil,
			},
			wantTGP: 0,
		},
		{
			name: "maps all fields",
			input: corev1.PodSpec{
				RestartPolicy:                 corev1.RestartPolicyNever,
				ServiceAccountName:            "my-sa",
				NodeName:                      "node-1",
				NodeSelector:                  map[string]string{"zone": "us-east-1"},
				TerminationGracePeriodSeconds: &tgps,
				Containers:                    []corev1.Container{{Name: "main", Image: "nginx:latest"}},
				InitContainers:                []corev1.Container{{Name: "init", Image: "busybox"}},
			},
			wantTGP: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapPodSpec(tt.input)
			if result.TerminationGracePeriodSeconds != tt.wantTGP {
				t.Errorf("expected TerminationGracePeriodSeconds %d, got %d", tt.wantTGP, result.TerminationGracePeriodSeconds)
			}
		})
	}

	t.Run("maps all fields fully", func(t *testing.T) {
		spec := corev1.PodSpec{
			RestartPolicy:                 corev1.RestartPolicyNever,
			ServiceAccountName:            "my-sa",
			NodeName:                      "node-1",
			TerminationGracePeriodSeconds: &tgps,
			Containers:                    []corev1.Container{{Name: "main"}},
			InitContainers:                []corev1.Container{{Name: "init"}},
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
		if len(result.Containers) != 1 {
			t.Errorf("expected 1 container, got %d", len(result.Containers))
		}
		if len(result.InitContainers) != 1 {
			t.Errorf("expected 1 init container, got %d", len(result.InitContainers))
		}
	})
}

func TestMapJobCondition(t *testing.T) {
	lastProbe := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	lastTransition := metav1.NewTime(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))

	tests := []struct {
		name    string
		input   *batchv1.JobCondition
		wantNil bool
	}{
		{
			name:    "nil returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "maps all fields",
			input: &batchv1.JobCondition{
				Type:               batchv1.JobFailed,
				Status:             "False",
				LastProbeTime:      lastProbe,
				LastTransitionTime: lastTransition,
				Reason:             "BackoffLimitExceeded",
				Message:            "Job has reached the specified backoff limit",
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapJobCondition(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Error("expected nil result")
				}
				return
			}
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

	tests := []struct {
		name               string
		input              batchv1.JobSpec
		wantParallelism    int32
		wantCompletions    int32
		wantDeadline       int64
		wantBackoff        int32
		wantSuspend        bool
		wantCompletionMode string
	}{
		{
			name:               "nil pointers default to zero values",
			input:              batchv1.JobSpec{},
			wantParallelism:    0,
			wantCompletions:    0,
			wantDeadline:       0,
			wantBackoff:        0,
			wantSuspend:        false,
			wantCompletionMode: "",
		},
		{
			name: "maps all fields",
			input: batchv1.JobSpec{
				Parallelism:           &parallelism,
				Completions:           &completions,
				ActiveDeadlineSeconds: &activeDeadlineSeconds,
				BackoffLimit:          &backoffLimit,
				Suspend:               &suspend,
				CompletionMode:        &completionMode,
			},
			wantParallelism:    2,
			wantCompletions:    5,
			wantDeadline:       300,
			wantBackoff:        3,
			wantSuspend:        true,
			wantCompletionMode: "Indexed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapJobSpec(tt.input)
			if result.Parallelism != tt.wantParallelism {
				t.Errorf("expected Parallelism %d, got %d", tt.wantParallelism, result.Parallelism)
			}
			if result.Completions != tt.wantCompletions {
				t.Errorf("expected Completions %d, got %d", tt.wantCompletions, result.Completions)
			}
			if result.ActiveDeadlineSeconds != tt.wantDeadline {
				t.Errorf("expected ActiveDeadlineSeconds %d, got %d", tt.wantDeadline, result.ActiveDeadlineSeconds)
			}
			if result.BackoffLimit != tt.wantBackoff {
				t.Errorf("expected BackoffLimit %d, got %d", tt.wantBackoff, result.BackoffLimit)
			}
			if result.Suspend != tt.wantSuspend {
				t.Errorf("expected Suspend %v, got %v", tt.wantSuspend, result.Suspend)
			}
			if result.CompletionMode != tt.wantCompletionMode {
				t.Errorf("expected CompletionMode %q, got %q", tt.wantCompletionMode, result.CompletionMode)
			}
		})
	}
}

func TestMapCronJobSpec(t *testing.T) {
	suspend := true
	successfulJobsHistoryLimit := int32(5)
	failedJobsHistoryLimit := int32(3)
	timezone := "Europe/Berlin"
	startingDeadlineSeconds := int64(60)

	tests := []struct {
		name                       string
		input                      batchv1.CronJobSpec
		wantSchedule               string
		wantTimezone               string
		wantConcurrencyPolicy      string
		wantSuspend                bool
		wantSuccessfulHistoryLimit int32
		wantFailedHistoryLimit     int32
		wantStartingDeadline       int64
	}{
		{
			name:         "nil pointers default to zero values",
			input:        batchv1.CronJobSpec{Schedule: "0 * * * *"},
			wantSchedule: "0 * * * *",
		},
		{
			name: "maps all fields",
			input: batchv1.CronJobSpec{
				Schedule:                   "*/5 * * * *",
				TimeZone:                   &timezone,
				ConcurrencyPolicy:          batchv1.ForbidConcurrent,
				Suspend:                    &suspend,
				SuccessfulJobsHistoryLimit: &successfulJobsHistoryLimit,
				FailedJobsHistoryLimit:     &failedJobsHistoryLimit,
				StartingDeadlineSeconds:    &startingDeadlineSeconds,
			},
			wantSchedule:               "*/5 * * * *",
			wantTimezone:               "Europe/Berlin",
			wantConcurrencyPolicy:      "Forbid",
			wantSuspend:                true,
			wantSuccessfulHistoryLimit: 5,
			wantFailedHistoryLimit:     3,
			wantStartingDeadline:       60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapCronJobSpec(tt.input)
			if result.Schedule != tt.wantSchedule {
				t.Errorf("expected Schedule %q, got %q", tt.wantSchedule, result.Schedule)
			}
			if result.Timezone != tt.wantTimezone {
				t.Errorf("expected Timezone %q, got %q", tt.wantTimezone, result.Timezone)
			}
			if result.ConcurrencyPolicy != tt.wantConcurrencyPolicy {
				t.Errorf("expected ConcurrencyPolicy %q, got %q", tt.wantConcurrencyPolicy, result.ConcurrencyPolicy)
			}
			if result.Suspend != tt.wantSuspend {
				t.Errorf("expected Suspend %v, got %v", tt.wantSuspend, result.Suspend)
			}
			if result.SuccessfulJobsHistoryLimit != tt.wantSuccessfulHistoryLimit {
				t.Errorf("expected SuccessfulJobsHistoryLimit %d, got %d", tt.wantSuccessfulHistoryLimit, result.SuccessfulJobsHistoryLimit)
			}
			if result.FailedJobsHistoryLimit != tt.wantFailedHistoryLimit {
				t.Errorf("expected FailedJobsHistoryLimit %d, got %d", tt.wantFailedHistoryLimit, result.FailedJobsHistoryLimit)
			}
			if result.StartingDeadlineSeconds != tt.wantStartingDeadline {
				t.Errorf("expected StartingDeadlineSeconds %d, got %d", tt.wantStartingDeadline, result.StartingDeadlineSeconds)
			}
		})
	}
}

func TestMapUncountedTerminatedPods(t *testing.T) {
	tests := []struct {
		name        string
		input       *batchv1.UncountedTerminatedPods
		wantNil     bool
		wantSuccLen int
		wantFailLen int
	}{
		{
			name:    "nil returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "maps succeeded and failed",
			input: &batchv1.UncountedTerminatedPods{
				Succeeded: []types.UID{"uid-1", "uid-2"},
				Failed:    []types.UID{"uid-3"},
			},
			wantNil:     false,
			wantSuccLen: 2,
			wantFailLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapUncountedTerminatedPods(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Error("expected nil result")
				}
				return
			}
			if len(result.Succeeded) != tt.wantSuccLen {
				t.Errorf("expected %d succeeded, got %d", tt.wantSuccLen, len(result.Succeeded))
			}
			if len(result.Failed) != tt.wantFailLen {
				t.Errorf("expected %d failed, got %d", tt.wantFailLen, len(result.Failed))
			}
		})
	}
}

func TestMapCustomJobStatus(t *testing.T) {
	startTime := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	completionTime := metav1.NewTime(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))
	terminating := int32(1)
	ready := int32(2)

	tests := []struct {
		name            string
		input           batchv1.JobStatus
		wantTerminating int32
		wantReady       int32
		wantUTPNil      bool
	}{
		{
			name:            "nil pointers default to zero values",
			input:           batchv1.JobStatus{},
			wantTerminating: 0,
			wantReady:       0,
			wantUTPNil:      true,
		},
		{
			name: "maps all fields",
			input: batchv1.JobStatus{
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
			},
			wantTerminating: 1,
			wantReady:       2,
			wantUTPNil:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapCustomJobStatus(tt.input)
			if result.Terminating != tt.wantTerminating {
				t.Errorf("expected Terminating %d, got %d", tt.wantTerminating, result.Terminating)
			}
			if result.Ready != tt.wantReady {
				t.Errorf("expected Ready %d, got %d", tt.wantReady, result.Ready)
			}
			if tt.wantUTPNil && result.UncountedTerminatedPods != nil {
				t.Error("expected UncountedTerminatedPods to be nil")
			}
			if !tt.wantUTPNil && result.UncountedTerminatedPods == nil {
				t.Error("expected UncountedTerminatedPods to be set")
			}
		})
	}
}

func TestMapCustomJobConditions(t *testing.T) {
	lastProbe := metav1.NewTime(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	lastTransition := metav1.NewTime(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC))

	tests := []struct {
		name    string
		input   []batchv1.JobCondition
		wantLen int
	}{
		{
			name:    "empty returns empty",
			input:   []batchv1.JobCondition{},
			wantLen: 0,
		},
		{
			name: "maps all fields",
			input: []batchv1.JobCondition{
				{
					Type:               batchv1.JobComplete,
					Status:             "True",
					LastProbeTime:      lastProbe,
					LastTransitionTime: lastTransition,
					Reason:             "Completed",
					Message:            "job completed successfully",
				},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapCustomJobConditions(tt.input)
			if len(result) != tt.wantLen {
				t.Fatalf("expected %d conditions, got %d", tt.wantLen, len(result))
			}
			if tt.wantLen == 0 {
				return
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
		})
	}
}
