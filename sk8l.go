package main

import (
	"bytes"
	"cmp"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"

	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/danroux/sk8l/internal/dashboard"
	"github.com/danroux/sk8l/protos"
	badger "github.com/dgraph-io/badger/v4"
	"google.golang.org/grpc/health/grpc_health_v1"
	gyaml "sigs.k8s.io/yaml"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sproto "k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
)

//go:embed annotations.tmpl
var content embed.FS

const (
	jobPodsKeyFmt    = "jobs_pods_for_job_%s"
	cronjobsKeyFmt   = "sk8l_cronjob_%s_%s"
	badgerTTLSeconds = 15
	refreshSeconds   = 10
)

var (
	cronjobsCacheKey   = []byte("sk8l_cronjobs")
	jobsMappedCacheKey = []byte("sk8l_jobs_mapped")
	jobsCacheKey       = []byte("sk8l_jobs")
	badgerTTL          = time.Duration(badgerTTLSeconds)
	refreshInterval    = time.Second * refreshSeconds
	k8sSerializer      = k8sproto.NewSerializer(scheme.Scheme, scheme.Scheme)
)

type Sk8lServer struct {
	grpc_health_v1.UnimplementedHealthServer
	protos.UnimplementedCronjobServer
	*CronJobDBStore
	dashboardGen    *dashboard.Generator
	metricsNamesMap *sync.Map
	target          string
	dialOptions     []grpc.DialOption
}

type APICall (func() []byte)

func NewSk8lServer(
	target string,
	cronJobDBStore *CronJobDBStore,
	dashboardGen *dashboard.Generator,
	metricsNamesMap *sync.Map,
	dialOptions ...grpc.DialOption,
) *Sk8lServer {
	return &Sk8lServer{
		target:          target,
		CronJobDBStore:  cronJobDBStore,
		dashboardGen:    dashboardGen,
		metricsNamesMap: metricsNamesMap,
		dialOptions:     dialOptions,
	}
}

func (s *Sk8lServer) GetTarget() string {
	return s.target
}

func (s *Sk8lServer) GetDialOptions() []grpc.DialOption {
	return s.dialOptions
}

func (s Sk8lServer) Check(
	ctx context.Context,
	req *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	log.Info().
		Str("component", "probe").
		Str("operation", "health").
		Msg("serving health")
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (s Sk8lServer) Watch(req *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error {
	response := &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}

	if err := stream.Send(response); err != nil {
		log.Error().
			Err(err).
			Str("operation", "Watch#stream.Send").
			Send()
		return fmt.Errorf("sk8l#Watch: stream.Send failed: %w", err)
	}

	return nil
}

func (s *Sk8lServer) Run(metricsCxt context.Context) {
	s.collectCronjobs()
	s.collectJobs()
	s.collectPods()
	recordMetrics(metricsCxt, s, s.metricsNamesMap)
}

func (s *Sk8lServer) GetCronjobs(in *protos.CronjobsRequest, stream protos.Cronjob_GetCronjobsServer) error {
	for {
		cronJobList := s.findCronjobs()
		jobsMapped := s.findJobsMapped()

		n := len(cronJobList.Items)
		cronjobs := make([]*protos.CronjobResponse, 0, n)

		wg := sync.WaitGroup{}
		wg.Add(n)
		for _, cronjobItem := range cronJobList.Items {
			go func(cronjobItem batchv1.CronJob) {
				defer wg.Done()
				jobsForCronjob := s.jobsForCronjob(jobsMapped, cronjobItem.Name)
				cronjob := s.cronJobResponse(cronjobItem, jobsForCronjob)
				cronjobs = append(cronjobs, cronjob)
			}(cronjobItem)
		}
		wg.Wait()

		slices.SortFunc(cronjobs,
			func(a, b *protos.CronjobResponse) int {
				return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
			})

		y := &protos.CronjobsResponse{
			Cronjobs: cronjobs,
		}

		select {
		case <-stream.Context().Done():
			err := stream.Context().Err()
			log.Error().
				Err(err).
				Str("operation", "GetCronJobs").
				Msg("stream context done: client canceled or deadline exceeded")
			return fmt.Errorf("sk8l#GetCronjobs: stream.Context().Done(): %w", err)
		default:
			if err := stream.Send(y); err != nil {
				return fmt.Errorf("sk8l#GetCronjobs: stream.Send() failed: %w", err)
			}
			time.Sleep(refreshInterval)
		}
	}
}

func (s *Sk8lServer) GetCronjob(in *protos.CronjobRequest, stream protos.Cronjob_GetCronjobServer) error {
	for {
		cronjob := s.findCronjob(in.CronjobNamespace, in.CronjobName)

		jobsMapped := s.findJobsMapped()
		jobsForCronjob := s.jobsForCronjob(jobsMapped, cronjob.Name)
		cronJobResponse := s.cronJobResponse(*cronjob, jobsForCronjob)
		if err := stream.Send(cronJobResponse); err != nil {
			return fmt.Errorf("sk8l#GetCronjob: stream.Send() failed: %w", err)
		}

		time.Sleep(refreshInterval)
	}
}

func (s *Sk8lServer) GetCronjobPods(in *protos.CronjobPodsRequest, stream protos.Cronjob_GetCronjobPodsServer) error {
	for {
		cronjob := s.findCronjob(in.CronjobNamespace, in.CronjobName)

		jobsMapped := s.findJobsMapped()
		jobs := s.jobsForCronjob(jobsMapped, cronjob.Name)

		cronjobResponse := s.cronJobResponse(*cronjob, jobs)
		lightweightCronjobPodsResponse := &protos.CronjobResponse{
			Name:      cronjob.Name,
			Namespace: cronjob.Namespace,
			Jobs:      cronjobResponse.Jobs,
		}

		slices.SortFunc(cronjobResponse.JobsPods,
			func(a, b *protos.PodResponse) int {
				return strings.Compare(a.Status.StartTime, b.Status.StartTime)
			})

		cronjobPodsResponse := &protos.CronjobPodsResponse{
			Pods:    cronjobResponse.JobsPods,
			Cronjob: lightweightCronjobPodsResponse,
		}

		if err := stream.Send(cronjobPodsResponse); err != nil {
			return fmt.Errorf("sk8l#GetCronjobPods: stream.Send() failed: %w", err)
		}

		time.Sleep(refreshInterval)
	}
}

func (s *Sk8lServer) GetJobs(in *protos.JobsRequest, stream protos.Cronjob_GetJobsServer) error {
	for {
		jobList := s.findJobs()

		n := len(jobList.Items)
		jobs := make([]*protos.JobResponse, 0, n)

		for i := range jobList.Items {
			job := s.buildJobResponse(&jobList.Items[i])
			jobs = append(jobs, job)
		}

		y := &protos.JobsResponse{
			Jobs: jobs,
		}

		if err := stream.Send(y); err != nil {
			return fmt.Errorf("sk8l#GetJobs: stream.Send() failed: %w", err)
		}

		time.Sleep(refreshInterval)
	}
}

func (s *Sk8lServer) GetCronjobYAML(
	ctx context.Context,
	in *protos.CronjobRequest,
) (*protos.CronjobYAMLResponse, error) {
	cronjob := s.K8sClient.GetCronjob(in.CronjobNamespace, in.CronjobName)
	prettyJSON, err := json.MarshalIndent(cronjob, "", "  ")

	if err != nil {
		log.Error().
			Err(err).
			Str("operation", "sk8l#GetCronjobYAML").
			Msg("json.MarshalIndent() failed")
	}

	y, _ := gyaml.JSONToYAML(prettyJSON)

	response := &protos.CronjobYAMLResponse{
		Cronjob: string(y),
	}

	return response, nil
}

func (s *Sk8lServer) GetJobYAML(ctx context.Context, in *protos.JobRequest) (*protos.JobYAMLResponse, error) {
	job := s.K8sClient.GetJob(in.JobNamespace, in.JobName)
	prettyJSON, err := json.MarshalIndent(job, "", "  ")

	if err != nil {
		log.Error().
			Err(err).
			Str("operation", "sk8l#GetJobYAML").
			Msg("json.MarshalIndent() failed")
	}

	y, _ := gyaml.JSONToYAML(prettyJSON)

	response := &protos.JobYAMLResponse{
		Job: string(y),
	}

	return response, nil
}

func (s *Sk8lServer) GetPodYAML(ctx context.Context, in *protos.PodRequest) (*protos.PodYAMLResponse, error) {
	pod := s.K8sClient.GetPod(in.PodNamespace, in.PodName)
	prettyJSON, err := json.MarshalIndent(pod, "", "  ")

	if err != nil {
		log.Error().
			Err(err).
			Str("operation", "sk8l#GetPodYAML").
			Msg("json.MarshalIndent() failed")
	}

	y, _ := gyaml.JSONToYAML(prettyJSON)

	response := &protos.PodYAMLResponse{
		Pod: string(y),
	}

	return response, nil
}

func (s *Sk8lServer) GetDashboardAnnotations(
	context.Context,
	*protos.DashboardAnnotationsRequest,
) (*protos.DashboardAnnotationsResponse, error) {
	panels := s.dashboardGen.GeneratePanels(s.metricsNamesMap)

	var tmplFile = "annotations.tmpl"
	t := template.New(tmplFile)
	t = t.Funcs(template.FuncMap{"marshal": func(v any) string {
		a, _ := json.Marshal(v)
		return string(a)
	}})
	t = template.Must(t.ParseFS(content, tmplFile))

	var b bytes.Buffer
	if err := t.Execute(&b, panels); err != nil {
		log.Error().
			Err(err).
			Str("operation", "sk8l#GetDashboardAnnotations").
			Msg("executing template")
	}

	return &protos.DashboardAnnotationsResponse{
		Annotations: b.String(),
	}, nil
}

func (s *Sk8lServer) findJobsMapped() map[string][]*batchv1.Job {
	jobs, err := s.getAndStore(jobsMappedCacheKey, func() []byte {
		jobList := s.K8sClient.GetAllJobs()
		var buf bytes.Buffer
		if err := k8sSerializer.Encode(jobList, &buf); err != nil {
			log.Error().
				Err(err).
				Str("operation", "findJobsMapped").
				Msg("k8sSerializer.Encode")
		}
		return buf.Bytes()
	})

	if err != nil {
		log.Error().
			Err(err).
			Str("operation", "findJobsMapped").
			Msg("getAndStore")
	}

	jobList := &batchv1.JobList{}
	if _, _, err := k8sSerializer.Decode(jobs, nil, jobList); err != nil {
		log.Error().
			Err(err).
			Str("operation", "findJobsMapped").
			Msg("k8sSerializer.Decode")
	}

	mapped := make(map[string][]*batchv1.Job)
	for i := range jobList.Items {
		job := &jobList.Items[i]
		for _, owr := range job.OwnerReferences {
			mapped[owr.Name] = append(mapped[owr.Name], job)
		}
	}
	return mapped
}

func (s *Sk8lServer) findJobPodsForJob(job *batchv1.Job) *corev1.PodList {
	fKey := fmt.Sprintf(jobPodsKeyFmt, job.Name)
	key := []byte(fKey)
	collection := &corev1.PodList{}
	err := s.DB.View(func(txn *badger.Txn) error {
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
		log.Error().
			Err(err).
			Str("operation", "findJobPodsForJob").
			Msg("DB.View")
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
				return cmp.Compare(a.ResourceVersion, b.ResourceVersion)
			})
		latestVersion := pods[len(pods)-1]
		podItems = append(podItems, latestVersion)
	}

	return &corev1.PodList{Items: podItems}
}

// Revisit this. JobConditions are not being used yet anywhere.
// PodResponse.TerminationReasons.TerminationDetails -> ContainerStateTerminated.
func jobFailed(
	job *batchv1.Job,
	jobPodsResponses []*protos.PodResponse,
) (bool, *protos.JobConditionResponse, []*protos.JobCondition) {
	var failed bool
	var failureCondition *protos.JobConditionResponse
	n := len(job.Status.Conditions)
	jobConditions := make([]*protos.JobCondition, 0, n)
	for i := range job.Status.Conditions {
		jobCondition := job.Status.Conditions[i]
		if !failed {
			if jobCondition.Type == batchv1.JobFailed {
				failed = true
				failureCondition = mapJobCondition(&jobCondition)
			}
		}
		jobConditions = append(jobConditions, &protos.JobCondition{
			Type:               string(jobCondition.Type),
			Status:             string(jobCondition.Status),
			LastProbeTime:      timeToString(&jobCondition.LastProbeTime),
			LastTransitionTime: timeToString(&jobCondition.LastTransitionTime),
			Reason:             jobCondition.Reason,
			Message:            jobCondition.Message,
		})
	}
	for _, pr := range jobPodsResponses {
		if pr.Failed {
			failed = true
		}
	}
	return failed, failureCondition, jobConditions
}

func (s *Sk8lServer) jobWithSidecarContainer(batchJob *batchv1.Job) bool {
	for _, container := range batchJob.Spec.Template.Spec.InitContainers {
		if container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			return true
		}
	}
	return false
}

func (s *Sk8lServer) buildJobResponse(batchJob *batchv1.Job) *protos.JobResponse {
	jobPodsForJob := s.findJobPodsForJob(batchJob)
	jobPodsResponses := buildJobPodsResponses(jobPodsForJob)
	jobFailed, failureCondition, jobConditions := jobFailed(batchJob, jobPodsResponses)
	duration := toDuration(batchJob, jobFailed, failureCondition)
	durationInS := toDurationInS(batchJob, jobFailed, failureCondition)
	completionTimeInS := toCompletionTimeInS(batchJob)
	terminationReasons := make([]*protos.TerminationReason, 0)
	for _, podResponse := range jobPodsResponses {
		terminationReasons = append(terminationReasons, podResponse.TerminationReasons...)
	}
	jobWithSidecar := s.jobWithSidecarContainer(batchJob)

	startTimeInS := int64(0)
	if batchJob.Status.StartTime != nil {
		startTimeInS = batchJob.Status.StartTime.Unix()
	}

	customStatus := mapCustomJobStatus(batchJob.Status)
	customStatus.StartTimeInS = startTimeInS
	customStatus.CompletionTimeInS = completionTimeInS
	customStatus.Conditions = jobConditions

	jobResponse := &protos.JobResponse{
		Name:                  batchJob.Name,
		Namespace:             batchJob.Namespace,
		Uuid:                  string(batchJob.UID),
		CreationTimestamp:     batchJob.GetCreationTimestamp().UTC().Format(time.RFC3339),
		Generation:            batchJob.Generation,
		Duration:              duration.String(),
		DurationInS:           durationInS,
		Metadata:              mapObjectMeta(batchJob.ObjectMeta),
		Spec:                  mapJobSpec(batchJob.Spec),
		JobStatus:             mapJobStatus(batchJob.Status),
		Status:                customStatus,
		Succeeded:             jobSucceeded(batchJob),
		Failed:                jobFailed,
		FailureCondition:      failureCondition,
		Pods:                  jobPodsResponses,
		TerminationReasons:    terminationReasons,
		WithSidecarContainers: jobWithSidecar,
	}
	return jobResponse
}

func (s *Sk8lServer) collectCronjobs() {
	x := s.K8sClient.WatchCronjobs()

	go func() {
		for {
			event, more := <-x.ResultChan()
			if more {
				eventCronjob, ok := event.Object.(*batchv1.CronJob)
				if !ok {
					log.Error().
						Str("operation", "collectCronjobs").
						Msg("event.Object.(*batchv1.CronJob)")
				}
				err := s.DB.Update(func(txn *badger.Txn) error {
					return handleCronJobEvent(txn, event, eventCronjob)
				})
				if err != nil {
					panic(err)
				}
			} else {
				x = s.K8sClient.WatchCronjobs()
				log.Error().
					Str("operation", "collectCronjobs").
					Msg("WatchCronjobs: Received all Cronjobs. Opening again")
			}
		}
	}()
}

func (s *Sk8lServer) collectJobs() {
	x := s.K8sClient.WatchJobs()

	go func() {
		for {
			event, more := <-x.ResultChan()
			if more {
				eventJob, ok := event.Object.(*batchv1.Job)
				if !ok {
					log.Error().
						Str("operation", "collectPods").
						Msg("event.Object.(*batchv1.Job)")
				}
				err := s.DB.Update(func(txn *badger.Txn) error {
					return handleJobEvent(txn, event, eventJob)
				})
				if err != nil {
					panic(err)
				}
			} else {
				x = s.K8sClient.WatchJobs()
				log.Error().
					Str("operation", "collectJobs").
					Msg("WatchJobs: Received all Jobs. Opening again")
			}
		}
	}()
}

func (s *Sk8lServer) collectPods() {
	x := s.K8sClient.WatchPods()

	go func() {
		for {
			event, more := <-x.ResultChan()
			if more {
				eventPod, ok := event.Object.(*corev1.Pod)
				if !ok {
					log.Error().
						Str("operation", "collectPods").
						Msg("event.Object.(*corev1.Pod)")
				}
				err := s.DB.Update(func(txn *badger.Txn) error {
					return handlePodEvent(txn, event, eventPod)
				})
				if err != nil {
					panic(err)
				}
			} else {
				x = s.K8sClient.WatchPods()
				log.Error().
					Str("operation", "collectPods").
					Msg("WatchJobs: Received all Pods. Opening again")
			}
		}
	}()
}

func handleCronJobEvent(txn *badger.Txn, event watch.Event, eventCronJob *batchv1.CronJob) error {
	item, err := txn.Get(cronjobsCacheKey)
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
		if err := k8sSerializer.Encode(cjList, &buf); err != nil {
			log.Error().
				Err(err).
				Str("operation", "handleCronJobEvent").
				Msg("k8sSerializer.Encode")
			return fmt.Errorf("%s: Encode() failed: %w", "sk8l#collectCronjobs", err)
		}
		return storeEntry(txn, cronjobsCacheKey, buf.Bytes(), "sk8l#collectCronjobs")
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
	item, err := txn.Get(jobsCacheKey)
	if err != nil {
		jList := &batchv1.JobList{
			Items: []batchv1.Job{*eventJob},
		}
		var buf bytes.Buffer
		if err := k8sSerializer.Encode(jList, &buf); err != nil {
			log.Error().
				Err(err).
				Str("operation", "handleJobEvent").
				Msg("k8sSerializer.Encode")
			return fmt.Errorf("%s: Encode() failed: %w", "sk8l#collectJobs", err)
		}
		return storeEntry(txn, jobsCacheKey, buf.Bytes(), "sk8l#collectJobs")
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
	fKey := fmt.Sprintf(jobPodsKeyFmt, eventPod.Labels["job-name"])
	key := []byte(fKey)
	item, err := txn.Get(key)
	if err != nil {
		podList := &corev1.PodList{
			Items: []corev1.Pod{*eventPod},
		}
		var buf bytes.Buffer
		if err = k8sSerializer.Encode(podList, &buf); err != nil {
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
	_, _, err := k8sSerializer.Decode(stored, nil, storedCjList)
	if err != nil {
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
	if err = k8sSerializer.Encode(storedCjList, &buf); err != nil {
		log.Error().
			Err(err).
			Str("operation", "updateStoredCronjobList").
			Msg("k8sSerializer.Encode")
		return fmt.Errorf("sk8l#collectCronjobs: Encode() failed: %w", err)
	}
	return storeEntry(txn, cronjobsCacheKey, buf.Bytes(), "sk8l#collectJobs")
}

func updateStoredJobList(txn *badger.Txn, stored []byte, event watch.Event, eventJob *batchv1.Job) error {
	storedJList := &batchv1.JobList{}
	_, _, err := k8sSerializer.Decode(stored, nil, storedJList)
	if err != nil {
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
	if err = k8sSerializer.Encode(storedJList, &buf); err != nil {
		log.Error().
			Err(err).
			Str("operation", "updateStoredJobList").
			Msg("k8sSerializer.Encode")
		return fmt.Errorf("sk8l#collectJobs: Encode() failed: %w", err)
	}
	return storeEntry(txn, jobsCacheKey, buf.Bytes(), "sk8l#collectJobs")
}

func updateStoredPodList(txn *badger.Txn, stored []byte, key []byte, eventPod *corev1.Pod) error {
	podList := &corev1.PodList{}
	_, _, err := k8sSerializer.Decode(stored, nil, podList)
	if err != nil {
		log.Error().
			Err(err).
			Str("operation", "updateStoredPodList").
			Msg("k8sSerializer.Decode")
		return fmt.Errorf("operation#k8sSerializer.Decode: %w", err)
	}
	podList.Items = append(podList.Items, *eventPod)
	var buf bytes.Buffer
	if err := k8sSerializer.Encode(podList, &buf); err != nil {
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

func (s *Sk8lServer) allAndRunningJobsAnPods(
	jobs []*batchv1.Job,
	cronjobUID types.UID,
) ([]*protos.JobResponse, []*protos.PodResponse, []*protos.JobResponse, []*protos.PodResponse) {
	jn := len(jobs)
	allJobsForCronJob := make([]*protos.JobResponse, 0, jn)
	allJobPodsForCronjob := make([]*protos.PodResponse, 0)
	runningJobs := make([]*protos.JobResponse, 0, jn)
	runningPods := make([]*protos.PodResponse, 0)

	wg := sync.WaitGroup{}
	wg.Add(jn)

	for _, batchJob := range jobs {
		go func(batchJob *batchv1.Job) {
			defer wg.Done()
			jobResponse := s.buildJobResponse(batchJob)
			allJobsForCronJob = append(allJobsForCronJob, jobResponse)
			allJobPodsForCronjob = append(allJobPodsForCronjob, jobResponse.Pods...)

			if jobResponse.Status.Active > 0 {
				runningJobs = append(runningJobs, jobResponse)
			}

			for _, pod := range jobResponse.Pods {
				if pod.Phase == string(corev1.PodRunning) {
					runningPods = append(runningPods, pod)
				}
			}
		}(batchJob)
	}
	wg.Wait()

	slices.SortFunc(runningJobs,
		func(a, b *protos.JobResponse) int {
			aTime, _ := time.Parse(time.RFC3339, a.CreationTimestamp)
			bTime, _ := time.Parse(time.RFC3339, b.CreationTimestamp)
			return aTime.Compare(bTime)
		})

	slices.SortFunc(allJobsForCronJob,
		func(a, b *protos.JobResponse) int {
			aTime, _ := time.Parse(time.RFC3339, a.CreationTimestamp)
			bTime, _ := time.Parse(time.RFC3339, b.CreationTimestamp)
			return aTime.Compare(bTime)
		})

	return allJobsForCronJob, allJobPodsForCronjob, runningJobs, runningPods
}

func (s *Sk8lServer) jobsForCronjob(jobsMapped map[string][]*batchv1.Job, cronjobName string) []*batchv1.Job {
	if jobs, ok := jobsMapped[cronjobName]; ok {
		return jobs
	}
	return []*batchv1.Job{}
}

func (s *Sk8lServer) cronJobResponse(cronJob batchv1.CronJob, jobsForCronjob []*batchv1.Job) *protos.CronjobResponse {
	allJobsForCronJob, jobPodsForCronJob, runningJobs, runningJobPods := s.allAndRunningJobsAnPods(
		jobsForCronjob,
		cronJob.UID,
	)

	lastDuration := getLastDuration(allJobsForCronJob)
	currentDuration := getCurrentDuration(runningJobs)
	commands := buildCronJobCommand(cronJob)
	lastSuccessfulTime, lastScheduleTime := buildLastTimes(cronJob)

	var cjFailed bool
	for _, job := range allJobsForCronJob {
		if job.Failed {
			cjFailed = true
			break
		}
	}

	return &protos.CronjobResponse{
		Name:               cronJob.Name,
		Namespace:          cronJob.Namespace,
		Uid:                string(cronJob.UID),
		ContainerCommands:  commands,
		Definition:         cronJob.Spec.Schedule,
		CreationTimestamp:  cronJob.GetCreationTimestamp().UTC().Format(time.RFC3339),
		LastSuccessfulTime: lastSuccessfulTime,
		LastScheduleTime:   lastScheduleTime,
		Active:             len(cronJob.Status.Active) > 0,
		LastDuration:       lastDuration,
		CurrentDuration:    currentDuration,
		Jobs:               allJobsForCronJob,
		RunningJobs:        runningJobs,
		RunningJobsPods:    runningJobPods,
		JobsPods:           jobPodsForCronJob,
		Spec:               mapCronJobSpec(cronJob.Spec),
		Failed:             cjFailed,
	}
}

func collectTerminatedAndFailedContainers(
	pod *corev1.Pod,
	statuses []corev1.ContainerStatus,
	terminationReasons *[]*protos.TerminationReason,
) (terminatedContainers []*protos.ContainerResponse, failedContainers []*protos.ContainerResponse) {
	terminatedContainers = make([]*protos.ContainerResponse, 0)
	failedContainers = make([]*protos.ContainerResponse, 0)

	for _, containerStatus := range statuses {
		podConditions := make([]*protos.PodConditionResponse, 0, len(pod.Status.Conditions))
		for _, pc := range pod.Status.Conditions {
			podConditions = append(podConditions, &protos.PodConditionResponse{
				Type:               string(pc.Type),
				Status:             string(pc.Status),
				LastProbeTime:      timeToString(&pc.LastProbeTime),
				LastTransitionTime: timeToString(&pc.LastTransitionTime),
				Reason:             pc.Reason,
				Message:            pc.Message,
			})
		}

		mappedStatus := mapContainerStatus(containerStatus)

		if containerStatus.State.Waiting != nil {
			cr := &protos.ContainerResponse{
				Status:     mappedStatus,
				Phase:      string(pod.Status.Phase),
				Conditions: podConditions,
			}
			terminatedContainers = append(terminatedContainers, cr)

			if containerStatus.State.Waiting.Reason == "CreateContainerConfigError" {
				cr.TerminatedReason = &protos.TerminationReason{
					TerminationDetails: &protos.ContainerStateTerminatedResponse{
						Message:    containerStatus.State.Waiting.Message,
						Reason:     containerStatus.State.Waiting.Reason,
						FinishedAt: timeToString(pod.Status.StartTime),
					},
					ContainerName: mappedStatus.Name,
				}
				*terminationReasons = append(*terminationReasons, cr.TerminatedReason)
				failedContainers = append(failedContainers, cr)
			}
		}

		if containerStatus.State.Terminated != nil {
			cr := &protos.ContainerResponse{
				Status:     mappedStatus,
				Phase:      string(pod.Status.Phase),
				Conditions: podConditions,
			}
			terminatedContainers = append(terminatedContainers, cr)

			if containerStatus.State.Terminated.Reason == "Error" {
				cr.TerminatedReason = &protos.TerminationReason{
					TerminationDetails: mapContainerStateTerminated(containerStatus.State.Terminated),
					ContainerName:      mappedStatus.Name,
				}
				*terminationReasons = append(*terminationReasons, cr.TerminatedReason)
				failedContainers = append(failedContainers, cr)
			}
		}
	}

	return terminatedContainers, failedContainers
}

func terminatedAndFailedContainersResponses(
	pod *corev1.Pod,
) (terminatedContainersResponse *protos.TerminatedContainers, failedContainersResponse *protos.TerminatedContainers) {
	terminatedReasons := make([]*protos.TerminationReason, 0)

	terminatedEphContainers, failedEphContainers := collectTerminatedAndFailedContainers(
		pod,
		pod.Status.EphemeralContainerStatuses,
		&terminatedReasons,
	)
	terminatedInitContainers, failedInitContainers := collectTerminatedAndFailedContainers(
		pod,
		pod.Status.InitContainerStatuses,
		&terminatedReasons,
	)
	terminatedContainers, failedContainers := collectTerminatedAndFailedContainers(
		pod,
		pod.Status.ContainerStatuses,
		&terminatedReasons,
	)

	terminatedContainersResponse = &protos.TerminatedContainers{
		InitContainers:      terminatedInitContainers,
		EphemeralContainers: terminatedEphContainers,
		Containers:          terminatedContainers,
	}

	failedContainersResponse = &protos.TerminatedContainers{
		InitContainers:      failedInitContainers,
		EphemeralContainers: failedEphContainers,
		Containers:          failedContainers,
		TerminationReasons:  terminatedReasons,
	}

	return terminatedContainersResponse, failedContainersResponse
}

func buildJobPodsResponses(gJobPods *corev1.PodList) []*protos.PodResponse {
	n := len(gJobPods.Items)
	jobPodsResponses := make([]*protos.PodResponse, 0, n)
	for _, pod := range gJobPods.Items {
		terminatedContainers, failedContainers := terminatedAndFailedContainersResponses(&pod)
		failed := len(failedContainers.TerminationReasons) > 0
		containerFinishedAtTimes := make([]*metav1.Time, 0)
		for _, x := range terminatedContainers.Containers {
			if x.TerminatedReason != nil && x.TerminatedReason.TerminationDetails != nil {
				finishedAt := x.TerminatedReason.TerminationDetails.FinishedAt
				if finishedAt != "" {
					t, err := time.Parse(time.RFC3339, finishedAt)
					if err == nil && !t.IsZero() {
						mt := metav1.NewTime(t)
						containerFinishedAtTimes = append(containerFinishedAtTimes, &mt)
					}
				}
			}
		}
		var finishedAt string
		if len(containerFinishedAtTimes) > 0 {
			slices.SortFunc(containerFinishedAtTimes,
				func(a, b *metav1.Time) int {
					return a.Compare(b.Time)
				})
			finishedAt = timeToString(containerFinishedAtTimes[len(containerFinishedAtTimes)-1])
		}
		podResponse := &protos.PodResponse{
			Metadata:             mapObjectMeta(pod.ObjectMeta),
			Spec:                 mapPodSpec(pod.Spec),
			Status:               mapPodStatus(pod.Status),
			TerminatedContainers: terminatedContainers,
			FailedContainers:     failedContainers,
			Failed:               failed,
			Phase:                string(pod.Status.Phase),
			TerminationReasons:   failedContainers.TerminationReasons,
			FinishedAt:           finishedAt,
		}
		jobPodsResponses = append(jobPodsResponses, podResponse)
	}
	return jobPodsResponses
}

func jobSucceeded(job *batchv1.Job) bool {
	return job.Status.CompletionTime != nil
}

func buildLastTimes(cronJob batchv1.CronJob) (lastSuccessfulTime string, lastScheduleTime string) {
	if cronJob.Status.LastSuccessfulTime != nil {
		lastSuccessfulTime = cronJob.Status.LastSuccessfulTime.UTC().Format(time.RFC3339)
	}
	if cronJob.Status.LastScheduleTime != nil {
		lastScheduleTime = cronJob.Status.LastScheduleTime.UTC().Format(time.RFC3339)
	}
	return lastSuccessfulTime, lastScheduleTime
}

func buildCronJobCommand(cronJob batchv1.CronJob) map[string]*protos.ContainerCommands {
	commands := make(map[string]*protos.ContainerCommands)
	n := len(cronJob.Spec.JobTemplate.Spec.Template.Spec.InitContainers)
	initContainersCommands := make([]string, 0, n)
	var command bytes.Buffer
	for _, container := range cronJob.Spec.JobTemplate.Spec.Template.Spec.InitContainers {
		for _, ccmd := range container.Command {
			_, err := fmt.Fprintf(&command, "%s ", ccmd)
			if err != nil {
				log.Error().
					Err(err).
					Str("operation", "buildCronJobCommand").
					Msg("InitContainers: command.WriteString")
			}
		}
		initContainersCommands = append(initContainersCommands, command.String())
		command.Reset()
	}

	containersCommands := make([]string, 0, n)
	for _, container := range cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers {
		for _, ccmd := range container.Command {
			_, err := fmt.Fprintf(&command, "%s ", ccmd)
			if err != nil {
				log.Error().
					Err(err).
					Str("operation", "buildCronJobCommand").
					Msg("Containers: command.WriteString")
			}
		}
		containersCommands = append(containersCommands, command.String())
		command.Reset()
	}

	ephemeralContainersCommands := make([]string, 0, n)
	for _, container := range cronJob.Spec.JobTemplate.Spec.Template.Spec.EphemeralContainers {
		for _, ccmd := range container.Command {
			_, err := fmt.Fprintf(&command, "%s ", ccmd)
			if err != nil {
				log.Error().
					Err(err).
					Str("operation", "buildCronJobCommand").
					Msg("EphemeralContainers: command.WriteString")
			}
		}
		ephemeralContainersCommands = append(ephemeralContainersCommands, command.String())
		command.Reset()
	}

	commands["InitContainers"] = &protos.ContainerCommands{Commands: initContainersCommands}
	commands["Containers"] = &protos.ContainerCommands{Commands: containersCommands}
	commands["EphemeralContainers"] = &protos.ContainerCommands{Commands: ephemeralContainersCommands}

	return commands
}

func getCurrentDuration(runningJobsForCronJob []*protos.JobResponse) int64 {
	var lastDuration int64
	if len(runningJobsForCronJob) > 0 {
		last := runningJobsForCronJob[len(runningJobsForCronJob)-1]
		if last.DurationInS != 0 {
			lastDuration = last.DurationInS
		}
	}
	return lastDuration
}

func getLastDuration(allJobsForCronJob []*protos.JobResponse) int64 {
	var lastDuration int64
	if len(allJobsForCronJob) > 0 {
		var i int
		if len(allJobsForCronJob) > 2 {
			i = 2
		} else {
			i = 1
		}
		last := allJobsForCronJob[len(allJobsForCronJob)-i]
		lastDuration = last.DurationInS
	}
	return lastDuration
}

func toDuration(job *batchv1.Job, jobFailed bool, failureCondition *protos.JobConditionResponse) time.Duration {
	var d time.Duration
	status := job.Status
	if jobFailed && failureCondition != nil {
		lastTransition, err := time.Parse(time.RFC3339, failureCondition.LastTransitionTime)
		if err == nil && status.StartTime != nil {
			d = lastTransition.Sub(status.StartTime.Time)
			return d
		}
	}
	if status.StartTime == nil {
		return d
	}
	switch status.CompletionTime {
	case nil:
		d = time.Since(status.StartTime.Time)
	default:
		d = status.CompletionTime.Sub(status.StartTime.Time)
	}
	return d
}

func toDurationInS(job *batchv1.Job, jobFailed bool, failureCondition *protos.JobConditionResponse) int64 {
	d := toDuration(job, jobFailed, failureCondition)
	return int64(d.Seconds())
}

func toCompletionTimeInS(job *batchv1.Job) int64 {
	if job.Status.CompletionTime != nil {
		return job.Status.CompletionTime.Unix()
	}
	return int64(0)
}
