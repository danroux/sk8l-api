// Package testutil provides utilities for building API test objects.
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
				Name:              "default-cronjob",
				Namespace:         "default",
				CreationTimestamp: defaultTime,
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

// baseContainerBuilder holds common fields and methods.
type baseContainerBuilder struct {
	command       []string
	restartPolicy corev1.ContainerRestartPolicy
	name          string
	image         string
}

func (b *baseContainerBuilder) WithName(name string) {
	b.name = name
}

func (b *baseContainerBuilder) WithImage(image string) {
	b.image = image
}

func (b *baseContainerBuilder) WithCommand(cmd ...string) {
	b.command = cmd
}

// ContainerBuilder builds a corev1.Container.
type ContainerBuilder struct {
	baseContainerBuilder
}

func NewContainerBuilder() *ContainerBuilder {
	return &ContainerBuilder{
		baseContainerBuilder: baseContainerBuilder{
			name:    "default-container",
			image:   "busybox",
			command: []string{"echo", "Hello from CronJob"},
		},
	}
}

func (b *ContainerBuilder) WithName(name string) *ContainerBuilder {
	b.baseContainerBuilder.WithName(name)
	return b
}

func (b *ContainerBuilder) WithImage(image string) *ContainerBuilder {
	b.baseContainerBuilder.WithImage(image)
	return b
}

func (b *ContainerBuilder) WithCommand(cmd ...string) *ContainerBuilder {
	b.baseContainerBuilder.WithCommand(cmd...)
	return b
}

func (b *ContainerBuilder) WithRestartPolicy(rp corev1.ContainerRestartPolicy) *ContainerBuilder {
	b.baseContainerBuilder.restartPolicy = rp
	return b
}

func (b *ContainerBuilder) WithRestartPolicyAlways() *ContainerBuilder {
	b.WithRestartPolicy(corev1.ContainerRestartPolicyAlways)
	return b
}

func (b *ContainerBuilder) Build() corev1.Container {
	return corev1.Container{
		Name:          b.name,
		Image:         b.image,
		Command:       b.command,
		RestartPolicy: &b.restartPolicy,
	}
}

// EphemeralContainerBuilder builds a corev1.EphemeralContainer with minimal setters.
type EphemeralContainerBuilder struct {
	baseContainerBuilder
	tty bool
}

func NewEphemeralContainerBuilder() *EphemeralContainerBuilder {
	return &EphemeralContainerBuilder{
		baseContainerBuilder: baseContainerBuilder{
			name:    "debugger",
			image:   "busybox",
			command: []string{"sh"},
		},
		tty: true,
	}
}

func (b *EphemeralContainerBuilder) WithName(name string) *EphemeralContainerBuilder {
	b.baseContainerBuilder.WithName(name)
	return b
}

func (b *EphemeralContainerBuilder) WithImage(image string) *EphemeralContainerBuilder {
	b.baseContainerBuilder.WithImage(image)
	return b
}

func (b *EphemeralContainerBuilder) WithCommand(cmd ...string) *EphemeralContainerBuilder {
	b.baseContainerBuilder.WithCommand(cmd...)
	return b
}

func (b *EphemeralContainerBuilder) WithTTY(tty bool) *EphemeralContainerBuilder {
	b.tty = tty
	return b
}

func (b *EphemeralContainerBuilder) Build() corev1.EphemeralContainer {
	return corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:    b.name,
			Image:   b.image,
			Command: b.command,
			TTY:     b.tty,
		},
		TargetContainerName: "myapp-container",
	}
}

type PodTemplateSpecBuilder struct {
	podTemplateSpec corev1.PodTemplateSpec
}

func NewPodTemplateSpecBuilder() *PodTemplateSpecBuilder {
	containers := []corev1.Container{
		NewContainerBuilder().Build(),
	}

	var initContainers = []corev1.Container{
		NewContainerBuilder().
			WithName("init-myservice").
			WithImage("busybox:1.28").
			WithCommand("sh", "-c", "until nslookup myservice.default.svc.cluster.local; do echo waiting for myservice; sleep 2; done").
			Build(),
	}

	var ephemeralContainers = []corev1.EphemeralContainer{
		NewEphemeralContainerBuilder().
			WithName("debugger").
			WithImage("busybox").
			WithCommand("sh").
			Build(),
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

func (b *PodTemplateSpecBuilder) WithInitContainers(containers []corev1.Container) *PodTemplateSpecBuilder {
	b.podTemplateSpec.Spec.InitContainers = containers
	return b
}

// WithSidecarContainers sets the RestartPolicy of all init containers to Always.
func (b *PodTemplateSpecBuilder) WithSidecarContainers() *PodTemplateSpecBuilder {
	rp := corev1.ContainerRestartPolicyAlways

	for i := range b.podTemplateSpec.Spec.InitContainers {
		b.podTemplateSpec.Spec.InitContainers[i].RestartPolicy = &rp
	}

	return b
}

func (b *PodTemplateSpecBuilder) WithContainers(containers []corev1.Container) *PodTemplateSpecBuilder {
	b.podTemplateSpec.Spec.Containers = containers
	return b
}

func (b *PodTemplateSpecBuilder) WithEphemeralContainers(containers []corev1.EphemeralContainer) *PodTemplateSpecBuilder {
	b.podTemplateSpec.Spec.EphemeralContainers = containers
	return b
}

func (b *PodTemplateSpecBuilder) WithRestartPolicy(rp corev1.RestartPolicy) *PodTemplateSpecBuilder {
	b.podTemplateSpec.Spec.RestartPolicy = rp
	return b
}

func (b *PodTemplateSpecBuilder) WithVolumes(volumes []corev1.Volume) *PodTemplateSpecBuilder {
	b.podTemplateSpec.Spec.Volumes = volumes
	return b
}

type JobSpecBuilder struct {
	jobSpec            batchv1.JobSpec
	podTemplateBuilder *PodTemplateSpecBuilder
}

const (
	defaultParallelism          int32 = 1
	defaultCompletions          int32 = 1
	defaultBackoffLimit         int32 = 6
	jobCompletionTimeoutSeconds       = 120
)

var (
	parallelism  = defaultParallelism
	completions  = defaultCompletions
	backoffLimit = defaultBackoffLimit
)

func NewJobSpecBuilder() *JobSpecBuilder {
	podTemsplateSpecBuilder := NewPodTemplateSpecBuilder()

	return &JobSpecBuilder{
		jobSpec: batchv1.JobSpec{
			Parallelism:  &parallelism,
			Completions:  &completions,
			BackoffLimit: &backoffLimit,
			Template:     podTemsplateSpecBuilder.Build(),
		},
		podTemplateBuilder: podTemsplateSpecBuilder,
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

func (b *JobSpecBuilder) WithRestartPolicyAlways() *JobSpecBuilder {
	b.podTemplateBuilder.WithRestartPolicy(corev1.RestartPolicyAlways)
	b.jobSpec.Template = b.podTemplateBuilder.Build()
	return b
}

func (b *JobSpecBuilder) WithRestartPolicyNever() *JobSpecBuilder {
	b.podTemplateBuilder.WithRestartPolicy(corev1.RestartPolicyNever)
	b.jobSpec.Template = b.podTemplateBuilder.Build()
	return b
}

func (b *JobSpecBuilder) WithRestartPolicyOnFailure() *JobSpecBuilder {
	b.podTemplateBuilder.WithRestartPolicy(corev1.RestartPolicyOnFailure)
	b.jobSpec.Template = b.podTemplateBuilder.Build()
	return b
}

func (b *JobSpecBuilder) Build() batchv1.JobSpec {
	return b.jobSpec
}

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
	completion := metav1.NewTime(now.Time.Add(jobCompletionTimeoutSeconds * time.Second))
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

func (b *JobBuilder) WithCronjob(cronjob batchv1.CronJob) *JobBuilder {
	ownerRef := metav1.OwnerReference{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Name:       cronjob.Name,
		UID:        "some-uid",
	}
	b.job.ObjectMeta.OwnerReferences = []metav1.OwnerReference{ownerRef}
	return b
}

// Build returns the pointer to the constructed Job.
func (b *JobBuilder) Build() *batchv1.Job {
	return &b.job
}
