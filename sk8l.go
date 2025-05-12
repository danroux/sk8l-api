package main

import (
	"bytes"
	"cmp"
	"context"
	"embed"
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
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"

	// structpb "google.golang.org/protobuf/types/known/structpb".
	"google.golang.org/grpc"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
)

type Sk8lServer struct {
	grpc_health_v1.UnimplementedHealthServer
	protos.UnimplementedCronjobServer
	Target  string
	Options []grpc.DialOption
	// CronjobDBStore CronjobStore
	*CronjobDBStore
}

type APICall (func() []byte)

func (s Sk8lServer) Check(
	ctx context.Context,
	req *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	log.Default().Println("serving health")
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (s Sk8lServer) Watch(req *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error {
	response := &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}

	if err := stream.Send(response); err != nil {
		log.Println("Error: Watch#stream.Send", err)
		return err
	}

	return nil
}

func (s *Sk8lServer) Run(metricsCxt context.Context) {
	s.collectCronjobs()
	s.collectJobs()
	s.collectPods()
	recordMetrics(metricsCxt, s)
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
		// https://grpc.io/docs/guides/cancellation/
		// https://learn.microsoft.com/en-us/aspnet/core/grpc/performance?view=aspnetcore-9.0
		case <-stream.Context().Done():
			err := stream.Context().Err()
			log.Printf("stream context done: client canceled or deadline exceeded: %v", err)
			return err
		default:
			if err := stream.Send(y); err != nil {
				return err
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
			return err
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
			return err
		}

		time.Sleep(refreshInterval)
	}
}

func (s *Sk8lServer) GetCronjobYAML(
	ctx context.Context,
	in *protos.CronjobRequest,
) (*protos.CronjobYAMLResponse, error) {
	cronjob := s.K8sClient.GetCronjob(in.CronjobNamespace, in.CronjobName)
	prettyJSON, _ := json.MarshalIndent(cronjob, "", "  ")

	y, _ := gyaml.JSONToYAML(prettyJSON)

	response := &protos.CronjobYAMLResponse{
		Cronjob: string(y),
	}

	return response, nil
}

func (s *Sk8lServer) GetJobYAML(ctx context.Context, in *protos.JobRequest) (*protos.JobYAMLResponse, error) {
	job := s.K8sClient.GetJob(in.JobNamespace, in.JobName)
	prettyJSON, _ := json.MarshalIndent(job, "", "  ")

	y, _ := gyaml.JSONToYAML(prettyJSON)

	response := &protos.JobYAMLResponse{
		Job: string(y),
	}

	return response, nil
}

func (s *Sk8lServer) GetPodYAML(ctx context.Context, in *protos.PodRequest) (*protos.PodYAMLResponse, error) {
	pod := s.K8sClient.GetPod(in.PodNamespace, in.PodName)
	prettyJSON, _ := json.MarshalIndent(pod, "", "  ")

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
	panels := generatePanels()

	var tmplFile = "annotations.tmpl"
	t := template.New(tmplFile)
	// t = t.Funcs(template.FuncMap{"StringsJoin": strings.Join})
	t = t.Funcs(template.FuncMap{"marshal": func(v any) string {
		a, _ := json.Marshal(v)
		return string(a)
	},
	},
	)
	t = template.Must(t.ParseFS(content, tmplFile))

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

func (s *Sk8lServer) findJobsMapped() *protos.MappedJobs {
	jobsCall := func() []byte {
		result := s.K8sClient.GetAllJobsMapped()
		value, _ := proto.Marshal(result)
		return value
	}

	jobs, err := s.getAndStore(jobsMappedCacheKey, jobsCall)

	if err != nil {
		log.Println("Error: findJobsMapped#s.getAndStore", err)
	}

	mappedJobs := &protos.MappedJobs{}

	err = proto.Unmarshal(jobs, mappedJobs)

	if err != nil {
		log.Println("Error: findJobsMapped#proton.Unmarshal", err)
	}

	return mappedJobs
}

func (s *Sk8lServer) findJobPodsForJob(job *batchv1.Job) *corev1.PodList {
	fKey := fmt.Sprintf(jobPodsKeyFmt, job.Name)
	key := []byte(fKey)
	collection := &corev1.PodList{}
	collectionV2 := protoadapt.MessageV2Of(collection)

	err := s.DB.View(func(txn *badger.Txn) error {
		current, err := txn.Get(key)

		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}

		err = current.Value(func(val []byte) error {
			err = proto.Unmarshal(val, collectionV2)

			if err != nil {
				log.Println("findJobPodsForJob#proto.Unmarshal", err)
				return err
			}

			return nil
		})

		if err != nil {
			log.Println("Error: findJobPodsForJob#current.Value", err)
			return err
		}

		return nil
	})

	if err != nil {
		log.Println("Error: findJobPodsForJob#DB.View", err)
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

	podList := &corev1.PodList{
		Items: podItems,
	}

	return podList
}

// Revisit this. JobConditions are not being used yet anywhere.
// PodResponse.TerminationReasons.TerminationDetails -> ContainerStateTerminated.
func jobFailed(
	job *batchv1.Job,
	jobPodsResponses []*protos.PodResponse,
) (bool, *batchv1.JobCondition, []*batchv1.JobCondition) {
	var jobFailed bool
	var failureCondition *batchv1.JobCondition

	n := len(job.Status.Conditions)
	jobConditions := make([]*batchv1.JobCondition, 0, n)
	for i := range job.Status.Conditions {
		jobCondition := job.Status.Conditions[i]
		if !jobFailed {
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

func (s *Sk8lServer) jobWithSidecarContainer(batchJob *batchv1.Job) bool {
	for _, container := range batchJob.Spec.Template.Spec.InitContainers {
		if container.RestartPolicy != nil && corev1.ContainerRestartPolicy(*container.RestartPolicy) == corev1.ContainerRestartPolicyAlways {
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
					log.Println("Error: event.Object.(*batchv1.CronJob)")
				}

				err := s.DB.Update(func(txn *badger.Txn) error {
					item, err := txn.Get(cronjobsCacheKey)
					if err != nil {
						cjList := &batchv1.CronJobList{
							Items: []batchv1.CronJob{*eventCronjob},
						}

						cjListV2 := protoadapt.MessageV2Of(cjList)
						result, _ := proto.Marshal(cjListV2)

						entry := badger.NewEntry(cronjobsCacheKey, result)
						err = txn.SetEntry(entry)
						if err != nil {
							log.Println("Error: collectCronjobs#txn.SetEntry", err)
						}
						return err
					}

					err = item.Value(func(stored []byte) error {
						storedCjList := &batchv1.CronJobList{}

						storedCjListV2 := protoadapt.MessageV2Of(storedCjList)
						err = proto.Unmarshal(stored, storedCjListV2)

						if err != nil {
							log.Println("Error: collectCronjobs#proto.Unmarshal", err)
						}

						switch event.Type {
						case "ADDED":
							updateStoredCronjobList(storedCjList, eventCronjob)
							storedCjList.Items = append(storedCjList.Items, *eventCronjob)
						case "MODIFIED":
							updateStoredCronjobList(storedCjList, eventCronjob)
							storedCjList.Items = append(storedCjList.Items, *eventCronjob)
						case "DELETED":
							updateStoredCronjobList(storedCjList, eventCronjob)
						}

						result, err := proto.Marshal(storedCjListV2)
						if err != nil {
							log.Println("Error: collectCronjobs#proto.Marshal", err)
						}

						entry := badger.NewEntry(cronjobsCacheKey, result)
						err = txn.SetEntry(entry)
						if err != nil {
							log.Println("Error: collectCronjobs#txn.SetEntry", err)
						}
						return err
					})

					return err
				})

				if err != nil {
					panic(err)
				}
			} else {
				x = s.K8sClient.WatchCronjobs()
				log.Println("Cronjob watching: Received all Cronjobs. Opening again")
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
					log.Println("Error: event.Object.(*batchv1.Job)")
				}

				err := s.DB.Update(func(txn *badger.Txn) error {
					item, err := txn.Get(jobsCacheKey)
					if err != nil {
						jList := &batchv1.JobList{
							Items: []batchv1.Job{*eventJob},
						}

						jListV2 := protoadapt.MessageV2Of(jList)
						result, _ := proto.Marshal(jListV2)

						entry := badger.NewEntry(jobsCacheKey, result)
						err = txn.SetEntry(entry)
						if err != nil {
							log.Println("Error: collectJobs#txn.SetEntry", err)
						}
						return err
					}

					err = item.Value(func(stored []byte) error {
						storedJList := &batchv1.JobList{}

						storedJListV2 := protoadapt.MessageV2Of(storedJList)
						err = proto.Unmarshal(stored, storedJListV2)

						if err != nil {
							log.Println("Error: collectJobs#proto.Unmarshal", err)
						}

						switch event.Type {
						case "ADDED":
							updateStoredJobList(storedJList, eventJob)
							storedJList.Items = append(storedJList.Items, *eventJob)
						case "MODIFIED":
							updateStoredJobList(storedJList, eventJob)
							storedJList.Items = append(storedJList.Items, *eventJob)
						case "DELETED":
							updateStoredJobList(storedJList, eventJob)
						}

						result, err := proto.Marshal(storedJListV2)
						if err != nil {
							log.Println("Error: collectJobs#proto.Marshal", err)
						}

						entry := badger.NewEntry(jobsCacheKey, result)
						err = txn.SetEntry(entry)
						if err != nil {
							log.Println("Error: collectJobs#txn.SetEntry", err)
						}
						return err
					})

					return err
				})

				if err != nil {
					panic(err)
				}
			} else {
				x = s.K8sClient.WatchJobs()
				log.Println("Job watching: Received all Jobs. Opening again")
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
					log.Println("Error: event.Object.(*corev1.Pod)")
				}

				fKey := fmt.Sprintf(jobPodsKeyFmt, eventPod.Labels["job-name"])
				key := []byte(fKey)
				err := s.DB.Update(func(txn *badger.Txn) error {
					item, err := txn.Get(key)
					if err != nil {
						podList := &corev1.PodList{
							Items: []corev1.Pod{*eventPod},
						}

						podListV2 := protoadapt.MessageV2Of(podList)
						result, err := proto.Marshal(podListV2)

						if err != nil {
							log.Println("Error: collectPods#proto.Marshal", err)
						}

						entry := badger.NewEntry(key, result)
						err = txn.SetEntry(entry)
						return err
					}

					err = item.Value(func(val []byte) error {
						podList := &corev1.PodList{}
						podListV2 := protoadapt.MessageV2Of(podList)
						err = proto.Unmarshal(val, podListV2)

						if err != nil {
							log.Println("Error: collectPods#proto.Unmarshal", err)
						}

						podList.Items = append(podList.Items, *eventPod)
						result, err := proto.Marshal(podListV2)

						if err != nil {
							log.Println("Error: collectPods#proto.Marshal", err)
						}

						entry := badger.NewEntry(key, result)
						err = txn.SetEntry(entry)
						if err != nil {
							log.Println("Error: collectCronjobs#txn.SetEntry", err)
						}
						return err
					})

					return err
				})

				if err != nil {
					panic(err)
				}
			} else {
				x = s.K8sClient.WatchPods()
				log.Println("Job watching: Received all Pods. Opening again", x)
			}
		}
	}()
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

func collectTerminatedAndFailedContainers(
	pod *corev1.Pod,
	statuses []corev1.ContainerStatus,
	terminationReasons *[]*protos.TerminationReason,
) (terminatedContainers []*protos.ContainerResponse, failedContainers []*protos.ContainerResponse) {
	terminatedContainers = make([]*protos.ContainerResponse, 0)
	failedContainers = make([]*protos.ContainerResponse, 0)

	for _, containerStatus := range statuses {
		// ephStates = append(ephStates, container.State)
		// if container.State.Waiting != nil && container.State.Waiting.Reason == "Error" {
		//      failedEphContainers = append(failedEphContainers, &container)
		// }

		podConditions := []*corev1.PodCondition{}
		for _, pc := range pod.Status.Conditions {
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

	// ephStates := make([]corev1.ContainerState, 0)
	for _, pod := range gJobPods.Items {
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

		podResponse := &protos.PodResponse{
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
		jobPodsResponses = append(jobPodsResponses, podResponse)
	}

	return jobPodsResponses
}

func jobSucceeded(job *batchv1.Job) bool {
	// The completion time is only set when the job finishes successfully.
	return job.Status.CompletionTime != nil
}

func buildLastTimes(cronJob batchv1.CronJob) (lastSuccessfulTime string, lastScheduleTime string) {
	// var lastSuccessfulTime string
	// var lastScheduleTime string
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
			_, err := command.WriteString(fmt.Sprintf("%s ", ccmd))
			if err != nil {
				log.Println("Error: buildCronJobCommand#command.WriteString")
			}
		}
		initContainersCommands = append(initContainersCommands, command.String())
		command.Reset()
	}

	containersCommands := make([]string, 0, n)
	for _, container := range cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers {
		for _, ccmd := range container.Command {
			_, err := command.WriteString(fmt.Sprintf("%s ", ccmd))
			if err != nil {
				log.Println("Error: buildCronJobCommand#command.WriteString")
			}
		}
		containersCommands = append(containersCommands, command.String())
		command.Reset()
	}

	ephemeralContainersinersCommands := make([]string, 0, n)
	for _, container := range cronJob.Spec.JobTemplate.Spec.Template.Spec.EphemeralContainers {
		for _, ccmd := range container.Command {
			_, err := command.WriteString(fmt.Sprintf("%s ", ccmd))
			if err != nil {
				log.Println("Error: buildCronJobCommand#command.WriteString")
			}
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

func updateStoredCronjobList(storedCjList *batchv1.CronJobList, eventCronjob *batchv1.CronJob) {
	cjList := &batchv1.CronJobList{}
	// to avoid duplicates if the process is restarted and on "MODIFIED" to get the updated version of the resource
	for _, cronjob := range storedCjList.Items {
		if cronjob.Name != eventCronjob.Name {
			cjList.Items = append(cjList.Items, cronjob)
		}
	}
	storedCjList.Items = cjList.Items
}

func updateStoredJobList(storedJList *batchv1.JobList, eventJob *batchv1.Job) {
	jList := &batchv1.JobList{}
	// to avoid duplicates if the process is restarted and on "MODIFIED" to get the updated version of the resource
	for _, job := range storedJList.Items {
		if job.Name != eventJob.Name {
			jList.Items = append(jList.Items, job)
		}
	}
	storedJList.Items = jList.Items
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
