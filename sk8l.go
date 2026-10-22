package main

import (
	"bytes"
	"cmp"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/danroux/sk8l/internal/k8s"
	"github.com/danroux/sk8l/internal/mapper"
	"github.com/danroux/sk8l/internal/store"
	"github.com/danroux/sk8l/protos"
	"google.golang.org/grpc/health/grpc_health_v1"
	gyaml "sigs.k8s.io/yaml"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

//go:embed dashboards/sk8l-overview.json
var dashboardJSON string

var streamHeartbeatInterval = time.Second * store.RefreshSeconds

// isClientCanceled checks if the error or context indicates a client-side cancellation.
func isClientCanceled(ctx context.Context, err error) bool {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return true
	}
	return grpcstatus.Code(err) == codes.Canceled
}

type Sk8lServer struct {
	grpc_health_v1.UnimplementedHealthServer
	protos.UnimplementedCronjobServer
	store.Store
	K8sClient       k8s.ClientInterface
	metricsNamesMap *sync.Map
	target          string
	dialOptions     []grpc.DialOption
	watchInterval   time.Duration
}

func NewSk8lServer(
	target string,
	st store.Store,
	metricsNamesMap *sync.Map,
	dialOptions ...grpc.DialOption,
) *Sk8lServer {
	var k8sCl k8s.ClientInterface
	if c, ok := st.(*store.CronJobDBStore); ok {
		k8sCl = c.K8sClient
	}
	return &Sk8lServer{
		target:          target,
		Store:           st,
		K8sClient:       k8sCl,
		metricsNamesMap: metricsNamesMap,
		dialOptions:     dialOptions,
		watchInterval:   5 * time.Second,
	}
}

func (s *Sk8lServer) GetTarget() string {
	return s.target
}

func (s *Sk8lServer) GetDialOptions() []grpc.DialOption {
	return s.dialOptions
}

// IsHealthy reports whether the server's backing store is ready.
func (s *Sk8lServer) IsHealthy() bool {
	return s.Store != nil && s.Store.Ready()
}

func (s Sk8lServer) Check(
	ctx context.Context,
	req *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	status := grpc_health_v1.HealthCheckResponse_NOT_SERVING
	if s.IsHealthy() {
		status = grpc_health_v1.HealthCheckResponse_SERVING
	}

	log.Info().
		Str("component", "probe").
		Str("operation", "health").
		Str("status", status.String()).
		Msg("health check")

	return &grpc_health_v1.HealthCheckResponse{Status: status}, nil
}

// Watch implements the gRPC Health Checking Protocol streaming RPC.
// It sends an initial status immediately, then keeps the stream open and
// pushes a new message whenever the health status transitions. The stream
// is closed when the client disconnects or the server context is canceled.
func (s Sk8lServer) Watch(req *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error {
	ticker := time.NewTicker(s.watchInterval)
	defer ticker.Stop()

	send := func(servingStatus grpc_health_v1.HealthCheckResponse_ServingStatus) error {
		if err := stream.Send(&grpc_health_v1.HealthCheckResponse{Status: servingStatus}); err != nil {
			if isClientCanceled(stream.Context(), err) {
				log.Debug().Str("operation", "Watch#stream.Send").Msg("stream canceled by client")
				return nil
			}
			log.Error().
				Err(err).
				Str("operation", "Watch#stream.Send").
				Send()
			return fmt.Errorf("sk8l#Watch: stream.Send failed: %w", err)
		}
		return nil
	}

	healthStatus := func() grpc_health_v1.HealthCheckResponse_ServingStatus {
		if s.IsHealthy() {
			return grpc_health_v1.HealthCheckResponse_SERVING
		}
		return grpc_health_v1.HealthCheckResponse_NOT_SERVING
	}

	// Send initial status immediately.
	currentStatus := healthStatus()
	if err := send(currentStatus); err != nil {
		return err
	}

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
			if stream.Context().Err() != nil {
				return nil
			}
			newStatus := healthStatus()
			if newStatus == currentStatus {
				continue
			}
			currentStatus = newStatus
			if err := send(currentStatus); err != nil {
				return err
			}
		}
	}
}

func (s *Sk8lServer) Run(metricsCxt context.Context) {
	if starter, ok := s.Store.(store.Starter); ok {
		starter.Start(metricsCxt)
	}
	recordMetrics(metricsCxt, s, s.metricsNamesMap)
}

func (s *Sk8lServer) GetCronjobs(in *protos.CronjobsRequest, stream protos.Cronjob_GetCronjobsServer) error {
	ctx := stream.Context()

	send := func() error {
		cronJobList, err := s.FindCronjobs()
		if err != nil {
			if isClientCanceled(ctx, err) {
				log.Debug().Str("operation", "GetCronjobs").Msg("client canceled stream; aborting FindCronjobs")
				return nil
			}
			log.Error().Err(err).Str("operation", "GetCronjobs").Msg("FindCronjobs")
			return fmt.Errorf("sk8l#GetCronjobs: FindCronjobs() failed: %w", err)
		}

		jobsMapped, err := s.FindJobsMapped(ctx)
		if err != nil {
			if isClientCanceled(ctx, err) {
				log.Debug().Str("operation", "GetCronjobs").Msg("client canceled stream; aborting FindJobsMapped")
				return nil
			}
			log.Error().Err(err).Str("operation", "GetCronjobs").Msg("FindJobsMapped")
			return fmt.Errorf("sk8l#GetCronjobs: FindJobsMapped() failed: %w", err)
		}

		n := len(cronJobList.Items)
		cronjobs := make([]*protos.CronjobResponse, 0, n)

		var mu sync.Mutex
		wg := sync.WaitGroup{}
		wg.Add(n)
		for _, cronjobItem := range cronJobList.Items {
			go func(cronjobItem batchv1.CronJob) {
				defer wg.Done()
				jobsForCronjob := s.jobsForCronjob(jobsMapped, cronjobItem.Name)
				cronjob := s.cronJobResponse(cronjobItem, jobsForCronjob)
				mu.Lock()
				cronjobs = append(cronjobs, cronjob)
				mu.Unlock()
			}(cronjobItem)
		}
		wg.Wait()

		slices.SortFunc(cronjobs,
			func(a, b *protos.CronjobResponse) int {
				return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
			})

		if err := stream.Send(&protos.CronjobsResponse{Cronjobs: cronjobs}); err != nil {
			if isClientCanceled(ctx, err) {
				log.Debug().Str("operation", "GetCronjobs").Msg("client canceled stream; aborting stream.Send")
				return nil
			}
			return fmt.Errorf("sk8l#GetCronjobs: stream.Send() failed: %w", err)
		}
		return nil
	}

	if err := send(); err != nil {
		return err
	}

	notifyCh := s.Subscribe(ctx)

	ticker := time.NewTicker(streamHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-notifyCh:
			if !ok || ctx.Err() != nil {
				return nil
			}
			if err := send(); err != nil {
				return err
			}
		case <-ticker.C:
			if ctx.Err() != nil {
				return nil
			}
			if err := send(); err != nil {
				return err
			}
		}
	}
}

func (s *Sk8lServer) GetCronjob(in *protos.CronjobRequest, stream protos.Cronjob_GetCronjobServer) error {
	ctx := stream.Context()

	send := func() error {
		cronjob, err := s.FindCronjob(ctx, in.CronjobNamespace, in.CronjobName)
		if err != nil {
			if isClientCanceled(ctx, err) {
				log.Debug().Str("operation", "GetCronjob").Msg("client canceled stream; aborting FindCronjob")
				return nil
			}
			log.Error().Err(err).Str("operation", "GetCronjob").Msg("FindCronjob")
			return fmt.Errorf("sk8l#GetCronjob: FindCronjob() failed: %w", err)
		}

		jobsMapped, err := s.FindJobsMapped(ctx)
		if err != nil {
			if isClientCanceled(ctx, err) {
				log.Debug().Str("operation", "GetCronjob").Msg("client canceled stream; aborting FindJobsMapped")
				return nil
			}
			log.Error().Err(err).Str("operation", "GetCronjob").Msg("FindJobsMapped")
			return fmt.Errorf("sk8l#GetCronjob: FindJobsMapped() failed: %w", err)
		}

		jobsForCronjob := s.jobsForCronjob(jobsMapped, cronjob.Name)
		if err := stream.Send(s.cronJobResponse(*cronjob, jobsForCronjob)); err != nil {
			if isClientCanceled(ctx, err) {
				log.Debug().Str("operation", "GetCronjob").Msg("client canceled stream; aborting stream.Send")
				return nil
			}
			return fmt.Errorf("sk8l#GetCronjob: stream.Send() failed: %w", err)
		}
		return nil
	}

	if err := send(); err != nil {
		return err
	}

	notifyCh := s.Subscribe(ctx)

	ticker := time.NewTicker(streamHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-notifyCh:
			if !ok || ctx.Err() != nil {
				return nil
			}
			if err := send(); err != nil {
				return err
			}
		case <-ticker.C:
			if ctx.Err() != nil {
				return nil
			}
			if err := send(); err != nil {
				return err
			}
		}
	}
}

func (s *Sk8lServer) GetCronjobPods(in *protos.CronjobPodsRequest, stream protos.Cronjob_GetCronjobPodsServer) error {
	ctx := stream.Context()

	send := func() error {
		cronjob, err := s.FindCronjob(ctx, in.CronjobNamespace, in.CronjobName)
		if err != nil {
			if isClientCanceled(ctx, err) {
				log.Debug().Str("operation", "GetCronjobPods").Msg("client canceled stream; aborting FindCronjob")
				return nil
			}
			log.Error().Err(err).Str("operation", "GetCronjobPods").Msg("FindCronjob")
			return fmt.Errorf("sk8l#GetCronjobPods: FindCronjob() failed: %w", err)
		}

		jobsMapped, err := s.FindJobsMapped(ctx)
		if err != nil {
			if isClientCanceled(ctx, err) {
				log.Debug().Str("operation", "GetCronjobPods").Msg("client canceled stream; aborting FindJobsMapped")
				return nil
			}
			log.Error().Err(err).Str("operation", "GetCronjobPods").Msg("FindJobsMapped")
			return fmt.Errorf("sk8l#GetCronjobPods: FindJobsMapped() failed: %w", err)
		}

		jobs := s.jobsForCronjob(jobsMapped, cronjob.Name)
		cronjobResponse := s.cronJobResponse(*cronjob, jobs)

		slices.SortFunc(cronjobResponse.JobsPods,
			func(a, b *protos.PodResponse) int {
				return strings.Compare(a.Status.StartTime, b.Status.StartTime)
			})

		if err := stream.Send(&protos.CronjobPodsResponse{
			Pods: cronjobResponse.JobsPods,
			Cronjob: &protos.CronjobResponse{
				Name:      cronjob.Name,
				Namespace: cronjob.Namespace,
				Jobs:      cronjobResponse.Jobs,
			},
		}); err != nil {
			if isClientCanceled(ctx, err) {
				log.Debug().Str("operation", "GetCronjobPods").Msg("client canceled stream; aborting stream.Send")
				return nil
			}
			return fmt.Errorf("sk8l#GetCronjobPods: stream.Send() failed: %w", err)
		}
		return nil
	}

	if err := send(); err != nil {
		return err
	}

	notifyCh := s.Subscribe(ctx)

	ticker := time.NewTicker(streamHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-notifyCh:
			if !ok || ctx.Err() != nil {
				return nil
			}
			if err := send(); err != nil {
				return err
			}
		case <-ticker.C:
			if ctx.Err() != nil {
				return nil
			}
			if err := send(); err != nil {
				return err
			}
		}
	}
}

func (s *Sk8lServer) GetJobs(in *protos.JobsRequest, stream protos.Cronjob_GetJobsServer) error {
	ctx := stream.Context()

	send := func() error {
		jobList, err := s.FindJobs()
		if err != nil {
			if isClientCanceled(ctx, err) {
				log.Debug().Str("operation", "GetJobs").Msg("client canceled stream; aborting FindJobs")
				return nil
			}
			log.Error().Err(err).Str("operation", "GetJobs").Msg("FindJobs")
			return fmt.Errorf("sk8l#GetJobs: FindJobs() failed: %w", err)
		}

		// filter out jobs that belong to cronjobs
		jobTasks := make([]*batchv1.Job, 0, len(jobList.Items))
		for i := range jobList.Items {
			if jobList.Items[i].OwnerReferences == nil {
				jobTasks = append(jobTasks, &jobList.Items[i])
			}
		}

		n := len(jobTasks)
		jobs := make([]*protos.JobResponse, 0, n)
		for _, job := range jobTasks {
			jobs = append(jobs, s.buildJobResponse(job))
		}

		if err := stream.Send(&protos.JobsResponse{Jobs: jobs}); err != nil {
			if isClientCanceled(ctx, err) {
				log.Debug().Str("operation", "GetJobs").Msg("client canceled stream; aborting stream.Send")
				return nil
			}
			return fmt.Errorf("sk8l#GetJobs: stream.Send() failed: %w", err)
		}
		return nil
	}

	if err := send(); err != nil {
		return err
	}

	notifyCh := s.Subscribe(ctx)

	ticker := time.NewTicker(streamHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-notifyCh:
			if !ok || ctx.Err() != nil {
				return nil
			}
			if err := send(); err != nil {
				return err
			}
		case <-ticker.C:
			if ctx.Err() != nil {
				return nil
			}
			if err := send(); err != nil {
				return err
			}
		}
	}
}

func (s *Sk8lServer) GetCronjobYAML(
	ctx context.Context,
	in *protos.CronjobRequest,
) (*protos.CronjobYAMLResponse, error) {
	cronjob, err := s.K8sClient.GetCronjob(ctx, in.CronjobNamespace, in.CronjobName)
	if err != nil {
		return nil, fmt.Errorf("sk8l#GetCronjobYAML: %w", err)
	}
	prettyJSON, err := json.MarshalIndent(cronjob, "", "  ")

	if err != nil {
		log.Error().
			Err(err).
			Str("operation", "sk8l#GetCronjobYAML").
			Msg("json.MarshalIndent() failed")
		return nil, fmt.Errorf("sk8l#GetCronjobYAML: json.MarshalIndent failed: %w", err)
	}

	y, _ := gyaml.JSONToYAML(prettyJSON)

	response := &protos.CronjobYAMLResponse{
		Cronjob: string(y),
	}

	return response, nil
}

func (s *Sk8lServer) GetJobYAML(ctx context.Context, in *protos.JobRequest) (*protos.JobYAMLResponse, error) {
	job, err := s.K8sClient.GetJob(ctx, in.JobNamespace, in.JobName)
	if err != nil {
		return nil, fmt.Errorf("sk8l#GetJobYAML: %w", err)
	}
	prettyJSON, err := json.MarshalIndent(job, "", "  ")

	if err != nil {
		log.Error().
			Err(err).
			Str("operation", "sk8l#GetJobYAML").
			Msg("json.MarshalIndent() failed")
		return nil, fmt.Errorf("sk8l#GetJobYAML: json.MarshalIndent failed: %w", err)
	}

	y, _ := gyaml.JSONToYAML(prettyJSON)

	response := &protos.JobYAMLResponse{
		Job: string(y),
	}

	return response, nil
}

func (s *Sk8lServer) GetPodYAML(ctx context.Context, in *protos.PodRequest) (*protos.PodYAMLResponse, error) {
	pod, err := s.K8sClient.GetPod(ctx, in.PodNamespace, in.PodName)
	if err != nil {
		return nil, fmt.Errorf("sk8l#GetPodYAML: %w", err)
	}
	prettyJSON, err := json.MarshalIndent(pod, "", "  ")

	if err != nil {
		log.Error().
			Err(err).
			Str("operation", "sk8l#GetPodYAML").
			Msg("json.MarshalIndent() failed")
		return nil, fmt.Errorf("sk8l#GetPodYAML: json.MarshalIndent failed: %w", err)
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
	return &protos.DashboardAnnotationsResponse{
		Annotations: dashboardJSON,
	}, nil
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
				failureCondition = mapper.MapJobCondition(&jobCondition)
			}
		}
		jobConditions = append(jobConditions, &protos.JobCondition{
			Type:               string(jobCondition.Type),
			Status:             string(jobCondition.Status),
			LastProbeTime:      mapper.TimeToString(&jobCondition.LastProbeTime),
			LastTransitionTime: mapper.TimeToString(&jobCondition.LastTransitionTime),
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
	jobPodsForJob, err := s.FindJobPodsForJob(batchJob)
	if err != nil {
		log.Error().Err(err).Str("operation", "buildJobResponse").Msg("FindJobPodsForJob")
		jobPodsForJob = &corev1.PodList{}
	}

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

	customStatus := mapper.MapCustomJobStatus(batchJob.Status)
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
		Metadata:              mapper.MapObjectMeta(batchJob.ObjectMeta),
		Spec:                  mapper.MapJobSpec(batchJob.Spec),
		JobStatus:             mapper.MapJobStatus(batchJob.Status),
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

func (s *Sk8lServer) allAndRunningJobsAnPods(
	jobs []*batchv1.Job,
	cronjobUID types.UID,
) ([]*protos.JobResponse, []*protos.PodResponse, []*protos.JobResponse, []*protos.PodResponse) {
	jn := len(jobs)
	allJobsForCronJob := make([]*protos.JobResponse, 0, jn)
	allJobPodsForCronjob := make([]*protos.PodResponse, 0)
	runningJobs := make([]*protos.JobResponse, 0, jn)
	runningPods := make([]*protos.PodResponse, 0)

	var mu sync.Mutex
	wg := sync.WaitGroup{}
	wg.Add(jn)

	for _, batchJob := range jobs {
		go func(batchJob *batchv1.Job) {
			defer wg.Done()
			jobResponse := s.buildJobResponse(batchJob)

			running := jobResponse.Status.Active > 0
			runningPodsList := make([]*protos.PodResponse, 0)
			for _, pod := range jobResponse.Pods {
				if pod.Phase == string(corev1.PodRunning) {
					runningPodsList = append(runningPodsList, pod)
				}
			}

			mu.Lock()
			allJobsForCronJob = append(allJobsForCronJob, jobResponse)
			allJobPodsForCronjob = append(allJobPodsForCronjob, jobResponse.Pods...)
			if running {
				runningJobs = append(runningJobs, jobResponse)
			}
			runningPods = append(runningPods, runningPodsList...)
			mu.Unlock()
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
		Spec:               mapper.MapCronJobSpec(cronJob.Spec),
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
				LastProbeTime:      mapper.TimeToString(&pc.LastProbeTime),
				LastTransitionTime: mapper.TimeToString(&pc.LastTransitionTime),
				Reason:             pc.Reason,
				Message:            pc.Message,
			})
		}

		mappedStatus := mapper.MapContainerStatus(containerStatus)

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
						FinishedAt: mapper.TimeToString(pod.Status.StartTime),
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
					TerminationDetails: mapper.MapContainerStateTerminated(containerStatus.State.Terminated),
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
			finishedAt = mapper.TimeToString(containerFinishedAtTimes[len(containerFinishedAtTimes)-1])
		}
		podResponse := &protos.PodResponse{
			Metadata:             mapper.MapObjectMeta(pod.ObjectMeta),
			Spec:                 mapper.MapPodSpec(pod.Spec),
			Status:               mapper.MapPodStatus(pod.Status),
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
