// Package store provides the Badger-backed cache layer for CronJob, Job and Pod data.
package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/danroux/sk8l/internal/k8s"
	"github.com/danroux/sk8l/internal/logger"
	badger "github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	batchv1 "k8s.io/api/batch/v1"
	k8sproto "k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
	"k8s.io/client-go/kubernetes/scheme"
)

var k8sSerializer = k8sproto.NewSerializer(scheme.Scheme, scheme.Scheme)

type APICall func() []byte

type CronJobDBStore struct {
	K8sClient k8s.ClientInterface
	*badger.DB
	l zerolog.Logger
}

type CronJobDBStoreOptionFn func(*CronJobDBStore)

func NewCronJobDBStore(optsFn ...CronJobDBStoreOptionFn) *CronJobDBStore {
	cjdbs := &CronJobDBStore{
		l: log.With().Str("component", "db_store").Logger(),
	}

	for _, opt := range optsFn {
		opt(cjdbs)
	}

	if cjdbs.K8sClient == nil {
		cjdbs.l.Fatal().Msg("NewCronJobDBStore: K8sClient must be provided.")
	}

	if cjdbs.DB == nil {
		cjdbs.l.Info().Msg("NewCronJobDBStore: DB not provided, setting default one.")
		dbFn := WithDefaultDB()
		dbFn(cjdbs)
	}

	return cjdbs
}

func WithDB(db *badger.DB) CronJobDBStoreOptionFn {
	return func(cjdbs *CronJobDBStore) { cjdbs.DB = db }
}

func WithK8sClient(k8sClient *k8s.Client) CronJobDBStoreOptionFn {
	return func(cjdbs *CronJobDBStore) {
		cjdbs.K8sClient = k8sClient
	}
}

func WithDefaultK8sClient(k8sNamespace string) CronJobDBStoreOptionFn {
	k8sClient := k8s.NewClient(
		k8s.WithNamespace(k8sNamespace),
		k8s.WithLogger(log.With().Str("component", "k8s").Logger()),
	)

	return WithK8sClient(k8sClient)
}

func WithDefaultDB(badgerTTL time.Duration) CronJobDBStoreOptionFn {
	badgerLogger := logger.NewBadgerLogger(zerolog.GlobalLevel())
	badgerOpts := badger.DefaultOptions("/tmp/badger").WithLogger(badgerLogger)
	db, err := badger.Open(badgerOpts)
	if err != nil {
		badgerLogger.Fatal().Err(err).Msg("failed to open Badger DB")
	}

	return WithDB(db)
}

func (c *CronJobDBStore) GetAndStore(key []byte, apiCall APICall, badgerTTL time.Duration) ([]byte, error) {
	var valueResponse []byte
	err := c.DB.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			apiResult := apiCall()
			entry := badger.NewEntry(key, apiResult).WithTTL(time.Second * badgerTTL)
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

func (c *CronJobDBStore) FindCronjobs(cronjobsCacheKey []byte) (*batchv1.CronJobList, error) {
	cronjobs, err := c.Get(cronjobsCacheKey)
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

func (c *CronJobDBStore) FindCronjob(ctx context.Context, cronjobsKeyFmt, cronjobNamespace, cronjobName string, badgerTTL time.Duration) (*batchv1.CronJob, error) {
	gCjCall := func() []byte {
		cronjob := c.K8sClient.GetCronjob(ctx, cronjobNamespace, cronjobName)
		var buf bytes.Buffer
		if err := k8sSerializer.Encode(cronjob, &buf); err != nil {
			c.l.Error().Err(err).Msg("findCronjob#k8sSerializer.Encode")
			return nil
		}
		return buf.Bytes()
	}

	cacheKey := []byte(fmt.Sprintf(cronjobsKeyFmt, cronjobNamespace, cronjobName))
	cronjobValue, err := c.GetAndStore(cacheKey, gCjCall, badgerTTL)
	if err != nil {
		return nil, fmt.Errorf("findCronjob#getAndStore: %w", err)
	}

	cronjob := &batchv1.CronJob{}
	if _, _, err = k8sSerializer.Decode(cronjobValue, nil, cronjob); err != nil {
		return nil, fmt.Errorf("findCronjob#Decode: %w", err)
	}
	return cronjob, nil
}

func (c *CronJobDBStore) FindJobs(jobsCacheKey []byte) (*batchv1.JobList, error) {
	jobs, err := c.Get(jobsCacheKey)
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
