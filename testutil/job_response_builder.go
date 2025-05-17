package testutil

import (
	"github.com/danroux/sk8l/protos"
)

type JobResponseBuilder struct {
	job protos.JobResponse
}

func NewJobResponseBuilder() *JobResponseBuilder {
	return &JobResponseBuilder{
		job: protos.JobResponse{
			Name:                  "default-name",
			Namespace:             "default-namespace",
			CreationTimestamp:     "1970-01-01T00:00:00Z",
			Uuid:                  "default-uuid",
			Generation:            1,
			Duration:              "0s",
			DurationInS:           0,
			Succeeded:             false,
			Failed:                false,
			WithSidecarContainers: false,
			Pods:                  []*protos.PodResponse{},
			TerminationReasons:    []*protos.TerminationReason{},
		},
	}
}

func (b *JobResponseBuilder) WithName(name string) *JobResponseBuilder {
	b.job.Name = name
	return b
}

func (b *JobResponseBuilder) WithNamespace(ns string) *JobResponseBuilder {
	b.job.Namespace = ns
	return b
}

func (b *JobResponseBuilder) WithSucceeded(succeeded bool) *JobResponseBuilder {
	b.job.Succeeded = succeeded
	return b
}

func (b *JobResponseBuilder) WithFailed(failed bool) *JobResponseBuilder {
	b.job.Failed = failed
	return b
}

func (b *JobResponseBuilder) WithPods(pods []*protos.PodResponse) *JobResponseBuilder {
	b.job.Pods = pods
	return b
}

func (b *JobResponseBuilder) WithStatus(status *protos.JobStatus) *JobResponseBuilder {
	b.job.Status = status
	return b
}

func (b *JobResponseBuilder) Build() *protos.JobResponse {
	return &b.job
}
