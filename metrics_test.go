package main

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danroux/sk8l/internal/k8s"
	"github.com/danroux/sk8l/protos"
	"github.com/danroux/sk8l/testutil"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func gatherMetricValue(t *testing.T, gatherer prometheus.Gatherer, metricName string, wantLabels map[string]string) (float64, bool) {
	t.Helper()

	families, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("Gather() failed: %v", err)
	}

	for _, mf := range families {
		if mf.GetName() != metricName {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m.GetLabel(), wantLabels) {
				return m.GetGauge().GetValue(), true
			}
		}
	}
	return 0, false
}

func labelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	if len(pairs) != len(want) {
		return false
	}
	for _, lp := range pairs {
		val, ok := want[lp.GetName()]
		if !ok || val != lp.GetValue() {
			return false
		}
	}
	return true
}

func mustGatherMetricValue(t *testing.T, gatherer prometheus.Gatherer, metricName string, wantLabels map[string]string) float64 {
	t.Helper()
	val, ok := gatherMetricValue(t, gatherer, metricName, wantLabels)
	if !ok {
		t.Fatalf("metric %s with labels %v not found", metricName, wantLabels)
	}
	return val
}

// waitForValue polls read until it returns want or the deadline expires.
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

type countingMetricsStore struct {
	fakeHealthStore
	findCalls atomic.Int32
}

func (c *countingMetricsStore) FindCronjobs() (*batchv1.CronJobList, error) {
	c.findCalls.Add(1)
	return &batchv1.CronJobList{}, nil
}

func (c *countingMetricsStore) FindJobsMapped(context.Context) (map[string][]*batchv1.Job, error) {
	return map[string][]*batchv1.Job{}, nil
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
		val, _ := gatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjobs_count", map[string]string{
			"namespace": ns,
			"status":    "registered",
		})
		return val
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

// TestMetricsCollector_PreSyncPublishesNoZeroes verifies that /metrics emits no bogus
// zero counts while the store has not yet completed initial synchronization.
func TestMetricsCollector_PreSyncPublishesNoZeroes(t *testing.T) {
	const ns = "metrics-presync"

	notifyCh := make(chan struct{}, 4)
	cs := &countingMetricsStore{}
	cs.notifyCh = notifyCh
	cs.ready.Store(false)
	cs.degradedSince = 0

	reg := prometheus.NewPedanticRegistry()
	collector := newRegisteredMetricsCollector(reg)
	srv := NewSk8lServer(cs, k8s.NewClientWithInterface(fake.NewClientset(), k8s.WithNamespace(ns)), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		collectMetricsInto(ctx, srv, nil, time.Hour, collector)
	}()

	// Wait briefly to confirm initial refresh did not emit pre-sync zeroes.
	time.Sleep(50 * time.Millisecond)
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() failed: %v", err)
	}
	if len(families) != 0 {
		t.Fatalf("expected 0 metric families before sync, got %d", len(families))
	}

	// Transition store to synced and notify: true count (0 cronjobs) should now appear.
	cs.ready.Store(true)
	notifyCh <- struct{}{}

	registered := func() float64 {
		val, ok := gatherMetricValue(t, reg, "sk8l_cronjobs_count", map[string]string{
			"namespace": ns,
			"status":    "registered",
		})
		if !ok {
			return -1
		}
		return val
	}
	waitForValue(t, registered, 0)

	cancel()
	<-done
}

// TestCollectMetrics_DebounceCoalescesRapidNotifications verifies that rapid burst
// updates are coalesced instead of triggering a full refresh per notification.
func TestCollectMetrics_DebounceCoalescesRapidNotifications(t *testing.T) {
	notifyCh := make(chan struct{}, 10)
	cs := &countingMetricsStore{}
	cs.notifyCh = notifyCh
	cs.ready.Store(true)

	collector := newRegisteredMetricsCollector(prometheus.NewPedanticRegistry())
	srv := NewSk8lServer(cs, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		collectMetricsInto(ctx, srv, nil, time.Hour, collector)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && cs.findCalls.Load() < 1 {
		time.Sleep(10 * time.Millisecond)
	}
	if cs.findCalls.Load() != 1 {
		t.Fatalf("expected 1 initial refresh, got %d", cs.findCalls.Load())
	}

	for range 5 {
		notifyCh <- struct{}{}
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(streamDebounceInterval + 150*time.Millisecond)
	if got := cs.findCalls.Load(); got != 2 {
		t.Fatalf("expected 2 total refreshes (initial + 1 coalesced burst), got %d", got)
	}

	cancel()
	<-done
}

// TestMetricsCollector_AtomicSnapshotDuringConcurrentGather verifies that concurrent
// scrapes never observe empty or partially updated metrics while snapshots rotate.
func TestMetricsCollector_AtomicSnapshotDuringConcurrentGather(t *testing.T) {
	const ns = "metrics-race"

	reg := prometheus.NewPedanticRegistry()
	collector := newRegisteredMetricsCollector(reg)

	one := []*protos.CronjobResponse{
		{Name: "cj-1", Namespace: ns},
	}
	two := []*protos.CronjobResponse{
		{Name: "cj-1", Namespace: ns},
		{Name: "cj-2", Namespace: ns},
	}
	collector.snapshot.Store(buildMetricsSnapshot(one, ns, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; ctx.Err() == nil; i++ {
			if i%2 == 0 {
				collector.snapshot.Store(buildMetricsSnapshot(two, ns, nil))
			} else {
				collector.snapshot.Store(buildMetricsSnapshot(one, ns, nil))
			}
		}
	})

	wg.Go(func() {
		for ctx.Err() == nil {
			families, err := reg.Gather()
			if err != nil {
				t.Errorf("Gather() failed during concurrent swap: %v", err)
				return
			}
			var registered float64
			var infoCount int
			var foundRegistered bool
			for _, mf := range families {
				switch mf.GetName() {
				case "sk8l_cronjobs_count":
					for _, m := range mf.GetMetric() {
						if labelsMatch(m.GetLabel(), map[string]string{"namespace": ns, "status": "registered"}) {
							registered = m.GetGauge().GetValue()
							foundRegistered = true
						}
					}
				case "sk8l_cronjob_info":
					infoCount = len(mf.GetMetric())
				}
			}
			if !foundRegistered || int(registered) != infoCount {
				t.Errorf("inconsistent scrape snapshot: found=%v registered=%v infoCount=%d",
					foundRegistered, registered, infoCount)
				return
			}
		}
	})

	wg.Wait()
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
	if got := mustGatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjobs_count", map[string]string{
		"namespace": "production",
		"status":    "registered",
	}); got != 2 {
		t.Errorf("expected 2 registered cronjobs, got %f", got)
	}
	if got := mustGatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjobs_count", map[string]string{
		"namespace": "production",
		"status":    "running",
	}); got != 1 {
		t.Errorf("expected 1 running cronjob, got %f", got)
	}
	if got := mustGatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjobs_count", map[string]string{
		"namespace": "production",
		"status":    "failing",
	}); got != 1 {
		t.Errorf("expected 1 failing cronjob, got %f", got)
	}
	if got := mustGatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjobs_count", map[string]string{
		"namespace": "production",
		"status":    "completed",
	}); got != 1 {
		t.Errorf("expected 1 completed cronjob, got %f", got)
	}

	// Verify last run status
	if got := mustGatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjob_last_run_status", map[string]string{
		"namespace": "production",
		"cronjob":   "backup-job",
		"status":    "succeeded",
	}); got != 1 {
		t.Errorf("expected backup-job succeeded=1, got %f", got)
	}
	if got := mustGatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjob_last_run_status", map[string]string{
		"namespace": "production",
		"cronjob":   "backup-job",
		"status":    "failed",
	}); got != 0 {
		t.Errorf("expected backup-job failed=0, got %f", got)
	}
	if got := mustGatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjob_last_run_status", map[string]string{
		"namespace": "production",
		"cronjob":   "sync-job",
		"status":    "succeeded",
	}); got != 0 {
		t.Errorf("expected sync-job succeeded=0, got %f", got)
	}
	if got := mustGatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjob_last_run_status", map[string]string{
		"namespace": "production",
		"cronjob":   "sync-job",
		"status":    "failed",
	}); got != 1 {
		t.Errorf("expected sync-job failed=1, got %f", got)
	}

	// Verify duration
	if got := mustGatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjob_last_run_duration_seconds", map[string]string{
		"namespace": "production",
		"cronjob":   "backup-job",
	}); got != 42 {
		t.Errorf("expected backup-job duration 42s, got %f", got)
	}

	// Verify active jobs
	if got := mustGatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjob_active_jobs", map[string]string{
		"namespace": "production",
		"cronjob":   "sync-job",
	}); got != 1 {
		t.Errorf("expected sync-job active=1, got %f", got)
	}

	// Verify cronjobsMap tracked active cronjobs
	if _, ok := cronjobsMap.Load("backup-job"); !ok {
		t.Errorf("expected backup-job to be tracked in cronjobsMap")
	}
	if _, ok := cronjobsMap.Load("sync-job"); !ok {
		t.Errorf("expected sync-job to be tracked in cronjobsMap")
	}

	// Removing sync-job from the snapshot must remove its series cleanly.
	processCronjobsResponse(cronjobs[:1], "production", cronjobsMap)
	if _, ok := gatherMetricValue(t, prometheus.DefaultGatherer, "sk8l_cronjob_active_jobs", map[string]string{
		"namespace": "production",
		"cronjob":   "sync-job",
	}); ok {
		t.Errorf("expected deleted sync-job metric to be absent from snapshot")
	}
	if _, ok := cronjobsMap.Load("sync-job"); ok {
		t.Errorf("expected deleted sync-job to be pruned from cronjobsMap")
	}
}

func TestBuildMetricsSnapshot_LastRunTransition(t *testing.T) {
	const ns = "production"
	s := Sk8lServer{}
	t0 := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	t0Meta := metav1.NewTime(t0)
	t0Fail := metav1.NewTime(t0.Add(15 * time.Second))
	t1Meta := metav1.NewTime(t0.Add(time.Minute))
	t1Done := metav1.NewTime(t0.Add(time.Minute + 42*time.Second))

	recoveredCJ := testutil.NewCronJobBuilder().
		WithName("nightly-backup").
		WithNamespace(ns).
		WithStatus(batchv1.CronJobStatus{LastSuccessfulTime: &t1Done}).
		Build()

	recoveredResp := s.cronJobResponse(*recoveredCJ, []*batchv1.Job{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "nightly-backup-1", Namespace: ns, CreationTimestamp: t0Meta},
			Status: batchv1.JobStatus{
				StartTime: &t0Meta,
				Failed:    1,
				Conditions: []batchv1.JobCondition{
					{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: t0Fail},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "nightly-backup-2", Namespace: ns, CreationTimestamp: t1Meta},
			Status: batchv1.JobStatus{
				StartTime:      &t1Meta,
				CompletionTime: &t1Done,
				Succeeded:      1,
			},
		},
	})

	neverRunCJ := testutil.NewCronJobBuilder().
		WithName("brand-new").
		WithNamespace(ns).
		WithStatus(batchv1.CronJobStatus{}).
		Build()
	neverRunResp := s.cronJobResponse(*neverRunCJ, nil)

	zeroHistoryResp := &protos.CronjobResponse{
		Name:               "zero-history",
		Namespace:          ns,
		LastSuccessfulTime: "2026-11-25T10:05:00Z",
	}

	cases := []struct {
		cronjob           *protos.CronjobResponse
		name              string
		wantFailed        float64
		wantSucceeded     float64
		wantFailingCount  float64
		wantCompleted     float64
		wantFailedJobs    float64
		wantSucceededJobs float64
	}{
		{
			name:              "run 1 failed then run 2 succeeded flips last_run_status while retaining failed job count",
			cronjob:           recoveredResp,
			wantFailed:        0,
			wantSucceeded:     1,
			wantFailingCount:  0,
			wantCompleted:     1,
			wantFailedJobs:    1,
			wantSucceededJobs: 1,
		},
		{
			name:              "never-run cronjob emits 0 for both failed and succeeded",
			cronjob:           neverRunResp,
			wantFailed:        0,
			wantSucceeded:     0,
			wantFailingCount:  0,
			wantCompleted:     0,
			wantFailedJobs:    0,
			wantSucceededJobs: 0,
		},
		{
			name:              "successfulJobHistoryLimit 0 falls back to LastSuccessfulTime",
			cronjob:           zeroHistoryResp,
			wantFailed:        0,
			wantSucceeded:     1,
			wantFailingCount:  0,
			wantCompleted:     1,
			wantFailedJobs:    0,
			wantSucceededJobs: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewPedanticRegistry()
			collector := newRegisteredMetricsCollector(reg)
			collector.snapshot.Store(buildMetricsSnapshot([]*protos.CronjobResponse{tc.cronjob}, ns, nil))

			name := tc.cronjob.Name
			assertGauge(t, reg, "sk8l_cronjob_last_run_status", cjStatusLabels(ns, name, "failed"), tc.wantFailed)
			assertGauge(t, reg, "sk8l_cronjob_last_run_status", cjStatusLabels(ns, name, "succeeded"), tc.wantSucceeded)
			assertGauge(t, reg, "sk8l_cronjobs_count", nsStatusLabels(ns, "failing"), tc.wantFailingCount)
			assertGauge(t, reg, "sk8l_cronjobs_count", nsStatusLabels(ns, "completed"), tc.wantCompleted)
			assertGauge(t, reg, "sk8l_cronjob_jobs_count", cjStatusLabels(ns, name, "failed"), tc.wantFailedJobs)
			assertGauge(t, reg, "sk8l_cronjob_jobs_count", cjStatusLabels(ns, name, "succeeded"), tc.wantSucceededJobs)
		})
	}
}

func nsStatusLabels(ns, status string) prometheus.Labels {
	return prometheus.Labels{"namespace": ns, "status": status}
}

func cjStatusLabels(ns, cronjob, status string) prometheus.Labels {
	return prometheus.Labels{"namespace": ns, "cronjob": cronjob, "status": status}
}

func assertGauge(t *testing.T, g prometheus.Gatherer, metric string, labels prometheus.Labels, want float64) {
	t.Helper()
	if got := mustGatherMetricValue(t, g, metric, labels); got != want {
		t.Errorf("%s%v = %v, want %v", metric, labels, got, want)
	}
}

func TestMetricsCollector_ExpositionSnapshot(t *testing.T) {
	const ns = "production"
	s := &Sk8lServer{}
	t0 := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	t0Meta := metav1.NewTime(t0)
	t0Fail := metav1.NewTime(t0.Add(15 * time.Second))
	t1Meta := metav1.NewTime(t0.Add(time.Minute))
	t1Done := metav1.NewTime(t0.Add(time.Minute + 42*time.Second))

	cj := testutil.NewCronJobBuilder().
		WithName("nightly-backup").
		WithNamespace(ns).
		WithSchedule("0 2 * * *").
		WithStatus(batchv1.CronJobStatus{LastSuccessfulTime: &t1Done}).
		Build()

	resp := s.cronJobResponse(*cj, []*batchv1.Job{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "nightly-backup-1", Namespace: ns, CreationTimestamp: t0Meta},
			Status: batchv1.JobStatus{
				StartTime: &t0Meta,
				Failed:    1,
				Conditions: []batchv1.JobCondition{
					{
						Type:               batchv1.JobFailed,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: t0Fail,
						Reason:             batchv1.JobReasonBackoffLimitExceeded,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "nightly-backup-2", Namespace: ns, CreationTimestamp: t1Meta},
			Status: batchv1.JobStatus{
				StartTime:      &t1Meta,
				CompletionTime: &t1Done,
				Succeeded:      1,
			},
		},
	})

	collector := newRegisteredMetricsCollector(prometheus.NewPedanticRegistry())
	collector.snapshot.Store(buildMetricsSnapshot([]*protos.CronjobResponse{resp}, ns, nil))

	expected := `
# HELP sk8l_cronjob_active_jobs Number of currently active child jobs for a cronjob.
# TYPE sk8l_cronjob_active_jobs gauge
sk8l_cronjob_active_jobs{cronjob="nightly-backup",namespace="production"} 0
# HELP sk8l_cronjob_info Metadata and configuration for a cronjob (constant value of 1).
# TYPE sk8l_cronjob_info gauge
sk8l_cronjob_info{cronjob="nightly-backup",namespace="production",schedule="0 2 * * *",suspend="false"} 1
# HELP sk8l_cronjob_jobs_count Count of retained child jobs for a cronjob by state (succeeded, failed).
# TYPE sk8l_cronjob_jobs_count gauge
sk8l_cronjob_jobs_count{cronjob="nightly-backup",namespace="production",status="failed"} 1
sk8l_cronjob_jobs_count{cronjob="nightly-backup",namespace="production",status="succeeded"} 1
# HELP sk8l_cronjob_last_run_duration_seconds Duration in seconds of the latest execution of a cronjob.
# TYPE sk8l_cronjob_last_run_duration_seconds gauge
sk8l_cronjob_last_run_duration_seconds{cronjob="nightly-backup",namespace="production"} 42
# HELP sk8l_cronjob_last_run_status Status of the last execution of a cronjob (1 if true, 0 if false).
# TYPE sk8l_cronjob_last_run_status gauge
sk8l_cronjob_last_run_status{cronjob="nightly-backup",namespace="production",status="failed"} 0
sk8l_cronjob_last_run_status{cronjob="nightly-backup",namespace="production",status="succeeded"} 1
# HELP sk8l_cronjobs_count Current count of cronjobs grouped by namespace and status (registered, running, failing, completed).
# TYPE sk8l_cronjobs_count gauge
sk8l_cronjobs_count{namespace="production",status="completed"} 1
sk8l_cronjobs_count{namespace="production",status="failing"} 0
sk8l_cronjobs_count{namespace="production",status="registered"} 1
sk8l_cronjobs_count{namespace="production",status="running"} 0
`

	if err := promtest.CollectAndCompare(collector, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected Prometheus exposition diff:\n%v", err)
	}
}
