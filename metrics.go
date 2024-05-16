package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/danroux/sk8l/protos"
	"google.golang.org/grpc"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var metricsNamesMap = &sync.Map{}

var (
	namespace           = os.Getenv("K8_NAMESPACE")
	optNamespace        = "sk8l"
	summaryMap          = &sync.Map{}
	failingCronjobsOpts = prometheus.GaugeOpts{
		Namespace: optNamespace,
		Name:      "failing_cronjobs_total",
		Subsystem: namespace,
	}
	runningCronjobsOpts = prometheus.GaugeOpts{
		Namespace: optNamespace,
		Name:      "running_cronjobs_total",
		Subsystem: namespace,
	}
	completedCronjobsOpts = prometheus.GaugeOpts{
		Namespace: optNamespace,
		Name:      "completed_cronjobs_total",
		Subsystem: namespace,
	}
	registeredCronjobsOpts = prometheus.GaugeOpts{
		Namespace: optNamespace,
		Name:      "registered_cronjobs_total",
		Subsystem: namespace,
	}

	failingCronjobsGauge    = promauto.NewGauge(failingCronjobsOpts)
	runningCronjobsGauge    = promauto.NewGauge(runningCronjobsOpts)
	completedCronjobsGauge  = promauto.NewGauge(completedCronjobsOpts)
	registeredCronjobsGauge = promauto.NewGauge(registeredCronjobsOpts)

	cronjobFailingJobs     float64
	cronjobCompletions     float64
	completedCronjobs      float64
	jobDuration            float64
	failingJobs            float64
	runningCronjobs        float64
	cronjobCompletionsOpts prometheus.GaugeOpts
	cronjobDurationOpts    prometheus.GaugeOpts
	failingJobsOpts        prometheus.GaugeOpts
	completionsKey         string
	durationKey            string
	failuresKey            string
	metricNameRegex        = regexp.MustCompile(`_*[^0-9A-Za-z_]+_*`)
)

func recordMetrics(ctx context.Context, svr *Sk8lServer) {
	conn, err := grpc.NewClient(svr.Target, svr.Options...)
	if err != nil {
		panic(fmt.Sprintf("grpc.NewClient(%s) failed: %v", svr.Target, err))
	}

	c := protos.NewCronjobClient(conn)
	subSystem := svr.K8sClient.namespace

	log.Println("Metrics: Starting metrics collection")
	req := &protos.CronjobsRequest{}
	cronjobsClient, err := c.GetCronjobs(ctx, req)

	if err != nil {
		panic(err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("Metrics: Shutdown - Stopping metrics collection")
				return
			default:
				cronjobsResponse, err := cronjobsClient.Recv()

				if err == io.EOF {
					break
				}

				if err != nil {
					log.Printf("Error: %v.GetCronjobs(_) = _, %v\n", cronjobsClient, err)
				}

				registeredCronjobs := len(cronjobsResponse.Cronjobs)
				registeredCronjobsGauge.Set(float64(registeredCronjobs))
				var metricNames []string

				for _, cj := range cronjobsResponse.Cronjobs {
					sanitizedCjName := sanitizeMetricName(cj.Name)
					runningCronjobs += float64(len(cj.RunningJobs))

					completionMetricName := fmt.Sprintf("%s_completion_total", sanitizedCjName)
					failureMetricName := fmt.Sprintf("%s_failure_total", sanitizedCjName)
					durationMetricName := fmt.Sprintf("%s_duration_seconds", sanitizedCjName)

					metricNames = []string{
						fmt.Sprintf("%s_%s", MetricPrefix, completionMetricName),
						fmt.Sprintf("%s_%s", MetricPrefix, failureMetricName),
						fmt.Sprintf("%s_%s", MetricPrefix, durationMetricName),
					}
					metricsNamesMap.Store(sanitizedCjName, metricNames)

					for _, job := range cj.Jobs {
						if job.Failed {
							cronjobFailingJobs++
						}

						if job.Status.CompletionTime != nil {
							cronjobCompletions++
						}

						sanitizedJobName := job.Name
						labels := prometheus.Labels{}
						labels["job_name"] = sanitizedJobName
						cronjobDurationOpts = prometheus.GaugeOpts{
							Name:        durationMetricName,
							Namespace:   optNamespace,
							Subsystem:   subSystem,
							Help:        fmt.Sprintf("Duration of %s in seconds", sanitizedCjName),
							ConstLabels: labels,
						}
						durationKey = fmt.Sprintf(
							"%s_%s_%s_%s_durations",
							cronjobDurationOpts.Namespace,
							cronjobDurationOpts.Subsystem,
							sanitizedCjName,
							sanitizedJobName,
						)

						if *job.Status.Active > 0 {
							jobDuration = float64(job.DurationInS)
						} else {
							jobDuration = float64(0)
						}

						if jobsDurationssGauge, ok := summaryMap.Load(durationKey); ok {
							jobsDurationssGauge.(prometheus.Gauge).Set(jobDuration)
						} else {
							jobsDurationssGauge := promauto.NewGauge(cronjobDurationOpts)
							summaryMap.Store(durationKey, jobsDurationssGauge)
							jobsDurationssGauge.Set(jobDuration)
						}
					}

					cronjobCompletionsOpts = prometheus.GaugeOpts{
						Name:      completionMetricName,
						Namespace: optNamespace,
						Subsystem: subSystem,
						Help:      fmt.Sprintf("%s completion total", sanitizedCjName),
					}

					completionsKey = fmt.Sprintf(
						"%s_%s_%s_completions",
						cronjobCompletionsOpts.Namespace,
						cronjobCompletionsOpts.Subsystem,
						sanitizedCjName,
					)

					if cronjobCompletionsGauge, ok := summaryMap.Load(completionsKey); ok {
						cronjobCompletionsGauge.(prometheus.Gauge).Set(cronjobCompletions)
					} else {
						cronjobCompletionsGauge := promauto.NewGauge(cronjobCompletionsOpts)
						summaryMap.Store(completionsKey, cronjobCompletionsGauge)
						cronjobCompletionsGauge.Set(cronjobCompletions)
					}

					failingJobsOpts = prometheus.GaugeOpts{
						Name:      failureMetricName,
						Namespace: optNamespace,
						Subsystem: subSystem,
						Help:      fmt.Sprintf("%s failure total", sanitizedCjName),
					}

					failuresKey = fmt.Sprintf(
						"%s_%s_%s_failures",
						failingJobsOpts.Namespace,
						failingJobsOpts.Subsystem,
						sanitizedCjName,
					)

					if failingJobsGauge, ok := summaryMap.Load(failuresKey); ok {
						failingJobsGauge.(prometheus.Gauge).Set(cronjobFailingJobs)
					} else {
						failingJobsGauge := promauto.NewGauge(failingJobsOpts)
						summaryMap.Store(failuresKey, failingJobsGauge)
						failingJobsGauge.Set(cronjobFailingJobs)
					}

					failingJobs += cronjobFailingJobs
					completedCronjobs += cronjobCompletions
					cronjobFailingJobs = 0
					cronjobCompletions = 0
				}

				runningCronjobsGauge.Set(runningCronjobs)
				failingCronjobsGauge.Set(failingJobs)
				completedCronjobsGauge.Set(completedCronjobs)

				failingJobs = 0
				runningCronjobs = 0
				completedCronjobs = 0
				time.Sleep(10 * time.Second)
			}
		}
	}()
}

// https://github.com/prometheus/node_exporter/blob/4a1b77600c1873a8233f3ffb55afcedbb63b8d84/collector/helper.go#L48
func sanitizeMetricName(metricName string) string {
	return metricNameRegex.ReplaceAllString(metricName, "_")
}
