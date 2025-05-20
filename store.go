package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
	batchv1 "k8s.io/api/batch/v1"
)

type CronJobStore interface {
	Sk8lK8sClientInterface
}

type CronJobDBStore struct {
	// K8sClient *K8sClient
	K8sClient Sk8lK8sClientInterface
	*badger.DB
}

func (c *CronJobDBStore) getAndStore(key []byte, apiCall APICall) ([]byte, error) {
	var valueResponse []byte
	err := c.DB.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(key)

		if errors.Is(err, badger.ErrKeyNotFound) {
			err = c.DB.Update(func(txn *badger.Txn) error {
				apiResult := apiCall()
				entry := badger.NewEntry(key, apiResult).WithTTL(time.Second * badgerTTL)
				err = txn.SetEntry(entry)
				if err != nil {
					log.Println("Error: getAndStore#txn.SetEntry", err)
					return fmt.Errorf("sk8l#getAndStore: txn.SetEntry() failed: %w", err)
				}
				valueResponse = append([]byte{}, apiResult...)
				return nil
			})
		} else {
			err = item.Value(func(val []byte) error {
				valueResponse = append([]byte{}, val...)

				return nil
			})
		}

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

func (c *CronJobDBStore) get(key []byte) ([]byte, error) {
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
			log.Println("Error: get#current.Value", err)
			return fmt.Errorf("sk8l#get: current.Value() failed: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("sk8l#get: DB.Update() failed: %w", err)
	}

	return valueResponse, nil
}

func (c *CronJobDBStore) findCronjobs() *batchv1.CronJobList {
	cronjobs, err := c.get(cronjobsCacheKey)

	if err != nil {
		if !errors.Is(err, badger.ErrKeyNotFound) {
			log.Println("Error: findCronjobs#s.get", err)
		}
	}

	cronjobList := &batchv1.CronJobList{}
	cronjobListV2 := protoadapt.MessageV2Of(cronjobList)

	err = proto.Unmarshal(cronjobs, cronjobListV2)
	if err != nil {
		log.Println("Error: findCronjobs#proto.Unmarshal", err)
	}

	return cronjobList
}

func (c *CronJobDBStore) findCronjob(cronjobNamespace, cronjobName string) *batchv1.CronJob {
	gCjCall := func() []byte {
		cronjobName := cronjobName
		cronjobNamespace := cronjobNamespace
		cronjob := c.K8sClient.GetCronjob(cronjobNamespace, cronjobName)
		cronjobV2 := protoadapt.MessageV2Of(cronjob)
		cronjobValue, _ := proto.Marshal(cronjobV2)
		return cronjobValue
	}

	cacheKey := fmt.Sprintf(cronjobsKeyFmt, cronjobNamespace, cronjobName)
	key := []byte(cacheKey)
	cronjobValue, err := c.getAndStore(key, gCjCall)

	if err != nil {
		log.Println("Error: findCronjob#s.getAndStore", err)
	}

	cronjob := &batchv1.CronJob{}
	cronjobV2 := protoadapt.MessageV2Of(cronjob)
	err = proto.Unmarshal(cronjobValue, cronjobV2)

	if err != nil {
		log.Println("Error: findCronjob#proton.Unmarshal", err)
	}

	return cronjob
}

func (c *CronJobDBStore) findJobs() *batchv1.JobList {
	jobs, err := c.get(jobsCacheKey)

	if err != nil {
		if !errors.Is(err, badger.ErrKeyNotFound) {
			log.Println("Error: findCronjobs#s.get", err)
		}
	}

	jobList := &batchv1.JobList{}
	jobListV2 := protoadapt.MessageV2Of(jobList)

	err = proto.Unmarshal(jobs, jobListV2)
	if err != nil {
		log.Println("Error: findJobs#proto.Unmarshal", err)
	}

	// filter out jobs that belong to cronjobs
	jobTasks := make([]batchv1.Job, 0, len(jobList.Items))
	for _, job := range jobList.Items {
		if job.OwnerReferences == nil {
			jobTasks = append(jobTasks, job)
		}
	}

	jobTaskList := &batchv1.JobList{
		Items: jobTasks,
	}

	return jobTaskList
}
