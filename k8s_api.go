// Package main provides the Kubernetes client implementation and interface
// for interacting with the Kubernetes API to retrieve cronjob, job and pod data.
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/danroux/sk8l/protos"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Sk8lK8sClientInterface interface {
	kubernetes.Interface
	GetCronjob(cronjobNamespace, cronjobName string) *batchv1.CronJob
	WatchCronjobs() watch.Interface
	WatchJobs() watch.Interface
	WatchPods() watch.Interface
	GetPod(jobNamespace, podName string) *corev1.Pod
	GetJob(jobNamespace, jobName string) *batchv1.Job
	GetAllJobs() *batchv1.JobList
	Namespace() string
}

type K8sClient struct {
	kubernetes.Interface
	l         zerolog.Logger
	namespace string
}

var _ Sk8lK8sClientInterface = (*K8sClient)(nil)

// A ClientOption is used to configure a Client.
type ClientOption func(*K8sClient)

func WithNamespace(namespace string) ClientOption {
	return func(kc *K8sClient) {
		kc.namespace = namespace
	}
}

func WithLogger(l zerolog.Logger) ClientOption {
	return func(kc *K8sClient) {
		kc.l = l
	}
}

func NewK8sClient(options ...ClientOption) *K8sClient {
	config, err := rest.InClusterConfig()
	config.ContentConfig = rest.ContentConfig{
		AcceptContentTypes: "application/vnd.kubernetes.protobuf,application/json",
		ContentType:        "application/vnd.kubernetes.protobuf",
	}

	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	k8sClient := &K8sClient{
		Interface: clientset,
	}

	for _, optionFn := range options {
		optionFn(k8sClient)
	}

	return k8sClient
}

func (kc *K8sClient) Namespace() string {
	return kc.namespace
}

func (kc *K8sClient) GetCronjob(cronjobNamespace, cronjobName string) *batchv1.CronJob {
	ctx := context.TODO()
	cronJob, err := kc.BatchV1().CronJobs(cronjobNamespace).Get(ctx, cronjobName, metav1.GetOptions{})

	var statusError *k8serrors.StatusError
	switch {
	case k8serrors.IsNotFound(err):
		kc.l.Error().
			Err(err).
			Str("operation", "GetCronjob").
			Msg(fmt.Sprintf("Cronjob %s not found in default namespace", cronjobName))
	case errors.As(err, &statusError):
		kc.l.Error().
			Err(err).
			Str("operation", "GetCronjob").
			Msg(fmt.Sprintf("Error getting CronJob %v", statusError.ErrStatus.Message))
	case err != nil:
		panic(err.Error())
	default:
		kc.l.Info().
			Str("component", "k8s").
			Str("operation", "GetCronjob").
			Msg(fmt.Sprintf("CronJob %s found in %s namespace", cronjobName, cronjobNamespace))
	}

	return cronJob
}

func (kc *K8sClient) WatchCronjobs() watch.Interface {
	ctx := context.Background()

	watcher, err := kc.BatchV1().CronJobs(kc.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	return watcher
}

func (kc *K8sClient) WatchJobs() watch.Interface {
	ctx := context.Background()

	watcher, err := kc.BatchV1().Jobs(kc.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	return watcher
}

func (kc *K8sClient) WatchPods() watch.Interface {
	ctx := context.Background()

	watcher, _ := kc.CoreV1().Pods(kc.namespace).Watch(ctx, metav1.ListOptions{})

	return watcher
}

func (kc *K8sClient) GetPod(jobNamespace, podName string) *corev1.Pod {
	ctx := context.TODO()
	pod, err := kc.CoreV1().Pods(jobNamespace).Get(ctx, podName, metav1.GetOptions{})

	var statusError *k8serrors.StatusError
	switch {
	case k8serrors.IsNotFound(err):
		kc.l.Error().
			Err(err).
			Str("operation", "GetPod").
			Msg(fmt.Sprintf("Pod %s not found in default namespace", podName))
	case errors.As(err, &statusError):
		log.Printf("Error getting Pod %v\n", statusError.ErrStatus.Message)
		kc.l.Error().
			Err(err).
			Str("operation", "GetPod").
			Msg(fmt.Sprintf("Error getting Pod %v", statusError.ErrStatus.Message))
	case err != nil:
		panic(err.Error())
	default:
		kc.l.Info().
			Str("operation", "GetPod").
			Msg(fmt.Sprintf("Pod %s found in %s namespace", jobNamespace, podName))
	}

	return pod
}

func (kc *K8sClient) GetJob(jobNamespace, jobName string) *batchv1.Job {
	ctx := context.TODO()
	job, err := kc.BatchV1().Jobs(jobNamespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}

	return job
}

func (kc *K8sClient) GetAllJobs() *batchv1.JobList {
	ctx := context.TODO()

	// get pods in all the namespaces by omitting namespace
	// Or specify namespace to get pods in particular namespace
	// Limit: 10, // need to fix this - last duration / current duration get messed up
	jobs, err := kc.BatchV1().Jobs(kc.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	kc.l.Info().
		Str("operation", "GetAllJobs").
		Msg(fmt.Sprintf("There are %d jobs in the cluster", len(jobs.Items)))

	return jobs
}

// Deprecated: GetAllJobsMapped is no longer used. Use GetAllJobs via findJobsMapped instead.
func (kc *K8sClient) GetAllJobsMapped() *protos.MappedJobs {
	jobs := kc.GetAllJobs()

	cronjobNames := []string{}
	jobsMapped := make(map[string][]*batchv1.Job)
	for i := range jobs.Items {
		job := &jobs.Items[i]
		for _, owr := range job.ObjectMeta.OwnerReferences {
			jobsMapped[owr.Name] = append(jobsMapped[owr.Name], job)
			found := false
			for _, n := range cronjobNames {
				if n == owr.Name {
					found = true
					break
				}
			}
			if !found {
				cronjobNames = append(cronjobNames, owr.Name)
			}
		}
	}

	x := &protos.MappedJobs{}
	x.JobLists = make(map[string]*protos.JobList)
	for _, name := range cronjobNames {
		items := make([]*protos.JobResponse, 0, len(jobsMapped[name]))
		for _, j := range jobsMapped[name] {
			items = append(items, &protos.JobResponse{
				Name:      j.Name,
				Namespace: j.Namespace,
				Uuid:      string(j.UID),
			})
		}
		x.JobLists[name] = &protos.JobList{
			Items: items,
		}
	}

	return x
}
