package testutil

import (
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CronJobBuilder struct {
	cronJob batchv1.CronJob
}

func NewCronJobBuilder() *CronJobBuilder {
	jobSpec := NewJobSpecBuilder().Build()
	var defaultTime = metav1.NewTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	return &CronJobBuilder{
		cronJob: batchv1.CronJob{
			TypeMeta: metav1.TypeMeta{
				Kind:       "CronJob",
				APIVersion: "batch/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default-cronjob",
				Namespace: "default",
			},
			Spec: batchv1.CronJobSpec{
				Schedule:          "0 0 * * *",
				ConcurrencyPolicy: "Allow",
				JobTemplate: batchv1.JobTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default-jobtemplate",
						Namespace: "default",
					},
					Spec: jobSpec,
				},
			},
			Status: batchv1.CronJobStatus{
				Active:             []corev1.ObjectReference{},
				LastScheduleTime:   &defaultTime,
				LastSuccessfulTime: &defaultTime,
			},
		},
	}
}

func (b *CronJobBuilder) WithName(name string) *CronJobBuilder {
	b.cronJob.ObjectMeta.Name = name
	return b
}

func (b *CronJobBuilder) WithNamespace(ns string) *CronJobBuilder {
	b.cronJob.ObjectMeta.Namespace = ns
	return b
}

func (b *CronJobBuilder) WithSchedule(schedule string) *CronJobBuilder {
	b.cronJob.Spec.Schedule = schedule
	return b
}

func (b *CronJobBuilder) WithConcurrencyPolicy(policy batchv1.ConcurrencyPolicy) *CronJobBuilder {
	b.cronJob.Spec.ConcurrencyPolicy = policy
	return b
}

func (b *CronJobBuilder) WithJobTemplate(template batchv1.JobTemplateSpec) *CronJobBuilder {
	b.cronJob.Spec.JobTemplate = template
	return b
}

func (b *CronJobBuilder) WithSpec(spec batchv1.CronJobSpec) *CronJobBuilder {
	b.cronJob.Spec = spec
	return b
}

func (b *CronJobBuilder) WithStatus(status batchv1.CronJobStatus) *CronJobBuilder {
	b.cronJob.Status = status
	return b
}

func (b *CronJobBuilder) Build() *batchv1.CronJob {
	return &b.cronJob
}

type CronJobListBuilder struct {
	list batchv1.CronJobList
}

func NewCronJobListBuilder() *CronJobListBuilder {
	return &CronJobListBuilder{
		list: batchv1.CronJobList{
			TypeMeta: metav1.TypeMeta{
				Kind:       "CronJobList",
				APIVersion: "batch/v1",
			},
			ListMeta: metav1.ListMeta{},
			Items:    []batchv1.CronJob{},
		},
	}
}

func (b *CronJobListBuilder) WithItems(items ...*batchv1.CronJob) *CronJobListBuilder {
	b.list.Items = []batchv1.CronJob{}
	for _, item := range items {
		if item != nil {
			b.list.Items = append(b.list.Items, *item)
		}
	}
	return b
}

func (b *CronJobListBuilder) Build() *batchv1.CronJobList {
	return &b.list
}

///

type PodTemplateSpecBuilder struct {
	podTemplateSpec corev1.PodTemplateSpec
}

func NewPodTemplateSpecBuilder() *PodTemplateSpecBuilder {
	containers := []corev1.Container{
		{
			Name:  "default-container",
			Image: "busybox",
			Command: []string{
				"echo", "Hello from CronJob",
			},
		},
	}

	var initContainers = []corev1.Container{
		{
			Name:    "init-myservice",
			Image:   "busybox:1.28",
			Command: []string{"sh", "-c", "until nslookup myservice.default.svc.cluster.local; do echo waiting for myservice; sleep 2; done"},
		},
	}

	var ephemeralContainers = []corev1.EphemeralContainer{
		{
			EphemeralContainerCommon: corev1.EphemeralContainerCommon{
				Name:    "debugger",
				Image:   "busybox",
				Command: []string{"sh"},
				Stdin:   true,
				TTY:     true,
			},
			TargetContainerName: "myapp-container",
		},
	}

	var volumes = []corev1.Volume{
		{
			Name: "workdir",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	return &PodTemplateSpecBuilder{
		podTemplateSpec: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default-job-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers:          containers,
				InitContainers:      initContainers,
				EphemeralContainers: ephemeralContainers,
				Volumes:             volumes,
				RestartPolicy:       corev1.RestartPolicyOnFailure,
			},
		},
	}

}

func (b *PodTemplateSpecBuilder) Build() corev1.PodTemplateSpec {
	return b.podTemplateSpec
}

///

type JobSpecBuilder struct {
	jobSpec batchv1.JobSpec
}

func NewJobSpecBuilder() *JobSpecBuilder {
	parallelism := int32(1)
	completions := int32(1)
	backoffLimit := int32(6)

	return &JobSpecBuilder{
		jobSpec: batchv1.JobSpec{
			Parallelism:  &parallelism,
			Completions:  &completions,
			BackoffLimit: &backoffLimit,
			Template:     NewPodTemplateSpecBuilder().Build(),
		},
	}
}

func (b *JobSpecBuilder) WithParallelism(p int32) *JobSpecBuilder {
	b.jobSpec.Parallelism = &p
	return b
}

func (b *JobSpecBuilder) WithCompletions(c int32) *JobSpecBuilder {
	b.jobSpec.Completions = &c
	return b
}

func (b *JobSpecBuilder) WithBackoffLimit(l int32) *JobSpecBuilder {
	b.jobSpec.BackoffLimit = &l
	return b
}

func (b *JobSpecBuilder) WithPodTemplateSpec(template corev1.PodTemplateSpec) *JobSpecBuilder {
	b.jobSpec.Template = template
	return b
}

func (b *JobSpecBuilder) Build() batchv1.JobSpec {
	return b.jobSpec
}

///

// JobBuilder builds batchv1.Job objects for tests.
type JobBuilder struct {
	job batchv1.Job
}

// NewJobBuilder returns a JobBuilder with defaults and a JobSpec built from JobSpecBuilder.
func NewJobBuilder() *JobBuilder {
	ownerRef := metav1.OwnerReference{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Name:       "my-cronjob",
		UID:        "some-uid",
	}

	now := metav1.NewTime(time.Now())
	completion := metav1.NewTime(now.Time.Add(120 * time.Second))
	ready := int32(1)
	jobCondition := batchv1.JobCondition{

		Type:               batchv1.JobComplete,
		Status:             corev1.ConditionTrue,
		LastProbeTime:      now,
		LastTransitionTime: now,
		Reason:             "Completed",
		Message:            "Job completed successfully",
	}

	jobStatus := batchv1.JobStatus{
		StartTime:      &now,
		CompletionTime: &completion,
		Active:         1,
		Succeeded:      1,
		Failed:         0,
		Ready:          &ready,
		Conditions: []batchv1.JobCondition{
			jobCondition,
		},
	}

	return &JobBuilder{
		job: batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "default-job",
				Namespace:       "default",
				OwnerReferences: []metav1.OwnerReference{ownerRef},
			},
			Spec:   NewJobSpecBuilder().Build(),
			Status: jobStatus,
		},
	}
}

// WithName sets the Job name.
func (b *JobBuilder) WithName(name string) *JobBuilder {
	b.job.Name = name
	b.job.ObjectMeta.Name = name
	return b
}

// WithNamespace sets the Job namespace.
func (b *JobBuilder) WithNamespace(ns string) *JobBuilder {
	b.job.Namespace = ns
	b.job.ObjectMeta.Namespace = ns
	return b
}

// WithLabels sets labels on the Job metadata.
func (b *JobBuilder) WithLabels(labels map[string]string) *JobBuilder {
	if b.job.Labels == nil {
		b.job.Labels = make(map[string]string)
	}
	for k, v := range labels {
		b.job.Labels[k] = v
	}
	return b
}

// WithAnnotations sets annotations on the Job metadata.
func (b *JobBuilder) WithAnnotations(annotations map[string]string) *JobBuilder {
	if b.job.Annotations == nil {
		b.job.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		b.job.Annotations[k] = v
	}
	return b
}

// WithJobSpec sets the JobSpec using an existing JobSpec.
func (b *JobBuilder) WithJobSpec(spec batchv1.JobSpec) *JobBuilder {
	b.job.Spec = spec
	return b
}

// Build returns the pointer to the constructed Job.
func (b *JobBuilder) Build() *batchv1.Job {
	return &b.job
}
