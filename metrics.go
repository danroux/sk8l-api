package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/danroux/sk8l/protos"
	"google.golang.org/grpc"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
)

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

func collectMetricsStream(ctx context.Context, c protos.CronjobClient, subSystem string, cronjobsMap *sync.Map) error {
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

		processCronjobsResponse(cronjobsResponse.Cronjobs, subSystem, cronjobsMap)
	}
}

func recordMetrics(ctx context.Context, svr *Sk8lServer, cronjobsMap *sync.Map) {
	conn, err := grpc.NewClient(svr.GetTarget(), svr.GetDialOptions()...)
	if err != nil {
		log.Error().
			Err(err).
			Str("operation", "recordMetrics").
			Msg(fmt.Sprintf("grpc.NewClient(%s) failed", svr.GetTarget()))
		return
	}

	c := protos.NewCronjobClient(conn)
	subSystem := ""
	if cl := svr.GetK8sClient(); cl != nil {
		subSystem = cl.Namespace()
	}

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

			if err := collectMetricsStream(ctx, c, subSystem, cronjobsMap); err != nil {
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
