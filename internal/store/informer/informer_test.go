package informer

import (
	"context"
	"errors"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	testTimeout    = 5 * time.Second
	testNamespace  = "test-ns"
	expectedTwo    = 2
	expectedSingle = 1
)

func TestNew_Validation(t *testing.T) {
	_, err := New(nil, testNamespace)
	if !errors.Is(err, ErrClientRequired) {
		t.Fatalf("expected ErrClientRequired, got %v", err)
	}
}

func TestFindCronjobs_and_FindCronjob(t *testing.T) {
	cj1 := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "daily-report",
			Namespace: testNamespace,
		},
	}
	cj2 := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hourly-cleanup",
			Namespace: testNamespace,
		},
	}

	cs := fake.NewClientset(cj1, cj2)
	s, err := New(cs, testNamespace)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	s.Start(ctx)
	if !s.WaitForSync(ctx) {
		t.Fatal("timed out waiting for informer sync")
	}

	if !s.Ready() {
		t.Error("expected Ready() == true after WaitForSync")
	}

	cronjobs, err := s.FindCronjobs()
	if err != nil {
		t.Fatalf("FindCronjobs failed: %v", err)
	}
	if len(cronjobs.Items) != expectedTwo {
		t.Fatalf("expected %d cronjobs, got %d", expectedTwo, len(cronjobs.Items))
	}

	found, err := s.FindCronjob(ctx, testNamespace, "daily-report")
	if err != nil {
		t.Fatalf("FindCronjob failed: %v", err)
	}
	if found.Name != "daily-report" {
		t.Errorf("expected daily-report, got %s", found.Name)
	}

	_, err = s.FindCronjob(ctx, testNamespace, "non-existent")
	if err == nil {
		t.Error("expected error for non-existent cronjob, got nil")
	}
}

func TestFindJobs_and_FindJobsMapped(t *testing.T) {
	job1 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "daily-report-1234",
			Namespace: testNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "CronJob",
					Name: "daily-report",
				},
			},
		},
	}
	job2 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone-job",
			Namespace: testNamespace,
		},
	}

	cs := fake.NewClientset(job1, job2)
	s, err := New(cs, testNamespace)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	s.Start(ctx)
	if !s.WaitForSync(ctx) {
		t.Fatal("timed out waiting for informer sync")
	}

	jobs, err := s.FindJobs()
	if err != nil {
		t.Fatalf("FindJobs failed: %v", err)
	}
	if len(jobs.Items) != expectedTwo {
		t.Fatalf("expected %d jobs, got %d", expectedTwo, len(jobs.Items))
	}

	mapped, err := s.FindJobsMapped(ctx)
	if err != nil {
		t.Fatalf("FindJobsMapped failed: %v", err)
	}
	dailyJobs, ok := mapped["daily-report"]
	if !ok || len(dailyJobs) != expectedSingle {
		t.Fatalf("expected 1 job mapped for daily-report, got %d", len(dailyJobs))
	}
	if dailyJobs[0].Name != "daily-report-1234" {
		t.Errorf("expected daily-report-1234, got %s", dailyJobs[0].Name)
	}
}

func TestFindJobPodsForJob(t *testing.T) {
	now := metav1.Now()
	isController := true

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-a-pod-1",
			Namespace: testNamespace,
			Labels: map[string]string{
				"job-name": "job-a",
			},
		},
		Status: corev1.PodStatus{
			StartTime: &now,
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-a-pod-2",
			Namespace: testNamespace,
			Labels: map[string]string{
				"job-name": "job-a",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Job",
					Name:       "job-a",
					Controller: &isController,
				},
			},
		},
		Status: corev1.PodStatus{
			StartTime: &now,
		},
	}

	podPending := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-a-pending",
			Namespace: testNamespace,
			Labels: map[string]string{
				"job-name": "job-a",
			},
		},
		// StartTime is nil: should be omitted for consistency with previous store
	}

	podOtherJob := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-b-pod-1",
			Namespace: testNamespace,
			Labels: map[string]string{
				"job-name": "job-b",
			},
		},
		Status: corev1.PodStatus{
			StartTime: &now,
		},
	}

	cs := fake.NewClientset(pod1, pod2, podPending, podOtherJob)
	s, err := New(cs, testNamespace)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	s.Start(ctx)
	if !s.WaitForSync(ctx) {
		t.Fatal("timed out waiting for informer sync")
	}

	jobA := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job-a"}}
	pods, err := s.FindJobPodsForJob(jobA)
	if err != nil {
		t.Fatalf("FindJobPodsForJob failed: %v", err)
	}

	if len(pods.Items) != expectedTwo {
		t.Fatalf("expected %d pods for job-a, got %d", expectedTwo, len(pods.Items))
	}

	// Test nil job input
	nilPods, err := s.FindJobPodsForJob(nil)
	if err != nil {
		t.Fatalf("expected nil error for nil job, got %v", err)
	}
	if len(nilPods.Items) != 0 {
		t.Errorf("expected 0 pods for nil job, got %d", len(nilPods.Items))
	}
}

func TestSubscribe(t *testing.T) {
	cs := fake.NewClientset()
	s, err := New(cs, testNamespace)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	s.Start(ctx)
	if !s.WaitForSync(ctx) {
		t.Fatal("timed out waiting for informer sync")
	}

	subCtx, subCancel := context.WithCancel(ctx)
	ch := s.Subscribe(subCtx)

	// Trigger a change via notify directly or through creating an object
	s.notifier.notify()

	select {
	case <-ch:
		// Success: notification received
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for subscriber notification")
	}

	// SubCancel should close the channel
	subCancel()

	select {
	case _, ok := <-ch:
		if ok {
			// Might drain previous buffered signal, verify subsequent read is closed
			select {
			case _, ok2 := <-ch:
				if ok2 {
					t.Error("expected subscribe channel to be closed after cancel")
				}
			case <-time.After(testTimeout):
				t.Fatal("timed out waiting for channel close")
			}
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for channel close after cancel")
	}
}

func TestSetTransform_ManagedFieldsStripped(t *testing.T) {
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "managed-cron",
			Namespace: testNamespace,
			ManagedFields: []metav1.ManagedFieldsEntry{
				{
					Manager: "test-manager",
				},
			},
		},
	}

	cs := fake.NewClientset(cj)
	s, err := New(cs, testNamespace)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	s.Start(ctx)
	if !s.WaitForSync(ctx) {
		t.Fatal("timed out waiting for informer sync")
	}

	found, err := s.FindCronjob(ctx, testNamespace, "managed-cron")
	if err != nil {
		t.Fatalf("FindCronjob failed: %v", err)
	}

	if len(found.ManagedFields) != 0 {
		t.Errorf("expected 0 managedFields due to SetTransform, got %d", len(found.ManagedFields))
	}
}

func TestJobNameIndexFunc(t *testing.T) {
	isController := true

	// 1. Pod with job-name label
	podWithLabel := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-label",
			Labels: map[string]string{
				"job-name": "job-from-label",
			},
		},
	}
	keys, err := jobNameIndexFunc(podWithLabel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 1 || keys[0] != "job-from-label" {
		t.Errorf("expected [job-from-label], got %v", keys)
	}

	// 2. Pod without label, but with Job controller OwnerReference
	podWithOwner := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-owner",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Job",
					Name:       "job-from-owner",
					Controller: &isController,
				},
			},
		},
	}
	keys, err = jobNameIndexFunc(podWithOwner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 1 || keys[0] != "job-from-owner" {
		t.Errorf("expected [job-from-owner], got %v", keys)
	}

	// 3. Pod with neither
	podEmpty := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-empty",
		},
	}
	keys, err = jobNameIndexFunc(podEmpty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty keys, got %v", keys)
	}

	// 4. Non-pod object
	keys, err = jobNameIndexFunc("not-a-pod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty keys for non-pod, got %v", keys)
	}
}
