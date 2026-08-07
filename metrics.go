package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/danroux/sk8l/protos"
	"google.golang.org/grpc"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
)

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

	metricNameRegex = regexp.MustCompile(`_*[^0-9A-Za-z_]+_*`)

	TotalMetricNames = []string{
		registeredCronjobsOpts.Name,
		completedCronjobsOpts.Name,
		runningCronjobsOpts.Name,
		failingCronjobsOpts.Name,
	}
)

func setGaugeInMap(key string, opts prometheus.GaugeOpts, val float64) {
	if gauge, ok := summaryMap.Load(key); ok {
		gauge.(prometheus.Gauge).Set(val)
		return
	}
	newGauge := promauto.NewGauge(opts)
	summaryMap.Store(key, newGauge)
	newGauge.Set(val)
}

// Computes job duration and sets the per-job duration gauge.
func recordJobDuration(job *protos.JobResponse, sanitizedCjName, durationMetricName, subSystem string) (isFailed bool, isCompleted bool) {
	if job.Failed {
		isFailed = true
	}
	if job.Status != nil && job.Status.CompletionTime != "" {
		isCompleted = true
	}

	sanitizedJobName := job.Name
	labels := prometheus.Labels{"job_name": sanitizedJobName}
	opts := prometheus.GaugeOpts{
		Name:        durationMetricName,
		Namespace:   optNamespace,
		Subsystem:   subSystem,
		Help:        fmt.Sprintf("Duration of %s in seconds", sanitizedCjName),
		ConstLabels: labels,
	}
	durationKey := fmt.Sprintf(
		"%s_%s_%s_%s_durations",
		opts.Namespace,
		opts.Subsystem,
		sanitizedCjName,
		sanitizedJobName,
	)

	var duration float64
	if job.Status != nil && job.Status.Active > 0 {
		duration = float64(job.DurationInS)
	}
	setGaugeInMap(durationKey, opts, duration)
	return isFailed, isCompleted
}

// Sets the completion and failure gauges for a specific cronjob and stores metric names in metricsNamesMap.
func recordSingleCronjobMetrics(
	cj *protos.CronjobResponse,
	subSystem string,
	metricsNamesMap *sync.Map,
) (running float64, failing float64, completed float64) {
	sanitizedCjName := sanitizeMetricName(cj.Name)
	running = float64(len(cj.RunningJobs))

	completionMetricName := fmt.Sprintf("%s_completion_total", sanitizedCjName)
	failureMetricName := fmt.Sprintf("%s_failure_total", sanitizedCjName)
	durationMetricName := fmt.Sprintf("%s_duration_seconds", sanitizedCjName)

	metricNames := []string{
		fmt.Sprintf("%s_%s", MetricPrefix, completionMetricName),
		fmt.Sprintf("%s_%s", MetricPrefix, failureMetricName),
		fmt.Sprintf("%s_%s", MetricPrefix, durationMetricName),
	}
	metricsNamesMap.Store(sanitizedCjName, metricNames)

	var cronjobFailingJobs, cronjobCompletions float64
	for _, job := range cj.Jobs {
		isFailed, isCompleted := recordJobDuration(job, sanitizedCjName, durationMetricName, subSystem)
		if isFailed {
			cronjobFailingJobs++
		}
		if isCompleted {
			cronjobCompletions++
		}
	}

	completionOpts := prometheus.GaugeOpts{
		Name:      completionMetricName,
		Namespace: optNamespace,
		Subsystem: subSystem,
		Help:      fmt.Sprintf("%s completion total", sanitizedCjName),
	}
	completionsKey := fmt.Sprintf(
		"%s_%s_%s_completions",
		completionOpts.Namespace,
		completionOpts.Subsystem,
		sanitizedCjName,
	)
	setGaugeInMap(completionsKey, completionOpts, cronjobCompletions)

	failureOpts := prometheus.GaugeOpts{
		Name:      failureMetricName,
		Namespace: optNamespace,
		Subsystem: subSystem,
		Help:      fmt.Sprintf("%s failure total", sanitizedCjName),
	}
	failuresKey := fmt.Sprintf(
		"%s_%s_%s_failures",
		failureOpts.Namespace,
		failureOpts.Subsystem,
		sanitizedCjName,
	)
	setGaugeInMap(failuresKey, failureOpts, cronjobFailingJobs)

	return running, cronjobFailingJobs, cronjobCompletions
}

// Aggregates totals (running, failing, completed, registered) and updates the global gauges.
func processCronjobsResponse(cronjobs []*protos.CronjobResponse, subSystem string, metricsNamesMap *sync.Map) {
	registeredCronjobsGauge.Set(float64(len(cronjobs)))

	var totalRunning, totalFailing, totalCompleted float64
	for _, cj := range cronjobs {
		running, failing, completed := recordSingleCronjobMetrics(cj, subSystem, metricsNamesMap)
		totalRunning += running
		totalFailing += failing
		totalCompleted += completed
	}

	runningCronjobsGauge.Set(totalRunning)
	failingCronjobsGauge.Set(totalFailing)
	completedCronjobsGauge.Set(totalCompleted)
}

func collectMetricsStream(ctx context.Context, c protos.CronjobClient, subSystem string, metricsNamesMap *sync.Map) error {
	cronjobsClient, err := c.GetCronjobs(ctx, &protos.CronjobsRequest{})
	if err != nil {
		return fmt.Errorf("c.GetCronjobs failed: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("metrics collection canceled: %w", ctx.Err())
		default:
		}

		cronjobsResponse, err := cronjobsClient.Recv()
		if errors.Is(err, io.EOF) {
			log.Info().
				Str("component", "metrics").
				Msg("GetCronjobs stream closed (EOF), reconnecting")
			return nil
		}
		if err != nil {
			return fmt.Errorf("cronjobsClient.Recv() failed: %w", err)
		}
		if cronjobsResponse == nil {
			continue
		}

		processCronjobsResponse(cronjobsResponse.Cronjobs, subSystem, metricsNamesMap)

		select {
		case <-ctx.Done():
			return fmt.Errorf("metrics collection canceled: %w", ctx.Err())
		case <-time.After(10 * time.Second):
		}
	}
}

func recordMetrics(ctx context.Context, svr *Sk8lServer, metricsNamesMap *sync.Map) {
	conn, err := grpc.NewClient(svr.GetTarget(), svr.GetDialOptions()...)
	if err != nil {
		log.Error().
			Err(err).
			Str("operation", "recordMetrics").
			Msg(fmt.Sprintf("grpc.NewClient(%s) failed", svr.GetTarget()))
		return
	}

	c := protos.NewCronjobClient(conn)
	subSystem := svr.K8sClient.Namespace()

	log.Info().
		Str("component", "metrics").
		Str("operation", "recordMetrics").
		Msg("Starting metrics collection")

	go func() {
		defer func() {
			_ = conn.Close()
		}()

		for {
			select {
			case <-ctx.Done():
				log.Info().
					Str("component", "metrics").
					Msg("Stopping metrics collection")
				return
			default:
			}

			if err := collectMetricsStream(ctx, c, subSystem, metricsNamesMap); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Error().
					Err(err).
					Str("operation", "recordMetrics").
					Msg("metrics collection error, retrying in 5s")
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
		}
	}()
}

// https://github.com/prometheus/node_exporter/blob/4a1b77600c1873a8233f3ffb55afcedbb63b8d84/collector/helper.go#L48
func sanitizeMetricName(metricName string) string {
	return metricNameRegex.ReplaceAllString(metricName, "_")
}
