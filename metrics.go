package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/danroux/sk8l/internal/store"
	"github.com/danroux/sk8l/protos"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// metricsRefreshInterval is the periodic refresh used in addition to store change
// notifications, so time-derived values (e.g. the duration of running jobs) keep advancing.
const (
	metricsRefreshInterval = streamHeartbeatInterval

	metricNamespace      = "sk8l"
	metricLabelNamespace = "namespace"
	metricLabelCronjob   = "cronjob"
	metricLabelStatus    = "status"
	metricStatusFailed   = "failed"
	metricStatusSuccess  = "succeeded"
	metricValueActive    = 1
	metricValueInactive  = 0
)

var (
	// TotalStatuses lists the status labels for summary metrics.
	TotalStatuses = []string{
		"registered",
		"running",
		"failing",
		"completed",
	}

	cronjobsCountDesc = prometheus.NewDesc(
		prometheus.BuildFQName(metricNamespace, "", "cronjobs_count"),
		"Current count of cronjobs grouped by namespace and status (registered, running, failing, completed).",
		[]string{metricLabelNamespace, metricLabelStatus},
		nil,
	)

	cronjobLastRunStatusDesc = prometheus.NewDesc(
		prometheus.BuildFQName(metricNamespace, "", "cronjob_last_run_status"),
		"Status of the last execution of a cronjob (1 if true, 0 if false).",
		[]string{metricLabelNamespace, metricLabelCronjob, metricLabelStatus},
		nil,
	)

	cronjobLastRunDurationDesc = prometheus.NewDesc(
		prometheus.BuildFQName(metricNamespace, "", "cronjob_last_run_duration_seconds"),
		"Duration in seconds of the latest execution of a cronjob.",
		[]string{metricLabelNamespace, metricLabelCronjob},
		nil,
	)

	cronjobActiveJobsDesc = prometheus.NewDesc(
		prometheus.BuildFQName(metricNamespace, "", "cronjob_active_jobs"),
		"Number of currently active child jobs for a cronjob.",
		[]string{metricLabelNamespace, metricLabelCronjob},
		nil,
	)

	cronjobJobsCountDesc = prometheus.NewDesc(
		prometheus.BuildFQName(metricNamespace, "", "cronjob_jobs_count"),
		"Count of retained child jobs for a cronjob by state (succeeded, failed).",
		[]string{metricLabelNamespace, metricLabelCronjob, metricLabelStatus},
		nil,
	)

	cronjobInfoDesc = prometheus.NewDesc(
		prometheus.BuildFQName(metricNamespace, "", "cronjob_info"),
		"Metadata and configuration for a cronjob (constant value of 1).",
		[]string{metricLabelNamespace, metricLabelCronjob, "schedule", "suspend"},
		nil,
	)

	defaultMetricsCollector = newRegisteredMetricsCollector(prometheus.DefaultRegisterer)
)

type metricsSnapshot struct {
	metrics []prometheus.Metric
}

// metricsCollector implements prometheus.Collector using lock-free atomic snapshot swaps
// so concurrent Prometheus scrapes always observe a complete, consistent point-in-time set.
type metricsCollector struct {
	snapshot atomic.Pointer[metricsSnapshot]
}

func newRegisteredMetricsCollector(reg prometheus.Registerer) *metricsCollector {
	c := &metricsCollector{}
	if reg != nil {
		reg.MustRegister(c)
	}
	return c
}

// Describe sends the descriptor for every cronjob metric to ch.
func (c *metricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- cronjobsCountDesc
	ch <- cronjobLastRunStatusDesc
	ch <- cronjobLastRunDurationDesc
	ch <- cronjobActiveJobsDesc
	ch <- cronjobJobsCountDesc
	ch <- cronjobInfoDesc
}

// Collect emits all metrics from the latest atomic snapshot. Before initial informer sync,
// the snapshot is nil so /metrics never publishes misleading zero counts.
func (c *metricsCollector) Collect(ch chan<- prometheus.Metric) {
	snap := c.snapshot.Load()
	if snap == nil {
		return
	}
	for _, m := range snap.metrics {
		ch <- m
	}
}

type syncWaiter interface {
	WaitForSync(ctx context.Context) bool
}

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

// buildSingleCronjobMetrics constructs per-cronjob const metrics and returns state flags.
func buildSingleCronjobMetrics(cj *protos.CronjobResponse, ns string) (metrics []prometheus.Metric, running, failing, completed float64) {
	cronjobName := cj.Name

	// Active running state
	activeJobsCount := float64(len(cj.RunningJobs))
	metrics = append(metrics, prometheus.MustNewConstMetric(
		cronjobActiveJobsDesc,
		prometheus.GaugeValue,
		activeJobsCount,
		ns,
		cronjobName,
	))
	if activeJobsCount > 0 {
		running = metricValueActive
	}

	// Failure / Success state
	failedVal := float64(metricValueInactive)
	succeededVal := float64(metricValueActive)
	if cj.Failed {
		failing = metricValueActive
		failedVal = metricValueActive
		succeededVal = metricValueInactive
	}
	metrics = append(metrics,
		prometheus.MustNewConstMetric(cronjobLastRunStatusDesc, prometheus.GaugeValue, failedVal, ns, cronjobName, metricStatusFailed),
		prometheus.MustNewConstMetric(cronjobLastRunStatusDesc, prometheus.GaugeValue, succeededVal, ns, cronjobName, metricStatusSuccess),
	)

	if !cj.Failed && activeJobsCount == 0 && cj.LastSuccessfulTime != "" {
		completed = metricValueActive
	}

	// Last execution duration
	if cj.LastDuration > 0 {
		metrics = append(metrics, prometheus.MustNewConstMetric(
			cronjobLastRunDurationDesc,
			prometheus.GaugeValue,
			float64(cj.LastDuration),
			ns,
			cronjobName,
		))
	} else if cj.CurrentDuration > 0 {
		metrics = append(metrics, prometheus.MustNewConstMetric(
			cronjobLastRunDurationDesc,
			prometheus.GaugeValue,
			float64(cj.CurrentDuration),
			ns,
			cronjobName,
		))
	}

	// Retained jobs count breakdown
	succeededJobs, failedJobs := countRetainedJobs(cj.Jobs)
	metrics = append(metrics,
		prometheus.MustNewConstMetric(cronjobJobsCountDesc, prometheus.GaugeValue, succeededJobs, ns, cronjobName, metricStatusSuccess),
		prometheus.MustNewConstMetric(cronjobJobsCountDesc, prometheus.GaugeValue, failedJobs, ns, cronjobName, metricStatusFailed),
	)

	// CronJob Info metadata
	schedule := ""
	suspend := "false"
	if cj.Spec != nil {
		schedule = cj.Spec.Schedule
		if cj.Spec.Suspend {
			suspend = "true"
		}
	}
	metrics = append(metrics, prometheus.MustNewConstMetric(
		cronjobInfoDesc,
		prometheus.GaugeValue,
		metricValueActive,
		ns,
		cronjobName,
		schedule,
		suspend,
	))

	return metrics, running, failing, completed
}

// buildMetricsSnapshot aggregates status counts into an immutable Prometheus metric slice.
func buildMetricsSnapshot(cronjobs []*protos.CronjobResponse, subSystem string, cronjobsMap *sync.Map) *metricsSnapshot {
	metrics := make([]prometheus.Metric, 0)
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
		cjMetrics, running, failing, completed := buildSingleCronjobMetrics(cj, ns)
		metrics = append(metrics, cjMetrics...)
		totalRunning += running
		totalFailing += failing
		totalCompleted += completed
	}

	// Update summary gauges for namespace
	metrics = append(metrics,
		prometheus.MustNewConstMetric(cronjobsCountDesc, prometheus.GaugeValue, totalRegistered, subSystem, "registered"),
		prometheus.MustNewConstMetric(cronjobsCountDesc, prometheus.GaugeValue, totalRunning, subSystem, "running"),
		prometheus.MustNewConstMetric(cronjobsCountDesc, prometheus.GaugeValue, totalFailing, subSystem, "failing"),
		prometheus.MustNewConstMetric(cronjobsCountDesc, prometheus.GaugeValue, totalCompleted, subSystem, "completed"),
	)

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

	return &metricsSnapshot{metrics: metrics}
}

// processCronjobsResponse builds an immutable metric snapshot and publishes it atomically.
func processCronjobsResponse(cronjobs []*protos.CronjobResponse, subSystem string, cronjobsMap *sync.Map) {
	defaultMetricsCollector.snapshot.Store(buildMetricsSnapshot(cronjobs, subSystem, cronjobsMap))
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

func storeSyncedOrDegraded(st store.Store) bool {
	ready, degradedFor := st.Health()
	return ready || degradedFor > 0
}

// collectMetrics refreshes Prometheus metrics straight from the in-memory store.
// It waits for initial informer sync before publishing metrics, debounces rapid change
// notifications like streamLoop, refreshes periodically on interval, and stops on ctx cancel.
func collectMetrics(ctx context.Context, svr *Sk8lServer, cronjobsMap *sync.Map, interval time.Duration) {
	collectMetricsInto(ctx, svr, cronjobsMap, interval, defaultMetricsCollector)
}

func collectMetricsInto(
	ctx context.Context,
	svr *Sk8lServer,
	cronjobsMap *sync.Map,
	interval time.Duration,
	collector *metricsCollector,
) {
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

	// Subscribe before waiting/refreshing so no change between sync and loop entry is missed.
	notifyCh := st.Subscribe(ctx)
	if waiter, ok := st.(syncWaiter); ok && !waiter.WaitForSync(ctx) {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var debouncer streamDebouncer
	defer debouncer.stop()

	refresh := func() {
		if !storeSyncedOrDegraded(st) {
			return
		}
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
		collector.snapshot.Store(buildMetricsSnapshot(cronjobs, subSystem, cronjobsMap))
	}

	// Drain any notification queued during initial sync before the first refresh.
	select {
	case _, ok := <-notifyCh:
		if !ok {
			return
		}
	default:
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
			debouncer.reset()
		case <-debouncer.ch:
			debouncer.clear()
			if ctx.Err() != nil {
				return
			}
			refresh()
		case <-ticker.C:
			if debouncer.active() {
				continue
			}
			refresh()
		}
	}
}
