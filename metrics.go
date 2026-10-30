package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/danroux/sk8l/protos"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
)

// metricsRefreshInterval is the periodic refresh used in addition to store change
// notifications, so time-derived values (e.g. the duration of running jobs) keep advancing.
const metricsRefreshInterval = streamHeartbeatInterval

var (
	metricNamespace = "sk8l"

	// TotalStatuses lists the status labels for summary metrics.
	TotalStatuses = []string{
		"registered",
		"running",
		"failing",
		"completed",
	}

	// cronjobsCountGauge tracks cluster/namespace overview counts by status.
	cronjobsCountGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "cronjobs_count",
			Help:      "Current count of cronjobs grouped by namespace and status (registered, running, failing, completed).",
		},
		[]string{"namespace", "status"},
	)

	// cronjobLastRunStatusGauge tracks the latest run status per cronjob.
	cronjobLastRunStatusGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "cronjob_last_run_status",
			Help:      "Status of the last execution of a cronjob (1 if true, 0 if false).",
		},
		[]string{"namespace", "cronjob", "status"},
	)

	// cronjobLastRunDurationGauge tracks the duration of the latest cronjob execution.
	cronjobLastRunDurationGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "cronjob_last_run_duration_seconds",
			Help:      "Duration in seconds of the latest execution of a cronjob.",
		},
		[]string{"namespace", "cronjob"},
	)

	// cronjobActiveJobsGauge tracks active child jobs per cronjob.
	cronjobActiveJobsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "cronjob_active_jobs",
			Help:      "Number of currently active child jobs for a cronjob.",
		},
		[]string{"namespace", "cronjob"},
	)

	// cronjobJobsCountGauge tracks the count of child jobs by status.
	cronjobJobsCountGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "cronjob_jobs_count",
			Help:      "Count of retained child jobs for a cronjob by state (succeeded, failed).",
		},
		[]string{"namespace", "cronjob", "status"},
	)

	// cronjobInfoGauge exports metadata and configuration for a cronjob.
	cronjobInfoGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "cronjob_info",
			Help:      "Metadata and configuration for a cronjob (constant value of 1).",
		},
		[]string{"namespace", "cronjob", "schedule", "suspend"},
	)
)

// countRetainedJobs calculates the number of succeeded and failed child jobs.
func countRetainedJobs(jobs []*protos.JobResponse) (succeeded, failed float64) {
	for _, job := range jobs {
		if job.Failed {
			failed++
		} else if job.Succeeded || (job.Status != nil && job.Status.CompletionTime != "") {
			succeeded++
		}
	}
	return succeeded, failed
}

// recordSingleCronjobMetrics updates per-cronjob gauge vectors and returns state flags.
func recordSingleCronjobMetrics(cj *protos.CronjobResponse, ns string) (running, failing, completed float64) {
	cronjobName := cj.Name

	// Active running state
	activeJobsCount := float64(len(cj.RunningJobs))
	cronjobActiveJobsGauge.WithLabelValues(ns, cronjobName).Set(activeJobsCount)
	if activeJobsCount > 0 {
		running = 1
	}

	// Failure / Success state
	if cj.Failed {
		failing = 1
		cronjobLastRunStatusGauge.WithLabelValues(ns, cronjobName, "failed").Set(1)
		cronjobLastRunStatusGauge.WithLabelValues(ns, cronjobName, "succeeded").Set(0)
	} else {
		cronjobLastRunStatusGauge.WithLabelValues(ns, cronjobName, "failed").Set(0)
		cronjobLastRunStatusGauge.WithLabelValues(ns, cronjobName, "succeeded").Set(1)
	}

	if !cj.Failed && activeJobsCount == 0 && cj.LastSuccessfulTime != "" {
		completed = 1
	}

	// Last execution duration
	if cj.LastDuration > 0 {
		cronjobLastRunDurationGauge.WithLabelValues(ns, cronjobName).Set(float64(cj.LastDuration))
	} else if cj.CurrentDuration > 0 {
		cronjobLastRunDurationGauge.WithLabelValues(ns, cronjobName).Set(float64(cj.CurrentDuration))
	}

	// Retained jobs count breakdown
	succeededJobs, failedJobs := countRetainedJobs(cj.Jobs)
	cronjobJobsCountGauge.WithLabelValues(ns, cronjobName, "succeeded").Set(succeededJobs)
	cronjobJobsCountGauge.WithLabelValues(ns, cronjobName, "failed").Set(failedJobs)

	// CronJob Info metadata
	schedule := ""
	suspend := "false"
	if cj.Spec != nil {
		schedule = cj.Spec.Schedule
		if cj.Spec.Suspend {
			suspend = "true"
		}
	}
	cronjobInfoGauge.WithLabelValues(ns, cronjobName, schedule, suspend).Set(1)

	return running, failing, completed
}

// ProcessCronjobsResponse aggregates status counts and updates all Prometheus metric vectors.
func processCronjobsResponse(cronjobs []*protos.CronjobResponse, subSystem string, cronjobsMap *sync.Map) {
	cronjobsCountGauge.Reset()
	cronjobLastRunStatusGauge.Reset()
	cronjobLastRunDurationGauge.Reset()
	cronjobActiveJobsGauge.Reset()
	cronjobJobsCountGauge.Reset()
	cronjobInfoGauge.Reset()

	var totalRegistered, totalRunning, totalFailing, totalCompleted float64
	currentCronjobs := make(map[string]bool)

	for _, cj := range cronjobs {
		ns := cj.Namespace
		if ns == "" {
			ns = subSystem
		}
		currentCronjobs[cj.Name] = true

		if cronjobsMap != nil {
			cronjobsMap.Store(cj.Name, true)
		}

		totalRegistered++
		running, failing, completed := recordSingleCronjobMetrics(cj, ns)
		totalRunning += running
		totalFailing += failing
		totalCompleted += completed
	}

	// Update summary gauges for namespace
	cronjobsCountGauge.WithLabelValues(subSystem, "registered").Set(totalRegistered)
	cronjobsCountGauge.WithLabelValues(subSystem, "running").Set(totalRunning)
	cronjobsCountGauge.WithLabelValues(subSystem, "failing").Set(totalFailing)
	cronjobsCountGauge.WithLabelValues(subSystem, "completed").Set(totalCompleted)

	// Clean up any deleted cronjobs from cronjobsMap
	if cronjobsMap != nil {
		cronjobsMap.Range(func(key, _ any) bool {
			name, ok := key.(string)
			if ok && !currentCronjobs[name] {
				cronjobsMap.Delete(key)
			}
			return true
		})
	}
}

// recordMetrics starts the in-process metrics collector in a background goroutine.
// It returns immediately; the collector stops when ctx is canceled.
func recordMetrics(ctx context.Context, svr *Sk8lServer, cronjobsMap *sync.Map) {
	log.Info().
		Str("component", "metrics").
		Str("operation", "recordMetrics").
		Msg("Starting metrics collection")

	go collectMetrics(ctx, svr, cronjobsMap, metricsRefreshInterval)
}

// collectMetrics refreshes the Prometheus gauges straight from the in-memory store.
// It recomputes once at start, on every store change notification and on every
// interval tick, and blocks until ctx is canceled.
func collectMetrics(ctx context.Context, svr *Sk8lServer, cronjobsMap *sync.Map, interval time.Duration) {
	defer func() {
		log.Info().
			Str("component", "metrics").
			Msg("Stopping metrics collection")
	}()

	st := svr.GetStore()
	if st == nil {
		log.Error().
			Err(ErrStoreNil).
			Str("operation", "collectMetrics").
			Msg("metrics collection disabled")
		return
	}

	subSystem := ""
	if cl := svr.GetK8sClient(); cl != nil {
		subSystem = cl.Namespace()
	}

	// Subscribe before the first refresh so a change between both steps is not missed.
	notifyCh := st.Subscribe(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	refresh := func() {
		cronjobs, err := svr.buildCronjobResponses(ctx, st)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Error().
					Err(err).
					Str("operation", "collectMetrics").
					Msg("buildCronjobResponses failed")
			}
			return
		}
		processCronjobsResponse(cronjobs, subSystem, cronjobsMap)
	}

	refresh()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-notifyCh:
			if !ok {
				return
			}
			refresh()
		case <-ticker.C:
			refresh()
		}
	}
}
