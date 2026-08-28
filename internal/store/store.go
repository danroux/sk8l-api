// Package store provides the Badger-backed cache layer for CronJob, Job and Pod data.
package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/danroux/sk8l/internal/k8s"
	"github.com/danroux/sk8l/internal/logger"
	badger "github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sproto "k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	JobPodsKeyFmt  = "jobs_pods_for_job_%s"
	CronjobsKeyFmt = "sk8l_cronjob_%s_%s"
	BadgerTTL      = 15 * time.Second
	RefreshSeconds = 10
)

var (
	ErrK8sClientRequired = errors.New("NewCronJobDBStore: K8sClient must be provided")
	CronjobsCacheKey     = []byte("sk8l_cronjobs")
	JobsMappedCacheKey   = []byte("sk8l_jobs_mapped")
	JobsCacheKey         = []byte("sk8l_jobs")
	k8sSerializer        = k8sproto.NewSerializer(scheme.Scheme, scheme.Scheme)
)

type APICall func() ([]byte, error)

type CronJobDBStore struct {
	K8sClient k8s.ClientInterface
	*badger.DB
	l zerolog.Logger
}

type CronJobDBStoreOptionFn func(*CronJobDBStore) error

func NewCronJobDBStore(optsFn ...CronJobDBStoreOptionFn) (*CronJobDBStore, error) {
	cjdbs := &CronJobDBStore{
		l: log.With().Str("component", "db_store").Logger(),
	}

	for _, opt := range optsFn {
		if err := opt(cjdbs); err != nil {
			return nil, err
		}
	}

	if cjdbs.K8sClient == nil {
		return nil, ErrK8sClientRequired
	}

	if cjdbs.DB == nil {
		cjdbs.l.Info().Msg("NewCronJobDBStore: DB not provided, setting default one")
		if err := WithDefaultDB()(cjdbs); err != nil {
			return nil, err
		}
	}

	return cjdbs, nil
}

func WithDB(db *badger.DB) CronJobDBStoreOptionFn {
	return func(cjdbs *CronJobDBStore) error {
		cjdbs.DB = db
		return nil
	}
}

func WithK8sClient(k8sClient k8s.ClientInterface) CronJobDBStoreOptionFn {
	return func(cjdbs *CronJobDBStore) error {
		cjdbs.K8sClient = k8sClient
		return nil
	}
}

func WithDefaultK8sClient(k8sNamespace string) CronJobDBStoreOptionFn {
	return func(cjdbs *CronJobDBStore) error {
		k8sClient, err := k8s.NewClient(
			k8s.WithNamespace(k8sNamespace),
			k8s.WithLogger(log.With().Str("component", "k8s").Logger()),
		)
		if err != nil {
			return fmt.Errorf("failed to create default k8s client: %w", err)
		}
		cjdbs.K8sClient = k8sClient
		return nil
	}
}

func WithDefaultDB() CronJobDBStoreOptionFn {
	return func(cjdbs *CronJobDBStore) error {
		badgerLogger := logger.NewBadgerLogger(zerolog.GlobalLevel())
		badgerOpts := badger.DefaultOptions("/tmp/badger").WithLogger(badgerLogger)
		db, err := badger.Open(badgerOpts)
		if err != nil {
			return fmt.Errorf("failed to open Badger DB: %w", err)
		}
		cjdbs.DB = db
		return nil
	}
}

// Ping verifies that the Badger DB is open and responsive by running a
// no-op read transaction. It returns an error if the DB is nil, closed,
// or if the view transaction fails.
func (c *CronJobDBStore) Ping() error {
	if c.DB == nil || c.DB.IsClosed() {
		return errors.New("db is unavailable")
	}

	return c.DB.View(func(_ *badger.Txn) error { return nil })
}

func (c *CronJobDBStore) GetAndStore(key []byte, apiCall APICall) ([]byte, error) {
	var valueResponse []byte
	err := c.DB.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			apiResult, err := apiCall()
			if err != nil {
				return fmt.Errorf("apiCall failed: %w", err)
			}
			entry := badger.NewEntry(key, apiResult).WithTTL(BadgerTTL)
			if err = txn.SetEntry(entry); err != nil {
				c.l.Error().Err(err).Msg("Error: getAndStore#txn.SetEntry")
				return fmt.Errorf("sk8l#getAndStore: txn.SetEntry() failed: %w", err)
			}
			valueResponse = append([]byte{}, apiResult...)
			return nil
		} else if err != nil {
			return fmt.Errorf("sk8l#getAndStore: DB.Update() failed: %w", err)
		}

		err = item.Value(func(val []byte) error {
			valueResponse = append([]byte{}, val...)
			return nil
		})
		if err != nil {
			return fmt.Errorf("sk8l#getAndStore: DB.Update() failed: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("sk8l#getAndStore: DB.Update() failed: %w", err)
	}

	return valueResponse, nil
}

func (c *CronJobDBStore) Get(key []byte) ([]byte, error) {
	var valueResponse []byte
	err := c.DB.View(func(txn *badger.Txn) error {
		current, err := txn.Get(key)
		if err != nil {
			return fmt.Errorf("sk8l#get: txn.Get() failed: %w", err)
		}

		err = current.Value(func(val []byte) error {
			valueResponse = append([]byte{}, val...)
			return nil
		})
		if err != nil {
			c.l.Error().Err(err).Msg("get#current.Value")
			return fmt.Errorf("sk8l#get: current.Value() failed: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("sk8l#get: DB.View() failed: %w", err)
	}

	return valueResponse, nil
}

func (c *CronJobDBStore) FindCronjobs() (*batchv1.CronJobList, error) {
	cronjobs, err := c.Get(CronjobsCacheKey)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return &batchv1.CronJobList{}, nil
		}
		return nil, fmt.Errorf("findCronjobs#get: %w", err)
	}

	cronjobList := &batchv1.CronJobList{}
	if _, _, err = k8sSerializer.Decode(cronjobs, nil, cronjobList); err != nil {
		return nil, fmt.Errorf("findCronjobs#Decode: %w", err)
	}
	return cronjobList, nil
}

func (c *CronJobDBStore) FindCronjob(ctx context.Context, cronjobNamespace, cronjobName string) (*batchv1.CronJob, error) {
	gCjCall := func() ([]byte, error) {
		cronjob, err := c.K8sClient.GetCronjob(ctx, cronjobNamespace, cronjobName)
		if err != nil {
			return nil, fmt.Errorf("GetCronjob failed: %w", err)
		}
		var buf bytes.Buffer
		if err := k8sSerializer.Encode(cronjob, &buf); err != nil {
			c.l.Error().Err(err).Msg("findCronjob#k8sSerializer.Encode")
			return nil, fmt.Errorf("k8sSerializer.Encode failed: %w", err)
		}
		return buf.Bytes(), nil
	}

	cacheKey := []byte(fmt.Sprintf(CronjobsKeyFmt, cronjobNamespace, cronjobName))
	cronjobValue, err := c.GetAndStore(cacheKey, gCjCall)
	if err != nil {
		return nil, fmt.Errorf("findCronjob#getAndStore: %w", err)
	}

	cronjob := &batchv1.CronJob{}
	if _, _, err = k8sSerializer.Decode(cronjobValue, nil, cronjob); err != nil {
		return nil, fmt.Errorf("findCronjob#Decode: %w", err)
	}
	return cronjob, nil
}

func (c *CronJobDBStore) FindJobs() (*batchv1.JobList, error) {
	jobs, err := c.Get(JobsCacheKey)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return &batchv1.JobList{}, nil
		}
		return nil, fmt.Errorf("findJobs#get: %w", err)
	}

	jobList := &batchv1.JobList{}
	if _, _, err = k8sSerializer.Decode(jobs, nil, jobList); err != nil {
		return nil, fmt.Errorf("findJobs#Decode: %w", err)
	}
	return jobList, nil
}

func (c *CronJobDBStore) FindJobsMapped(ctx context.Context) (map[string][]*batchv1.Job, error) {
	jobs, err := c.GetAndStore(JobsMappedCacheKey, func() ([]byte, error) {
		jobList, err := c.K8sClient.GetAllJobs(ctx)
		if err != nil {
			return nil, fmt.Errorf("GetAllJobs failed: %w", err)
		}
		var buf bytes.Buffer
		if err := k8sSerializer.Encode(jobList, &buf); err != nil {
			log.Error().
				Err(err).
				Str("operation", "FindJobsMapped").
				Msg("k8sSerializer.Encode")
			return nil, fmt.Errorf("k8sSerializer.Encode failed: %w", err)
		}
		return buf.Bytes(), nil
	})

	if err != nil {
		return nil, fmt.Errorf("FindJobsMapped#GetAndStore: %w", err)
	}

	jobList := &batchv1.JobList{}
	if _, _, err := k8sSerializer.Decode(jobs, nil, jobList); err != nil {
		return nil, fmt.Errorf("FindJobsMapped#Decode: %w", err)
	}

	mapped := make(map[string][]*batchv1.Job)
	for i := range jobList.Items {
		job := &jobList.Items[i]
		for _, owr := range job.OwnerReferences {
			mapped[owr.Name] = append(mapped[owr.Name], job)
		}
	}
	return mapped, nil
}

func (c *CronJobDBStore) FindJobPodsForJob(job *batchv1.Job) (*corev1.PodList, error) {
	fKey := fmt.Sprintf(JobPodsKeyFmt, job.Name)
	key := []byte(fKey)
	collection := &corev1.PodList{}

	err := c.DB.View(func(txn *badger.Txn) error {
		current, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		} else if err != nil {
			return fmt.Errorf("sk8l#findJobPodsForJob: txn.Get() failed: %w", err)
		}
		err = current.Value(func(val []byte) error {
			_, _, err = k8sSerializer.Decode(val, nil, collection)
			if err != nil {
				log.Error().
					Err(err).
					Str("operation", "findJobPodsForJob").
					Msg("proto.Unmarshal")
				return fmt.Errorf("sk8l#findJobPodsForJob: proto.Unmarshal() failed: %w", err)
			}
			return nil
		})
		if err != nil {
			log.Error().
				Err(err).
				Str("operation", "findJobPodsForJob").
				Msg("current.Value")
			return fmt.Errorf("sk8l#findJobPodsForJob: current.Value() failed: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("FindJobPodsForJob#DB.View: %w", err)
	}

	podItems := []corev1.Pod{}
	podMap := make(map[string][]corev1.Pod)
	for _, pod := range collection.Items {
		for _, ownr := range pod.OwnerReferences {
			if ownr.Name == job.Name && pod.Status.StartTime != nil {
				podMap[pod.Name] = append(podMap[pod.Name], pod)
			}
		}
	}

	for _, pods := range podMap {
		slices.SortFunc(pods,
			func(a, b corev1.Pod) int {
				if a.ResourceVersion < b.ResourceVersion {
					return -1
				}
				if a.ResourceVersion > b.ResourceVersion {
					return 1
				}
				return 0
			})
		latestVersion := pods[len(pods)-1]
		podItems = append(podItems, latestVersion)
	}

	return &corev1.PodList{Items: podItems}, nil
}

func K8sSerialize(obj runtime.Object, buf *bytes.Buffer) error {
	if err := k8sSerializer.Encode(obj, buf); err != nil {
		return fmt.Errorf("k8sSerializer.Encode: %w", err)
	}
	return nil
}

func K8sDeserialize(data []byte, obj runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	o, gvk, err := k8sSerializer.Decode(data, nil, obj)
	if err != nil {
		return nil, nil, fmt.Errorf("k8sSerializer.Decode: %w", err)
	}
	return o, gvk, nil
}
