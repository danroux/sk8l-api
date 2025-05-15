package testutil

import (
	"github.com/danroux/sk8l/protos"
	v11 "k8s.io/api/batch/v1"
)

type CronjobResponseBuilder struct {
	cronjob protos.CronjobResponse
}

func NewCronjobResponseBuilder() *CronjobResponseBuilder {
	return &CronjobResponseBuilder{
		cronjob: protos.CronjobResponse{
			Name:               "default-cronjob",
			Namespace:          "default-namespace",
			Uid:                "default-uid",
			ContainerCommands:  map[string]*protos.ContainerCommands{},
			CreationTimestamp:  "1970-01-01T00:00:00Z",
			Definition:         "",
			LastSuccessfulTime: "",
			LastScheduleTime:   "",
			Active:             false,
			Jobs:               []*protos.JobResponse{},
			RunningJobs:        []*protos.JobResponse{},
			RunningJobsPods:    []*protos.PodResponse{},
			JobsPods:           []*protos.PodResponse{},
			LastDuration:       0,
			CurrentDuration:    0,
			Spec:               nil,
			Failed:             false,
		},
	}
}

func (b *CronjobResponseBuilder) WithName(name string) *CronjobResponseBuilder {
	b.cronjob.Name = name
	return b
}

func (b *CronjobResponseBuilder) WithNamespace(ns string) *CronjobResponseBuilder {
	b.cronjob.Namespace = ns
	return b
}

func (b *CronjobResponseBuilder) WithUID(uid string) *CronjobResponseBuilder {
	b.cronjob.Uid = uid
	return b
}

func (b *CronjobResponseBuilder) WithContainerCommands(cmds map[string]*protos.ContainerCommands) *CronjobResponseBuilder {
	b.cronjob.ContainerCommands = cmds
	return b
}

func (b *CronjobResponseBuilder) WithCreationTimestamp(ts string) *CronjobResponseBuilder {
	b.cronjob.CreationTimestamp = ts
	return b
}

func (b *CronjobResponseBuilder) WithDefinition(def string) *CronjobResponseBuilder {
	b.cronjob.Definition = def
	return b
}

func (b *CronjobResponseBuilder) WithLastSuccessfulTime(t string) *CronjobResponseBuilder {
	b.cronjob.LastSuccessfulTime = t
	return b
}

func (b *CronjobResponseBuilder) WithLastScheduleTime(t string) *CronjobResponseBuilder {
	b.cronjob.LastScheduleTime = t
	return b
}

func (b *CronjobResponseBuilder) WithActive(active bool) *CronjobResponseBuilder {
	b.cronjob.Active = active
	return b
}

func (b *CronjobResponseBuilder) WithJobs(jobs []*protos.JobResponse) *CronjobResponseBuilder {
	b.cronjob.Jobs = jobs
	return b
}

func (b *CronjobResponseBuilder) WithRunningJobs(jobs []*protos.JobResponse) *CronjobResponseBuilder {
	b.cronjob.RunningJobs = jobs
	return b
}

func (b *CronjobResponseBuilder) WithRunningJobsPods(pods []*protos.PodResponse) *CronjobResponseBuilder {
	b.cronjob.RunningJobsPods = pods
	return b
}

func (b *CronjobResponseBuilder) WithJobsPods(pods []*protos.PodResponse) *CronjobResponseBuilder {
	b.cronjob.JobsPods = pods
	return b
}

func (b *CronjobResponseBuilder) WithLastDuration(dur int64) *CronjobResponseBuilder {
	b.cronjob.LastDuration = dur
	return b
}

func (b *CronjobResponseBuilder) WithCurrentDuration(dur int64) *CronjobResponseBuilder {
	b.cronjob.CurrentDuration = dur
	return b
}

func (b *CronjobResponseBuilder) WithSpec(spec *v11.CronJobSpec) *CronjobResponseBuilder {
	b.cronjob.Spec = spec
	return b
}

func (b *CronjobResponseBuilder) WithFailed(failed bool) *CronjobResponseBuilder {
	b.cronjob.Failed = failed
	return b
}

func (b *CronjobResponseBuilder) Build() *protos.CronjobResponse {
	return &b.cronjob
}
