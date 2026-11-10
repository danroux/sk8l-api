// Package store defines the storage abstraction used by sk8l for querying cached Kubernetes resources.
package store

import (
	"context"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// Store defines the storage abstraction used by sk8l for querying cached Kubernetes resources.
type Store interface {
	FindCronjobs() (*batchv1.CronJobList, error)
	FindCronjob(ctx context.Context, cronjobNamespace, cronjobName string) (*batchv1.CronJob, error)
	FindJobs() (*batchv1.JobList, error)
	FindJobsMapped(ctx context.Context) (map[string][]*batchv1.Job, error)
	FindJobPodsForJob(job *batchv1.Job) (*corev1.PodList, error)

	// Subscribe returns a channel that receives a signal when relevant store
	// resources are updated. The channel has capacity 1 (coalesced) and is closed
	// when ctx is done or the store is closed.
	Subscribe(ctx context.Context) <-chan struct{}
	Ready() bool

	// Health returns a consistent snapshot of the store state: ready reports whether the
	// store can serve data, and degradedFor is how long a previously synced store has been
	// degraded (0 when healthy or not yet synced). Reading both in one call avoids races
	// between separate Ready()/degraded-duration reads.
	Health() (ready bool, degradedFor time.Duration)
}

// Starter is an optional interface implemented by stores that require
// background synchronization or event collection before serving traffic.
type Starter interface {
	Start(ctx context.Context)
}
