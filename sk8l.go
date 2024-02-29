package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/danroux/sk8l/protos"
	badger "github.com/dgraph-io/badger/v4"
	gyaml "github.com/ghodss/yaml"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/health/grpc_health_v1"

	// structpb "google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/grpc"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	jobPodsKeyFmt = "jobs_pods_for_job_%s"
)

type Sk8lServer struct {
	grpc_health_v1.UnimplementedHealthServer
	protos.UnimplementedCronjobServer
	K8sClient *K8sClient
	*badger.DB
	Target  string
	Options []grpc.DialOption
}

type APICall (func() []byte)

func (h Sk8lServer) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	log.Default().Println("serving health")
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (h Sk8lServer) Watch(req *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error {
	response := &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}

	if err := stream.Send(response); err != nil {
		return err
	}

	return nil
}

func (s *Sk8lServer) GetCronjobs(in *protos.CronjobsRequest, stream protos.Cronjob_GetCronjobsServer) error {
	for {
		cronJobList := s.findCronjobs()
		jobsMapped := s.findJobs()

		// cronJobList := getMocks()
		// mocked := getMocks().Items
		// cronJobList.Items = append(cronJobList.Items, mocked...)

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

		if err := stream.Send(y); err != nil {
			return err
		}

		time.Sleep(time.Second * 10)
	}
}

func (s *Sk8lServer) GetCronjob(in *protos.CronjobRequest, stream protos.Cronjob_GetCronjobServer) error {
	for {
		cronjob := s.findCronjob(in.CronjobNamespace, in.CronjobName)

		jobsMapped := s.findJobs()
		jobsForCronjob := s.jobsForCronjob(jobsMapped, cronjob.Name)
		cronjobPodsResponse := s.cronJobResponse(*cronjob, jobsForCronjob)
		if err := stream.Send(cronjobPodsResponse); err != nil {
			return err
		}

		time.Sleep(time.Second * 10)
	}
}

func (s *Sk8lServer) GetCronjobPods(in *protos.CronjobPodsRequest, stream protos.Cronjob_GetCronjobPodsServer) error {
	for {
		cronjob := s.findCronjob(in.CronjobNamespace, in.CronjobName)

		jobsMapped := s.findJobs()
		jobs := s.jobsForCronjob(jobsMapped, cronjob.Name)

		cronjobResponse := s.cronJobResponse(*cronjob, jobs)
		lightweightCronjobPodsResponse := &protos.CronjobResponse{
			Name:      cronjob.Name,
			Namespace: cronjob.Namespace,
			Jobs:      cronjobResponse.Jobs,
		}

		slices.SortFunc(cronjobResponse.JobsPods,
			func(a, b *protos.PodResponse) int {
				aStartTime := a.Status.StartTime
				bStartTime := b.Status.StartTime
				return aStartTime.Compare(bStartTime.Time)
			})

		cronjobPodsResponse := &protos.CronjobPodsResponse{
			Pods:    cronjobResponse.JobsPods,
			Cronjob: lightweightCronjobPodsResponse,
		}

		if err := stream.Send(cronjobPodsResponse); err != nil {
			return err
		}

		time.Sleep(time.Second * 10)
	}
}

func (s *Sk8lServer) GetCronjobYAML(ctx context.Context, in *protos.CronjobRequest) (*protos.CronjobYAMLResponse, error) {
	cronjob := s.K8sClient.GetCronjob(in.CronjobNamespace, in.CronjobName)
	prettyJson, _ := json.MarshalIndent(cronjob, "", "  ")

	y, _ := gyaml.JSONToYAML(prettyJson)

	response := &protos.CronjobYAMLResponse{
		Cronjob: string(y),
	}

	return response, nil
}

func (s *Sk8lServer) GetJobYAML(ctx context.Context, in *protos.JobRequest) (*protos.JobYAMLResponse, error) {
	job := s.K8sClient.GetJob(in.JobNamespace, in.JobName)
	prettyJson, _ := json.MarshalIndent(job, "", "  ")

	y, _ := gyaml.JSONToYAML(prettyJson)

	response := &protos.JobYAMLResponse{
		Job: string(y),
	}

	return response, nil
}

func (s *Sk8lServer) GetPodYAML(ctx context.Context, in *protos.PodRequest) (*protos.PodYAMLResponse, error) {
	pod := s.K8sClient.GetPod(in.PodNamespace, in.PodName)
	prettyJson, _ := json.MarshalIndent(pod, "", "  ")

	y, _ := gyaml.JSONToYAML(prettyJson)

	response := &protos.PodYAMLResponse{
		Pod: string(y),
	}

	return response, nil
}

func (s *Sk8lServer) GetDashboardAnnotations(context.Context, *protos.DashboardAnnotationsRequest) (*protos.DashboardAnnotationsResponse, error) {
	panels := generatePanels()

	var tmplFile = "annotations.tmpl"
	t := template.New(tmplFile)
	// t = t.Funcs(template.FuncMap{"StringsJoin": strings.Join})
	t = t.Funcs(template.FuncMap{"marshal": func(v interface{}) string {
		a, _ := json.Marshal(v)
		return string(a)
	},
	},
	)
	t = template.Must(t.ParseFiles(tmplFile))

	var b bytes.Buffer
	err := t.Execute(&b, panels)
	if err != nil {
		log.Println("executing template:", err)
	}

	response := &protos.DashboardAnnotationsResponse{
		Annotations: b.String(),
	}

	return response, nil
}

func (s *Sk8lServer) getAndStore(key []byte, apiCall APICall) ([]byte, error) {
	var valueResponse []byte
	err := s.DB.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(key)

		if errors.Is(err, badger.ErrKeyNotFound) {
			err = s.DB.Update(func(txn *badger.Txn) error {
				apiResult := apiCall()
				entry := badger.NewEntry(key, apiResult).WithTTL(time.Second * 15)
				err := txn.SetEntry(entry)
				valueResponse = append([]byte{}, apiResult...)
				return err
			})

		} else {
			err = item.Value(func(val []byte) error {
				valueResponse = append([]byte{}, val...)

				return nil
			})
		}

		return err
	})

	if err == nil {
		return valueResponse, nil
	}

	return nil, err
}

func (s *Sk8lServer) findCronjobs() *batchv1.CronJobList {
	gCjsCall := func() []byte {
		result := s.K8sClient.GetCronjobs()
		value, _ := proto.Marshal(result)
		return value
	}

	key := []byte("sk8l_cronjobs")
	value, err := s.getAndStore(key, gCjsCall)

	if err != nil {

	}

	cronJobList := &batchv1.CronJobList{}
	err = proto.Unmarshal(value, cronJobList)

	if err != nil {

	}

	return cronJobList
}

func (s *Sk8lServer) findCronjob(cronjobNamespace, cronjobName string) *batchv1.CronJob {
	gCjCall := func() []byte {
		cronjobName := cronjobName
		cronjobNamespace := cronjobNamespace
		cronjob := s.K8sClient.GetCronjob(cronjobNamespace, cronjobName)
		cronjobValue, _ := proto.Marshal(cronjob)
		return cronjobValue
	}

	key := []byte(fmt.Sprintf("sk8l_cronjob_%s_%s", cronjobNamespace, cronjobName))
	cronjobValue, err := s.getAndStore(key, gCjCall)

	if err != nil {

	}

	cronjob := &batchv1.CronJob{}
	err = proto.Unmarshal(cronjobValue, cronjob)

	if err != nil {

	}

	return cronjob
}

func (s *Sk8lServer) findJobs() *protos.MappedJobs {
	jobsCall := func() []byte {
		result := s.K8sClient.GetAllJobsMapped()
		value, _ := proto.Marshal(result)
		return value
	}

	key := []byte("sk8l_jobs")
	value, err := s.getAndStore(key, jobsCall)

	if err != nil {

	}

	mappedJobs := &protos.MappedJobs{}

	err = proto.Unmarshal(value, mappedJobs)

	if err != nil {

	}

	return mappedJobs
}

func (s *Sk8lServer) findJobPodsForJob(job *batchv1.Job) *corev1.PodList {
	fKey := fmt.Sprintf(jobPodsKeyFmt, job.Name)
	key := []byte(fKey)
	collection := &corev1.PodList{}

	s.DB.View(func(txn *badger.Txn) error {
		current, err := txn.Get(key)
		// Your code here…

		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}

		current.Value(func(val []byte) error {
			proto.Unmarshal(val, collection)

			return nil
		})

		return nil
	})

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

	podList := &corev1.PodList{
		Items: podItems,
	}

	return podList
}

// Revisit this. JobConditions are not being used yet anywhere. PodResponse.TerminationReasons.TerminationDetails -> ContainerStateTerminated
func jobFailed(job *batchv1.Job, jobPodsResponses []*protos.PodResponse) (bool, *batchv1.JobCondition, []*batchv1.JobCondition) {
	var jobFailed bool
	var failureCondition *batchv1.JobCondition

	n := len(job.Status.Conditions)
	jobConditions := make([]*batchv1.JobCondition, 0, n)
	for _, jobCondition := range job.Status.Conditions {
		if jobFailed != true {
			if jobCondition.Type == batchv1.JobFailed {
				jobFailed = true
				failureCondition = &jobCondition
			}
		}
		jobConditions = append(jobConditions, &jobCondition)
	}

	for _, pr := range jobPodsResponses {
		if pr.Failed {
			jobFailed = true
		}
	}

	return jobFailed, failureCondition, jobConditions
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

	jobResponse := &protos.JobResponse{
		Name:              batchJob.Name,
		Namespace:         batchJob.Namespace,
		Uuid:              string(batchJob.UID),
		CreationTimestamp: batchJob.GetCreationTimestamp().UTC().Format(time.RFC3339),
		Generation:        batchJob.Generation,
		Duration:          duration.String(),
		DurationInS:       durationInS,
		Spec:              &batchJob.Spec,
		Status: &protos.JobStatus{
			StartTime:         batchJob.Status.StartTime,
			StartTimeInS:      batchJob.Status.StartTime.Unix(),
			CompletionTime:    batchJob.Status.CompletionTime,
			CompletionTimeInS: completionTimeInS,
			Active:            &batchJob.Status.Active,
			Failed:            &batchJob.Status.Failed,
			Ready:             batchJob.Status.Ready,
			Succeeded:         &batchJob.Status.Succeeded,
			Conditions:        jobConditions,
		},
		Succeeded:          jobSucceded(batchJob),
		Failed:             jobFailed,
		FailureCondition:   failureCondition,
		Pods:               jobPodsResponses,
		TerminationReasons: terminationReasons,
	}

	return jobResponse
}

func (s *Sk8lServer) WatchPods() {
	x := s.K8sClient.WatchPods()

	go func() {
		for {
			event, more := <-x.ResultChan()
			if more {
				podObject := event.Object.(*corev1.Pod)
				log.Println("Job watching - received pod", event.Type, podObject.Name, podObject.ResourceVersion)

				fKey := fmt.Sprintf(jobPodsKeyFmt, podObject.Labels["job-name"])
				key := []byte(fKey)
				err := s.DB.Update(func(txn *badger.Txn) error {
					item, err := txn.Get(key)
					if err != nil {
						rec := &corev1.PodList{
							Items: []corev1.Pod{*podObject},
						}

						result, _ := proto.Marshal(rec)
						entry := badger.NewEntry(key, result)
						err := txn.SetEntry(entry)
						return err
					}

					err = item.Value(func(val []byte) error {
						rec := &corev1.PodList{}
						proto.Unmarshal(val, rec)

						rec.Items = append(rec.Items, *podObject)
						result, _ := proto.Marshal(rec)

						entry := badger.NewEntry(key, result)
						err := txn.SetEntry(entry)
						return err
					})

					return err
				})

				if err != nil {
					panic(err)
				}
			} else {
				x = s.K8sClient.WatchPods()
				log.Println("Job watching - Received all Pods. Opening again", x)
			}
		}
	}()
}

func (s *Sk8lServer) allAndRunningJobsAnPods(jobs []*batchv1.Job, cronjobUID types.UID) ([]*protos.JobResponse, []*protos.PodResponse, []*protos.JobResponse, []*protos.PodResponse) {
	jn := len(jobs)
	allJobsForCronJob := make([]*protos.JobResponse, 0, jn)
	allJobPodsForCronjob := make([]*protos.PodResponse, 0)
	runningJobs := make([]*protos.JobResponse, 0, jn)
	runningPods := make([]*protos.PodResponse, 0)

	wg := sync.WaitGroup{}
	wg.Add(jn)

	// go through all jobs and get the ones that match the jobUID(owner)
	for _, batchJob := range jobs {
		go func(batchJob *batchv1.Job) {
			defer wg.Done()
			jobResponse := s.buildJobResponse(batchJob)
			allJobsForCronJob = append(allJobsForCronJob, jobResponse)
			allJobPodsForCronjob = append(allJobPodsForCronjob, jobResponse.Pods...)

			if *jobResponse.Status.Active > 0 {
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

func (s *Sk8lServer) jobsForCronjob(jobsMapped *protos.MappedJobs, cronjobName string) []*batchv1.Job {
	if jobsMapped.JobLists[cronjobName] == nil {
		return []*batchv1.Job{}
	}

	return jobsMapped.JobLists[cronjobName].Items
}

func (s *Sk8lServer) cronJobResponse(cronJob batchv1.CronJob, jobsForCronjob []*batchv1.Job) *protos.CronjobResponse {
	allJobsForCronJob, jobPodsForCronJob, runningJobs, runningJobPods := s.allAndRunningJobsAnPods(jobsForCronjob, cronJob.UID)

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

	cjr := &protos.CronjobResponse{
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
		Spec:               &cronJob.Spec,
		Failed:             cjFailed,
	}

	return cjr
}
func collectTerminatedAndFailedContainers(pod *corev1.Pod, statuses []corev1.ContainerStatus, terminationReasons *[]*protos.TerminationReason) ([]*protos.ContainerResponse, []*protos.ContainerResponse) {
	terminatedContainers := make([]*protos.ContainerResponse, 0)
	failedContainers := make([]*protos.ContainerResponse, 0)

	for _, containerStatus := range statuses {
		// ephStates = append(ephStates, container.State)
		// if container.State.Waiting != nil && container.State.Waiting.Reason == "Error" {
		//      failedEphContainers = append(failedEphContainers, &container)
		// }
		containerStatus := containerStatus

		podConditions := []*corev1.PodCondition{}
		for _, pc := range pod.Status.Conditions {
			pc := pc
			podConditions = append(podConditions, &pc)
		}

		if containerStatus.State.Waiting != nil {
			cr := &protos.ContainerResponse{
				Status:     &containerStatus,
				Phase:      string(pod.Status.Phase),
				Conditions: podConditions,
			}
			terminatedContainers = append(terminatedContainers, cr)

			if containerStatus.State.Waiting.Reason == "CreateContainerConfigError" {
				waitingTerminated := &corev1.ContainerStateTerminated{
					Message:    containerStatus.State.Waiting.Message,
					Reason:     containerStatus.State.Waiting.Reason,
					FinishedAt: *pod.Status.StartTime,
				}
				cr.TerminatedReason = &protos.TerminationReason{
					TerminationDetails: waitingTerminated,
					ContainerName:      cr.Status.Name,
				}
				*terminationReasons = append(*terminationReasons, cr.TerminatedReason)
				failedContainers = append(failedContainers, cr)
			}
		}

		if containerStatus.State.Terminated != nil {
			cr := &protos.ContainerResponse{
				Status:     &containerStatus,
				Phase:      string(pod.Status.Phase),
				Conditions: podConditions,
			}
			terminatedContainers = append(terminatedContainers, cr)

			if containerStatus.State.Terminated.Reason == "Error" {
				cr.TerminatedReason = &protos.TerminationReason{
					TerminationDetails: containerStatus.State.Terminated,
					ContainerName:      cr.Status.Name,
				}
				*terminationReasons = append(*terminationReasons, cr.TerminatedReason)
				failedContainers = append(failedContainers, cr)
			}
		}
	}

	return terminatedContainers, failedContainers
}

func terminatedAndFailedContainersResponses(pod *corev1.Pod) (*protos.TerminatedContainers, *protos.TerminatedContainers) {
	terminatedReasons := make([]*protos.TerminationReason, 0)

	terminatedEphContainers, failedEphContainers := collectTerminatedAndFailedContainers(pod, pod.Status.EphemeralContainerStatuses, &terminatedReasons)
	terminatedInitContainers, failedInitContainers := collectTerminatedAndFailedContainers(pod, pod.Status.InitContainerStatuses, &terminatedReasons)
	terminatedContainers, failedContainers := collectTerminatedAndFailedContainers(pod, pod.Status.ContainerStatuses, &terminatedReasons)

	terminatedContainersResponse := &protos.TerminatedContainers{
		InitContainers:      terminatedInitContainers,
		EphemeralContainers: terminatedEphContainers,
		Containers:          terminatedContainers,
	}

	failedContainersResponse := &protos.TerminatedContainers{
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

	// ephStates := make([]corev1.ContainerState, 0)
	for _, pod := range gJobPods.Items {
		pod := pod
		// jobPodsForJob.Items[0].Status.ContainerStatuses
		// jobPodsForJob.Items[0].Status.InitContainerStatuses
		terminatedContainers, failedContainers := terminatedAndFailedContainersResponses(&pod)
		failed := len(failedContainers.TerminationReasons) > 0

		var containerTerminatedState *corev1.ContainerStateTerminated
		containerFinishedAtTimes := make([]*metav1.Time, 0)
		for _, x := range terminatedContainers.Containers {
			containerTerminatedState = x.Status.State.Terminated
			if containerTerminatedState != nil && !containerTerminatedState.FinishedAt.Time.IsZero() {
				containerFinishedAtTimes = append(containerFinishedAtTimes, &containerTerminatedState.FinishedAt)
			}
		}

		var finishedAt *metav1.Time
		if len(containerFinishedAtTimes) > 0 {
			slices.SortFunc(containerFinishedAtTimes,
				func(aFinishedAtTime, bFinishedAtTime *metav1.Time) int {
					return aFinishedAtTime.Compare(bFinishedAtTime.Time)
				})

			finishedAt = containerFinishedAtTimes[len(containerFinishedAtTimes)-1]
		}

		jobResponse := &protos.PodResponse{
			Metadata:             &pod.ObjectMeta,
			Spec:                 &pod.Spec,
			Status:               &pod.Status,
			TerminatedContainers: terminatedContainers,
			FailedContainers:     failedContainers,
			Failed:               failed,
			Phase:                string(pod.Status.Phase),
			TerminationReasons:   failedContainers.TerminationReasons,
			FinishedAt:           finishedAt,
		}
		jobPodsResponses = append(jobPodsResponses, jobResponse)
	}

	return jobPodsResponses
}

func jobSucceded(job *batchv1.Job) bool {
	// The completion time is only set when the job finishes successfully.
	return job.Status.CompletionTime != nil
}

func buildLastTimes(cronJob batchv1.CronJob) (string, string) {
	var lastSuccessfulTime string
	var lastScheduleTime string
	if cronJob.Status.LastSuccessfulTime != nil {
		lastSuccessfulTime = cronJob.Status.LastSuccessfulTime.UTC().Format(time.RFC3339)
	}

	if cronJob.Status.LastScheduleTime != nil {
		lastScheduleTime = cronJob.Status.LastScheduleTime.UTC().Format(time.RFC3339)
	}

	return lastSuccessfulTime, lastScheduleTime
}

func buildCronJobCommand(cronJob batchv1.CronJob) map[string]*protos.ContainerCommands {
	// cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers
	// cronJob.Spec.JobTemplate.Spec.Template.Spec.InitContainers.Image
	// cronJob.Spec.JobTemplate.Spec.Template.Spec.InitContainers.Command
	// spec:
	//   backoffLimit:6
	//   commentmpletionMode: NonIndexed
	//   completions: 1
	//   parallelism: 1
	commands := make(map[string]*protos.ContainerCommands)
	n := len(cronJob.Spec.JobTemplate.Spec.Template.Spec.InitContainers)
	initContainersCommands := make([]string, 0, n)
	var command bytes.Buffer
	for _, container := range cronJob.Spec.JobTemplate.Spec.Template.Spec.InitContainers {
		for _, ccmd := range container.Command {
			command.WriteString(fmt.Sprintf("%s ", ccmd))
		}
		initContainersCommands = append(initContainersCommands, command.String())
		command.Reset()
	}

	containersCommands := make([]string, 0, n)
	for _, container := range cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers {
		for _, ccmd := range container.Command {
			command.WriteString(fmt.Sprintf("%s ", ccmd))
		}
		containersCommands = append(containersCommands, command.String())
		command.Reset()
	}

	ephemeralContainersinersCommands := make([]string, 0, n)
	for _, container := range cronJob.Spec.JobTemplate.Spec.Template.Spec.EphemeralContainers {
		for _, ccmd := range container.Command {
			command.WriteString(fmt.Sprintf("%s ", ccmd))
		}
		ephemeralContainersinersCommands = append(ephemeralContainersinersCommands, command.String())
		command.Reset()
	}

	commands["InitContainers"] = &protos.ContainerCommands{
		Commands: initContainersCommands,
	}
	commands["Containers"] = &protos.ContainerCommands{
		Commands: containersCommands,
	}
	commands["EphemeralContainers"] = &protos.ContainerCommands{
		Commands: ephemeralContainersinersCommands,
	}

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

func toDuration(job *batchv1.Job, jobFailed bool, failureCondition *batchv1.JobCondition) time.Duration {
	var d time.Duration

	status := job.Status
	if jobFailed && failureCondition != nil {
		d = failureCondition.LastTransitionTime.Sub(status.StartTime.Time)
		return d
	}

	if status.StartTime == nil {
		return d
	}

	switch {
	case status.CompletionTime == nil:
		d = time.Since(status.StartTime.Time)
	default:
		d = status.CompletionTime.Sub(status.StartTime.Time)
	}

	return d
	// return duration.HumanDuration(d)
}

func toDurationInS(job *batchv1.Job, jobFailed bool, failureCondition *batchv1.JobCondition) int64 {
	d := toDuration(job, jobFailed, failureCondition)

	return int64(d.Seconds())
}

func toCompletionTimeInS(job *batchv1.Job) int64 {
	if job.Status.CompletionTime != nil {
		return job.Status.CompletionTime.Unix()
	}

	return int64(0)
}
