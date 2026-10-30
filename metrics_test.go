package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/danroux/sk8l/internal/k8s"
	"github.com/danroux/sk8l/protos"
	"github.com/danroux/sk8l/testutil"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func getGaugeValue(g prometheus.Gauge) float64 {
	var m dto.Metric
	if err := g.Write(&m); err != nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

// waitForValue polls read until it returns want or the deadline expires.
// read must resolve the gauge on every call: processCronjobsResponse resets the
// vectors, so a child gauge captured once would never observe later updates.
func waitForValue(t *testing.T, read func() float64, want float64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := read()
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for gauge value %v, last value %v", want, got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestCollectMetrics_InMemory verifies that metrics are computed straight from the
// store (no gRPC loopback), refreshed on store notifications and stopped on cancel.
func TestCollectMetrics_InMemory(t *testing.T) {
	const ns = "metrics-inmem"

	cronjob := testutil.NewCronJobBuilder().
		WithName("backup").
		WithNamespace(ns).
		Build()
	clientSet := fake.NewClientset(cronjob)
	k8sClient := k8s.NewClientWithInterface(clientSet, k8s.WithNamespace(ns))
	st := setupTestInformerStore(t, clientSet, ns)
	s := NewSk8lServer(st, k8sClient, nil)
	cronjobsMap := &sync.Map{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// A long interval ensures updates below come from store notifications, not the ticker.
		collectMetrics(ctx, s, cronjobsMap, time.Hour)
	}()

	registered := func() float64 {
		return getGaugeValue(cronjobsCountGauge.WithLabelValues(ns, "registered"))
	}

	// Initial refresh runs without waiting for any store event.
	waitForValue(t, registered, 1)

	second := testutil.NewCronJobBuilder().
		WithName("cleanup").
		WithNamespace(ns).
		Build()
	if _, err := clientSet.BatchV1().CronJobs(ns).Create(ctx, second, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to create cronjob %q: %v", second.Name, err)
	}

	waitForValue(t, registered, 2)
	if _, ok := cronjobsMap.Load("cleanup"); !ok {
		t.Error("expected cleanup to be tracked in cronjobsMap")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("collectMetrics did not stop after context cancellation")
	}
}

// TestCollectMetrics_NilStore verifies the collector returns immediately when no store is configured.
func TestCollectMetrics_NilStore(t *testing.T) {
	s := NewSk8lServer(nil, nil, nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		collectMetrics(context.Background(), s, nil, time.Hour)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("collectMetrics should return immediately when the store is nil")
	}
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
