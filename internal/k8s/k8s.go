// Package k8s provides the Kubernetes client implementation and interface
// for interacting with the Kubernetes API to retrieve cronjob, job and pod data.
package k8s

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ClientInterface interface {
	GetCronjob(ctx context.Context, cronjobNamespace, cronjobName string) (*batchv1.CronJob, error)
	WatchCronjobs(ctx context.Context) (watch.Interface, error)
	WatchJobs(ctx context.Context) (watch.Interface, error)
	WatchPods(ctx context.Context) (watch.Interface, error)
	GetPod(ctx context.Context, jobNamespace, podName string) (*corev1.Pod, error)
	GetJob(ctx context.Context, jobNamespace, jobName string) (*batchv1.Job, error)
	GetAllJobs(ctx context.Context) (*batchv1.JobList, error)
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

func NewClient(options ...ClientOption) (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config failed: %w", err)
	}
	config.ContentConfig = rest.ContentConfig{
		AcceptContentTypes: "application/vnd.kubernetes.protobuf,application/json",
		ContentType:        "application/vnd.kubernetes.protobuf",
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("kubernetes.NewForConfig failed: %w", err)
	}

	kc := &Client{
		Interface: clientset,
	}

	for _, optionFn := range options {
		optionFn(kc)
	}

	return kc, nil
}

func NewClientWithInterface(iface kubernetes.Interface, options ...ClientOption) *Client {
	kc := &Client{
		Interface: iface,
	}
	for _, optionFn := range options {
		optionFn(kc)
	}
	return kc
}

func (kc *Client) Namespace() string {
	return kc.namespace
}

func (kc *Client) GetCronjob(ctx context.Context, cronjobNamespace, cronjobName string) (*batchv1.CronJob, error) {
	cronJob, err := kc.BatchV1().CronJobs(cronjobNamespace).Get(ctx, cronjobName, metav1.GetOptions{})
	if err != nil {
		kc.l.Error().
			Err(err).
			Str("operation", "GetCronjob").
			Msg(fmt.Sprintf("failed to get CronJob %s in namespace %s", cronjobName, cronjobNamespace))
		return nil, fmt.Errorf("failed to get CronJob %s in namespace %s: %w", cronjobName, cronjobNamespace, err)
	}

	kc.l.Info().
		Str("component", "k8s").
		Str("operation", "GetCronjob").
		Msg(fmt.Sprintf("CronJob %s found in %s namespace", cronjobName, cronjobNamespace))

	return cronJob, nil
}

func (kc *Client) WatchCronjobs(ctx context.Context) (watch.Interface, error) {
	watcher, err := kc.BatchV1().CronJobs(kc.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		kc.l.Error().
			Err(err).
			Str("operation", "WatchCronjobs").
			Msg("failed to start watching CronJobs")
		return nil, fmt.Errorf("failed to start watching CronJobs: %w", err)
	}

	return watcher, nil
}

func (kc *Client) WatchJobs(ctx context.Context) (watch.Interface, error) {
	watcher, err := kc.BatchV1().Jobs(kc.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		kc.l.Error().
			Err(err).
			Str("operation", "WatchJobs").
			Msg("failed to start watching Jobs")
		return nil, fmt.Errorf("failed to start watching Jobs: %w", err)
	}

	return watcher, nil
}

func (kc *Client) WatchPods(ctx context.Context) (watch.Interface, error) {
	watcher, err := kc.CoreV1().Pods(kc.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		kc.l.Error().
			Err(err).
			Str("operation", "WatchPods").
			Msg("failed to start watching Pods")
		return nil, fmt.Errorf("failed to start watching Pods: %w", err)
	}

	return watcher, nil
}

func (kc *Client) GetPod(ctx context.Context, jobNamespace, podName string) (*corev1.Pod, error) {
	pod, err := kc.CoreV1().Pods(jobNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		kc.l.Error().
			Err(err).
			Str("operation", "GetPod").
			Msg(fmt.Sprintf("failed to get Pod %s in namespace %s", podName, jobNamespace))
		return nil, fmt.Errorf("failed to get Pod %s in namespace %s: %w", podName, jobNamespace, err)
	}

	kc.l.Info().
		Str("operation", "GetPod").
		Msg(fmt.Sprintf("Pod %s found in %s namespace", podName, jobNamespace))

	return pod, nil
}

func (kc *Client) GetJob(ctx context.Context, jobNamespace, jobName string) (*batchv1.Job, error) {
	job, err := kc.BatchV1().Jobs(jobNamespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		kc.l.Error().
			Err(err).
			Str("operation", "GetJob").
			Msg(fmt.Sprintf("failed to get Job %s in namespace %s", jobName, jobNamespace))
		return nil, fmt.Errorf("failed to get Job %s in namespace %s: %w", jobName, jobNamespace, err)
	}

	return job, nil
}

func (kc *Client) GetAllJobs(ctx context.Context) (*batchv1.JobList, error) {
	jobs, err := kc.BatchV1().Jobs(kc.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		kc.l.Error().
			Err(err).
			Str("operation", "GetAllJobs").
			Msg("failed to list Jobs")
		return nil, fmt.Errorf("failed to list Jobs: %w", err)
	}

	kc.l.Info().
		Str("operation", "GetAllJobs").
		Msg(fmt.Sprintf("There are %d jobs in the cluster", len(jobs.Items)))

	return jobs, nil
}
