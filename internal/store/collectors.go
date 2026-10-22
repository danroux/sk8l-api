package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog/log"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
)

const watchRetryInterval = 5 * time.Second

// Start begins background collection of CronJobs, Jobs, and Pods.
func (c *CronJobDBStore) Start(ctx context.Context) {
	c.CollectCronjobs(ctx)
	c.CollectJobs(ctx)
	c.CollectPods(ctx)
}

func (c *CronJobDBStore) CollectCronjobs(ctx context.Context) {
	go func() {
		for {
			x, err := c.K8sClient.WatchCronjobs(ctx)
			if err != nil {
				log.Error().
					Err(err).
					Str("operation", "collectCronjobs").
					Msg("WatchCronjobs failed, retrying in 5s")
				select {
				case <-ctx.Done():
					log.Info().
						Str("operation", "collectCronjobs").
						Msg("context canceled, stopping watcher")
					return
				case <-time.After(watchRetryInterval):
					continue
				}
			}

			running := true
			for running {
				select {
				case <-ctx.Done():
					x.Stop()
					log.Info().
						Str("operation", "collectCronjobs").
						Msg("context canceled, stopping watcher")
					return
				case event, more := <-x.ResultChan():
					if !more {
						log.Error().
							Str("operation", "collectCronjobs").
							Msg("WatchCronjobs: Received all Cronjobs. Opening again")
						running = false
						break
					}

					eventCronjob, ok := event.Object.(*batchv1.CronJob)
					if !ok {
						log.Error().
							Str("operation", "collectCronjobs").
							Msg("event.Object.(*batchv1.CronJob) type assertion failed")
						continue
					}
					err := c.DB.Update(func(txn *badger.Txn) error {
						return handleCronJobEvent(txn, event, eventCronjob)
					})
					if err != nil {
						log.Error().
							Err(err).
							Str("operation", "collectCronjobs").
							Msg("handleCronJobEvent failed")
					}
				}
			}
		}
	}()
}

func (c *CronJobDBStore) CollectJobs(ctx context.Context) {
	go func() {
		for {
			x, err := c.K8sClient.WatchJobs(ctx)
			if err != nil {
				log.Error().
					Err(err).
					Str("operation", "collectJobs").
					Msg("WatchJobs failed, retrying in 5s")
				select {
				case <-ctx.Done():
					log.Info().
						Str("operation", "collectJobs").
						Msg("context canceled, stopping watcher")
					return
				case <-time.After(watchRetryInterval):
					continue
				}
			}

			running := true
			for running {
				select {
				case <-ctx.Done():
					x.Stop()
					log.Info().
						Str("operation", "collectJobs").
						Msg("context canceled, stopping watcher")
					return
				case event, more := <-x.ResultChan():
					if !more {
						log.Error().
							Str("operation", "collectJobs").
							Msg("WatchJobs: Received all Jobs. Opening again")
						running = false
						break
					}

					eventJob, ok := event.Object.(*batchv1.Job)
					if !ok {
						log.Error().
							Str("operation", "collectJobs").
							Msg("event.Object.(*batchv1.Job) type assertion failed")
						continue
					}
					err := c.DB.Update(func(txn *badger.Txn) error {
						return handleJobEvent(txn, event, eventJob)
					})
					if err != nil {
						log.Error().
							Err(err).
							Str("operation", "collectJobs").
							Msg("handleJobEvent failed")
					}
				}
			}
		}
	}()
}

func (c *CronJobDBStore) CollectPods(ctx context.Context) {
	go func() {
		for {
			x, err := c.K8sClient.WatchPods(ctx)
			if err != nil {
				log.Error().
					Err(err).
					Str("operation", "collectPods").
					Msg("WatchPods failed, retrying in 5s")
				select {
				case <-ctx.Done():
					log.Info().
						Str("operation", "collectPods").
						Msg("context canceled, stopping watcher")
					return
				case <-time.After(watchRetryInterval):
					continue
				}
			}

			running := true
			for running {
				select {
				case <-ctx.Done():
					x.Stop()
					log.Info().
						Str("operation", "collectPods").
						Msg("context canceled, stopping watcher")
					return
				case event, more := <-x.ResultChan():
					if !more {
						log.Error().
							Str("operation", "collectPods").
							Msg("WatchPods: Received all Pods. Opening again")
						running = false
						break
					}

					eventPod, ok := event.Object.(*corev1.Pod)
					if !ok {
						log.Error().
							Str("operation", "collectPods").
							Msg("event.Object.(*corev1.Pod) type assertion failed")
						continue
					}
					err := c.DB.Update(func(txn *badger.Txn) error {
						return handlePodEvent(txn, event, eventPod)
					})
					if err != nil {
						log.Error().
							Err(err).
							Str("operation", "collectPods").
							Msg("handlePodEvent failed")
					}
				}
			}
		}
	}()
}

func handleCronJobEvent(txn *badger.Txn, event watch.Event, eventCronJob *batchv1.CronJob) error {
	item, err := txn.Get(CronjobsCacheKey)
	if errors.Is(err, badger.ErrKeyNotFound) {
		cronJob := *eventCronJob
		log.Error().
			Err(badger.ErrKeyNotFound).
			Str("operation", "handleCronJobEvent").
			Msg(fmt.Sprintf("storing eventCronJob %s", cronJob.Name))
		cjList := &batchv1.CronJobList{
			Items: []batchv1.CronJob{cronJob},
		}
		var buf bytes.Buffer
		if err := K8sSerialize(cjList, &buf); err != nil {
			log.Error().
				Err(err).
				Str("operation", "handleCronJobEvent").
				Msg("k8sSerializer.Encode")
			return fmt.Errorf("%s: Encode() failed: %w", "sk8l#collectCronjobs", err)
		}
		return storeEntry(txn, CronjobsCacheKey, buf.Bytes(), "sk8l#collectCronjobs")
	}

	err = item.Value(func(stored []byte) error {
		return updateStoredCronjobList(txn, stored, event, eventCronJob)
	})
	if err != nil {
		return fmt.Errorf("sk8l#collectCronjobs: item.Value() failed: %w", err)
	}
	return nil
}

func handleJobEvent(txn *badger.Txn, event watch.Event, eventJob *batchv1.Job) error {
	item, err := txn.Get(JobsCacheKey)
	if err != nil {
		jList := &batchv1.JobList{
			Items: []batchv1.Job{*eventJob},
		}
		var buf bytes.Buffer
		if err := K8sSerialize(jList, &buf); err != nil {
			log.Error().
				Err(err).
				Str("operation", "handleJobEvent").
				Msg("k8sSerializer.Encode")
			return fmt.Errorf("%s: Encode() failed: %w", "sk8l#collectJobs", err)
		}
		return storeEntry(txn, JobsCacheKey, buf.Bytes(), "sk8l#collectJobs")
	}
	err = item.Value(func(stored []byte) error {
		return updateStoredJobList(txn, stored, event, eventJob)
	})
	if err != nil {
		return fmt.Errorf("sk8l#collectJobs: item.Value() failed: %w", err)
	}
	return nil
}

func handlePodEvent(txn *badger.Txn, event watch.Event, eventPod *corev1.Pod) error {
	fKey := fmt.Sprintf(JobPodsKeyFmt, eventPod.Labels["job-name"])
	key := []byte(fKey)
	item, err := txn.Get(key)
	if err != nil {
		podList := &corev1.PodList{
			Items: []corev1.Pod{*eventPod},
		}
		var buf bytes.Buffer
		if err = K8sSerialize(podList, &buf); err != nil {
			log.Error().
				Err(err).
				Str("operation", "handlePodEvent").
				Msg("k8sSerializer.Encode")
			return fmt.Errorf("%s: Encode() failed: %w", "sk8l#collectPods", err)
		}
		return storeEntry(txn, key, buf.Bytes(), "sk8l#collectPods")
	}

	err = item.Value(func(val []byte) error {
		return updateStoredPodList(txn, val, key, eventPod)
	})
	if err != nil {
		return fmt.Errorf("sk8l#collectPods: item.Value() failed: %w", err)
	}
	return nil
}

func updateStoredCronjobList(txn *badger.Txn, stored []byte, event watch.Event, eventCronJob *batchv1.CronJob) error {
	log.Info().
		Str("operation", "updateStoredCronjobList").
		Msg(fmt.Sprintf("Updating with %s", eventCronJob.Name))
	storedCjList := &batchv1.CronJobList{}
	if _, _, err := K8sDeserialize(stored, storedCjList); err != nil {
		log.Error().
			Err(err).
			Str("operation", "updateStoredCronjobList").
			Msg("k8sSerializer.Decode")
		return fmt.Errorf("sk8l#collectCronjobs: Decode() failed: %w", err)
	}

	//revive:disable:identical-switch-branches
	switch event.Type {
	case watch.Added, watch.Modified:
		filterCronJobsList(storedCjList, eventCronJob)
		storedCjList.Items = append(storedCjList.Items, *eventCronJob)
	case watch.Deleted:
		filterCronJobsList(storedCjList, eventCronJob)
	case watch.Bookmark, watch.Error:
		// no-op: explicitly ignored
	default:
		// default case to satisfy revive
	}
	//revive:enable:identical-switch-branches

	var buf bytes.Buffer
	if err := K8sSerialize(storedCjList, &buf); err != nil {
		log.Error().
			Err(err).
			Str("operation", "updateStoredCronjobList").
			Msg("k8sSerializer.Encode")
		return fmt.Errorf("sk8l#collectCronjobs: Encode() failed: %w", err)
	}
	return storeEntry(txn, CronjobsCacheKey, buf.Bytes(), "sk8l#collectJobs")
}

func updateStoredJobList(txn *badger.Txn, stored []byte, event watch.Event, eventJob *batchv1.Job) error {
	storedJList := &batchv1.JobList{}
	if _, _, err := K8sDeserialize(stored, storedJList); err != nil {
		log.Error().
			Err(err).
			Str("operation", "updateStoredJobList").
			Msg("k8sSerializer.Decode")
		return fmt.Errorf("sk8l#collectJobs: Decode() failed: %w", err)
	}

	//revive:disable:identical-switch-branches
	switch event.Type {
	case watch.Added, watch.Modified:
		filterStoredJobList(storedJList, eventJob)
		storedJList.Items = append(storedJList.Items, *eventJob)
	case watch.Deleted:
		filterStoredJobList(storedJList, eventJob)
	case watch.Bookmark, watch.Error:
		// no-op: explicitly ignored
	default:
		// default case to satisfy revive
	}
	//revive:enable:identical-switch-branches

	var buf bytes.Buffer
	if err := K8sSerialize(storedJList, &buf); err != nil {
		log.Error().
			Err(err).
			Str("operation", "updateStoredJobList").
			Msg("k8sSerializer.Encode")
		return fmt.Errorf("sk8l#collectJobs: Encode() failed: %w", err)
	}
	return storeEntry(txn, JobsCacheKey, buf.Bytes(), "sk8l#collectJobs")
}

func updateStoredPodList(txn *badger.Txn, stored []byte, key []byte, eventPod *corev1.Pod) error {
	podList := &corev1.PodList{}
	if _, _, err := K8sDeserialize(stored, podList); err != nil {
		log.Error().
			Err(err).
			Str("operation", "updateStoredPodList").
			Msg("k8sSerializer.Decode")
		return fmt.Errorf("operation#k8sSerializer.Decode: %w", err)
	}
	podList.Items = append(podList.Items, *eventPod)
	var buf bytes.Buffer
	if err := K8sSerialize(podList, &buf); err != nil {
		log.Error().
			Err(err).
			Str("operation", "updateStoredPodList").
			Msg("k8sSerializer.Encode")
		return fmt.Errorf("operation#k8sSerializer.Encode: %w", err)
	}
	return storeEntry(txn, key, buf.Bytes(), "sk8l#collectPods")
}

func filterCronJobsList(storedCjList *batchv1.CronJobList, eventCronJob *batchv1.CronJob) {
	log.Info().
		Str("operation", "filterCronJobsList").
		Msg(fmt.Sprintf("filtering out %s", eventCronJob.Name))
	cjList := &batchv1.CronJobList{}
	for _, cronjob := range storedCjList.Items {
		if cronjob.Name != eventCronJob.Name {
			cjList.Items = append(cjList.Items, cronjob)
		}
	}
	storedCjList.Items = cjList.Items
}

func filterStoredJobList(storedJList *batchv1.JobList, eventJob *batchv1.Job) {
	jList := &batchv1.JobList{}
	for _, job := range storedJList.Items {
		if job.Name != eventJob.Name {
			jList.Items = append(jList.Items, job)
		}
	}
	storedJList.Items = jList.Items
}

func storeEntry(txn *badger.Txn, key []byte, result []byte, errContext string) error {
	log.Info().
		Str("operation", "storeEntry").
		Msg(errContext)
	entry := badger.NewEntry(key, result)
	if err := txn.SetEntry(entry); err != nil {
		log.Printf("Error: %s#txn.SetEntry: %v", errContext, err)
		return fmt.Errorf("%s: txn.SetEntry() failed: %w", errContext, err)
	}
	return nil
}
