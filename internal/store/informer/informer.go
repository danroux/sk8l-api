// Package informer implements the store.Store interface using client-go informers,
// listers, and in-memory indexers.
package informer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/danroux/sk8l/internal/store"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	batchlisters "k8s.io/client-go/listers/batch/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	jobNameIndex       = "byJobName"
	noResyncPeriod     = time.Duration(0)
	notifyChannelCap   = 1
	defaultPodSelector = "job-name"
)

var (
	ErrClientRequired = errors.New("informer: kubernetes client interface is required")

	_ store.Store   = (*Store)(nil)
	_ store.Starter = (*Store)(nil)
)

// Option configures the Informer Store.
type Option func(*Store)

// WithResyncPeriod configures a custom resync period for informers (default is 0/disabled).
func WithResyncPeriod(d time.Duration) Option {
	return func(s *Store) {
		s.resyncPeriod = d
	}
}

// WithPodLabelSelector configures a custom label selector for pods (default is "job-name").
func WithPodLabelSelector(selector string) Option {
	return func(s *Store) {
		s.podLabelSelector = selector
	}
}

// Store is an in-memory store backed by client-go informers.
type Store struct {
	client           kubernetes.Interface
	factory          informers.SharedInformerFactory
	podFactory       informers.SharedInformerFactory
	cronJobInformer  cache.SharedIndexInformer
	jobInformer      cache.SharedIndexInformer
	podInformer      cache.SharedIndexInformer
	cronJobLister    batchlisters.CronJobLister
	jobLister        batchlisters.JobLister
	podLister        corelisters.PodLister
	podIndexer       cache.Indexer
	notifier         *notifier
	namespace        string
	podLabelSelector string
	resyncPeriod     time.Duration
}

// New creates and configures a new informer-backed Store.
func New(client kubernetes.Interface, namespace string, opts ...Option) (*Store, error) {
	if client == nil {
		return nil, ErrClientRequired
	}

	s := &Store{
		client:           client,
		namespace:        namespace,
		resyncPeriod:     noResyncPeriod,
		podLabelSelector: defaultPodSelector,
		notifier:         newNotifier(),
	}

	for _, opt := range opts {
		opt(s)
	}

	var factoryOpts []informers.SharedInformerOption
	if s.namespace != "" {
		factoryOpts = append(factoryOpts, informers.WithNamespace(s.namespace))
	}

	s.factory = informers.NewSharedInformerFactoryWithOptions(
		s.client,
		s.resyncPeriod,
		factoryOpts...,
	)

	podFactoryOpts := append([]informers.SharedInformerOption(nil), factoryOpts...)
	if s.podLabelSelector != "" {
		podFactoryOpts = append(podFactoryOpts, informers.WithTweakListOptions(func(lo *metav1.ListOptions) {
			lo.LabelSelector = s.podLabelSelector
		}))
	}

	s.podFactory = informers.NewSharedInformerFactoryWithOptions(
		s.client,
		s.resyncPeriod,
		podFactoryOpts...,
	)

	s.cronJobInformer = s.factory.Batch().V1().CronJobs().Informer()
	s.jobInformer = s.factory.Batch().V1().Jobs().Informer()
	s.podInformer = s.podFactory.Core().V1().Pods().Informer()

	s.cronJobLister = s.factory.Batch().V1().CronJobs().Lister()
	s.jobLister = s.factory.Batch().V1().Jobs().Lister()
	s.podLister = s.podFactory.Core().V1().Pods().Lister()
	s.podIndexer = s.podInformer.GetIndexer()

	// Strip managedFields to drastically reduce in-memory footprint
	stripManagedFields := func(obj any) (any, error) {
		if accessor, err := meta.Accessor(obj); err == nil {
			accessor.SetManagedFields(nil)
		}
		return obj, nil
	}

	if err := s.cronJobInformer.SetTransform(stripManagedFields); err != nil {
		return nil, fmt.Errorf("informer: set CronJob transform failed: %w", err)
	}
	if err := s.jobInformer.SetTransform(stripManagedFields); err != nil {
		return nil, fmt.Errorf("informer: set Job transform failed: %w", err)
	}
	if err := s.podInformer.SetTransform(stripManagedFields); err != nil {
		return nil, fmt.Errorf("informer: set Pod transform failed: %w", err)
	}

	// Register custom indexer for pods by job-name
	err := s.podIndexer.AddIndexers(cache.Indexers{
		jobNameIndex: jobNameIndexFunc,
	})
	if err != nil {
		return nil, fmt.Errorf("informer: add Pod indexer failed: %w", err)
	}

	// Event handlers for change notification
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ any) {
			s.notifier.notify()
		},
		UpdateFunc: func(oldObj, newObj any) {
			oldMeta, err1 := meta.Accessor(oldObj)
			newMeta, err2 := meta.Accessor(newObj)
			if err1 == nil && err2 == nil && oldMeta.GetResourceVersion() == newMeta.GetResourceVersion() {
				return
			}
			s.notifier.notify()
		},
		DeleteFunc: func(_ any) {
			s.notifier.notify()
		},
	}

	if _, err := s.cronJobInformer.AddEventHandler(handlers); err != nil {
		return nil, fmt.Errorf("informer: add CronJob event handler failed: %w", err)
	}
	if _, err := s.jobInformer.AddEventHandler(handlers); err != nil {
		return nil, fmt.Errorf("informer: add Job event handler failed: %w", err)
	}
	if _, err := s.podInformer.AddEventHandler(handlers); err != nil {
		return nil, fmt.Errorf("informer: add Pod event handler failed: %w", err)
	}

	return s, nil
}

// Start begins background execution of the informers.
func (s *Store) Start(ctx context.Context) {
	s.factory.Start(ctx.Done())
	if s.podFactory != nil {
		s.podFactory.Start(ctx.Done())
	}
}

// WaitForSync blocks until all informers have synced initial caches or ctx is canceled.
func (s *Store) WaitForSync(ctx context.Context) bool {
	return cache.WaitForNamedCacheSync("sk8l-informers", ctx.Done(),
		s.cronJobInformer.HasSynced,
		s.jobInformer.HasSynced,
		s.podInformer.HasSynced,
	)
}

// Ready reports whether all underlying informers have completed initial cache synchronization.
func (s *Store) Ready() bool {
	return s.cronJobInformer.HasSynced() &&
		s.jobInformer.HasSynced() &&
		s.podInformer.HasSynced()
}

// Subscribe returns a coalesced notification channel (cap 1) receiving a signal on any relevant change.
func (s *Store) Subscribe(ctx context.Context) <-chan struct{} {
	return s.notifier.subscribe(ctx)
}

// FindCronjobs returns all cached CronJobs in the configured namespace.
func (s *Store) FindCronjobs() (*batchv1.CronJobList, error) {
	var (
		cronjobs []*batchv1.CronJob
		err      error
	)
	if s.namespace != "" {
		cronjobs, err = s.cronJobLister.CronJobs(s.namespace).List(labels.Everything())
	} else {
		cronjobs, err = s.cronJobLister.List(labels.Everything())
	}
	if err != nil {
		return nil, fmt.Errorf("informer#FindCronjobs: %w", err)
	}

	items := make([]batchv1.CronJob, len(cronjobs))
	for i, cj := range cronjobs {
		items[i] = *cj.DeepCopy()
	}
	return &batchv1.CronJobList{Items: items}, nil
}

// FindCronjob returns a specific cached CronJob by name.
func (s *Store) FindCronjob(ctx context.Context, cronjobNamespace, cronjobName string) (*batchv1.CronJob, error) {
	ns := cronjobNamespace
	if ns == "" {
		ns = s.namespace
	}
	cj, err := s.cronJobLister.CronJobs(ns).Get(cronjobName)
	if err != nil {
		return nil, fmt.Errorf("informer#FindCronjob: %w", err)
	}
	return cj.DeepCopy(), nil
}

// FindJobs returns all cached Jobs in the configured namespace.
func (s *Store) FindJobs() (*batchv1.JobList, error) {
	var (
		jobs []*batchv1.Job
		err  error
	)
	if s.namespace != "" {
		jobs, err = s.jobLister.Jobs(s.namespace).List(labels.Everything())
	} else {
		jobs, err = s.jobLister.List(labels.Everything())
	}
	if err != nil {
		return nil, fmt.Errorf("informer#FindJobs: %w", err)
	}

	items := make([]batchv1.Job, len(jobs))
	for i, j := range jobs {
		items[i] = *j.DeepCopy()
	}
	return &batchv1.JobList{Items: items}, nil
}

// FindJobsMapped returns a mapping from CronJob name to its slice of child Jobs.
func (s *Store) FindJobsMapped(ctx context.Context) (map[string][]*batchv1.Job, error) {
	var (
		jobs []*batchv1.Job
		err  error
	)
	if s.namespace != "" {
		jobs, err = s.jobLister.Jobs(s.namespace).List(labels.Everything())
	} else {
		jobs, err = s.jobLister.List(labels.Everything())
	}
	if err != nil {
		return nil, fmt.Errorf("informer#FindJobsMapped: %w", err)
	}

	mapped := make(map[string][]*batchv1.Job)
	for _, j := range jobs {
		for _, owr := range j.OwnerReferences {
			mapped[owr.Name] = append(mapped[owr.Name], j.DeepCopy())
		}
	}
	return mapped, nil
}

// FindJobPodsForJob returns all pods associated with the given Job.
func (s *Store) FindJobPodsForJob(job *batchv1.Job) (*corev1.PodList, error) {
	if job == nil {
		return &corev1.PodList{}, nil
	}

	objs, err := s.podIndexer.ByIndex(jobNameIndex, job.Name)
	if err != nil {
		return nil, fmt.Errorf("informer#FindJobPodsForJob: %w", err)
	}

	items := make([]corev1.Pod, 0, len(objs))
	for _, obj := range objs {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			continue
		}
		if pod.Status.StartTime != nil {
			items = append(items, *pod.DeepCopy())
		}
	}
	return &corev1.PodList{Items: items}, nil
}

// jobNameIndexFunc indexes pods by their associated job name.
func jobNameIndexFunc(obj any) ([]string, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, nil
	}

	// Primary: label job-name set by the Kubernetes Job controller
	if jn, ok := pod.Labels["job-name"]; ok && jn != "" {
		return []string{jn}, nil
	}

	// Fallback: check controller owner reference
	for _, owr := range pod.OwnerReferences {
		if owr.Kind == "Job" || (owr.Controller != nil && *owr.Controller) {
			return []string{owr.Name}, nil
		}
	}

	return nil, nil
}

// notifier broadcasts change notifications to concurrent subscribers with cap-1 coalescing.
type notifier struct {
	subscribers map[chan struct{}]struct{}
	mu          sync.RWMutex
}

func newNotifier() *notifier {
	return &notifier{
		subscribers: make(map[chan struct{}]struct{}),
	}
}

func (n *notifier) subscribe(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{}, notifyChannelCap)
	n.mu.Lock()
	n.subscribers[ch] = struct{}{}
	n.mu.Unlock()

	go func() {
		<-ctx.Done()
		n.mu.Lock()
		delete(n.subscribers, ch)
		n.mu.Unlock()
		close(ch)
	}()

	return ch
}

func (n *notifier) notify() {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for ch := range n.subscribers {
		select {
		case ch <- struct{}{}:
		default:
			// Coalesce: subscriber already has an unread notification signal
		}
	}
}
