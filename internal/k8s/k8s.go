// Package k8s provides the Kubernetes client implementation and interface
// for interacting with the Kubernetes API to retrieve cronjob, job and pod data.
package k8s

import (
	"context"
	"errors"
	"fmt"

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

type ClientInterface interface {
	GetCronjob(cronjobNamespace, cronjobName string) *batchv1.CronJob
	WatchCronjobs() watch.Interface
	WatchJobs() watch.Interface
	WatchPods() watch.Interface
	GetPod(jobNamespace, podName string) *corev1.Pod
	GetJob(jobNamespace, jobName string) *batchv1.Job
	GetAllJobs() *batchv1.JobList
	Namespace() string
}

type Client struct {
	kubernetes.Interface
	l         zerolog.Logger
	namespace string
}

var _ ClientInterface = (*Client)(nil)

// A ClientOption is used to configure a Client.
type ClientOption func(*Client)

func WithNamespace(namespace string) ClientOption {
	return func(kc *Client) {
		kc.namespace = namespace
	}
}

func WithLogger(l zerolog.Logger) ClientOption {
	return func(kc *Client) {
		kc.l = l
	}
}

func NewClient(options ...ClientOption) *Client {
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

	kc := &Client{
		Interface: clientset,
	}

	for _, optionFn := range options {
		optionFn(kc)
	}

	return kc
}

func (kc *Client) Namespace() string {
	return kc.namespace
}

func (kc *Client) GetCronjob(cronjobNamespace, cronjobName string) *batchv1.CronJob {
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

func (kc *Client) WatchCronjobs() watch.Interface {
	ctx := context.Background()

	watcher, err := kc.BatchV1().CronJobs(kc.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	return watcher
}

func (kc *Client) WatchJobs() watch.Interface {
	ctx := context.Background()

	watcher, err := kc.BatchV1().Jobs(kc.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	return watcher
}

func (kc *Client) WatchPods() watch.Interface {
	ctx := context.Background()

	watcher, err := kc.CoreV1().Pods(kc.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	return watcher
}

func (kc *Client) GetPod(jobNamespace, podName string) *corev1.Pod {
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

func (kc *Client) GetJob(jobNamespace, jobName string) *batchv1.Job {
	ctx := context.TODO()
	job, err := kc.BatchV1().Jobs(jobNamespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}

	return job
}

func (kc *Client) GetAllJobs() *batchv1.JobList {
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
