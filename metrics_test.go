package main

import (
	"sync"
	"testing"

	"github.com/danroux/sk8l/protos"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func getGaugeValue(g prometheus.Gauge) float64 {
	var m dto.Metric
	if err := g.Write(&m); err != nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

func TestProcessCronjobsResponse_OpenMetrics(t *testing.T) {
	cronjobsMap := &sync.Map{}

	cronjobs := []*protos.CronjobResponse{
		{
			Name:               "backup-job",
			Namespace:          "production",
			Failed:             false,
			LastDuration:       42,
			LastSuccessfulTime: "2026-10-19T10:00:00Z",
			Spec: &protos.CronJobSpecResponse{
				Schedule: "0 2 * * *",
				Suspend:  false,
			},
			Jobs: []*protos.JobResponse{
				{
					Name:      "backup-job-1",
					Succeeded: true,
				},
				{
					Name:      "backup-job-2",
					Succeeded: true,
				},
			},
		},
		{
			Name:      "sync-job",
			Namespace: "production",
			Failed:    true,
			RunningJobs: []*protos.JobResponse{
				{
					Name: "sync-job-active",
				},
			},
			Jobs: []*protos.JobResponse{
				{
					Name:   "sync-job-failed-1",
					Failed: true,
				},
			},
		},
	}

	processCronjobsResponse(cronjobs, "production", cronjobsMap)

	// Verify overview counts
	registeredCount := getGaugeValue(cronjobsCountGauge.WithLabelValues("production", "registered"))
	if registeredCount != 2 {
		t.Errorf("expected 2 registered cronjobs, got %f", registeredCount)
	}

	runningCount := getGaugeValue(cronjobsCountGauge.WithLabelValues("production", "running"))
	if runningCount != 1 {
		t.Errorf("expected 1 running cronjob, got %f", runningCount)
	}

	failingCount := getGaugeValue(cronjobsCountGauge.WithLabelValues("production", "failing"))
	if failingCount != 1 {
		t.Errorf("expected 1 failing cronjob, got %f", failingCount)
	}

	completedCount := getGaugeValue(cronjobsCountGauge.WithLabelValues("production", "completed"))
	if completedCount != 1 {
		t.Errorf("expected 1 completed cronjob, got %f", completedCount)
	}

	// Verify last run status
	backupSuccess := getGaugeValue(cronjobLastRunStatusGauge.WithLabelValues("production", "backup-job", "succeeded"))
	if backupSuccess != 1 {
		t.Errorf("expected backup-job succeeded=1, got %f", backupSuccess)
	}
	backupFailed := getGaugeValue(cronjobLastRunStatusGauge.WithLabelValues("production", "backup-job", "failed"))
	if backupFailed != 0 {
		t.Errorf("expected backup-job failed=0, got %f", backupFailed)
	}

	syncSuccess := getGaugeValue(cronjobLastRunStatusGauge.WithLabelValues("production", "sync-job", "succeeded"))
	if syncSuccess != 0 {
		t.Errorf("expected sync-job succeeded=0, got %f", syncSuccess)
	}
	syncFailed := getGaugeValue(cronjobLastRunStatusGauge.WithLabelValues("production", "sync-job", "failed"))
	if syncFailed != 1 {
		t.Errorf("expected sync-job failed=1, got %f", syncFailed)
	}

	// Verify duration
	duration := getGaugeValue(cronjobLastRunDurationGauge.WithLabelValues("production", "backup-job"))
	if duration != 42 {
		t.Errorf("expected backup-job duration 42s, got %f", duration)
	}

	// Verify active jobs
	syncActive := getGaugeValue(cronjobActiveJobsGauge.WithLabelValues("production", "sync-job"))
	if syncActive != 1 {
		t.Errorf("expected sync-job active=1, got %f", syncActive)
	}

	// Verify cronjobsMap tracked active cronjobs
	if _, ok := cronjobsMap.Load("backup-job"); !ok {
		t.Errorf("expected backup-job to be tracked in cronjobsMap")
	}
	if _, ok := cronjobsMap.Load("sync-job"); !ok {
		t.Errorf("expected sync-job to be tracked in cronjobsMap")
	}
}
