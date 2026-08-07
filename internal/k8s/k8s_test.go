package k8s

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetCronjob(t *testing.T) {
	cronjob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cj",
			Namespace: "default",
		},
	}
	clientSet := fake.NewClientset(cronjob)
	client := NewClientWithInterface(clientSet, WithNamespace("default"))

	ctx := context.Background()

	// Found case
	cj, err := client.GetCronjob(ctx, "default", "test-cj")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cj.Name != "test-cj" {
		t.Errorf("expected test-cj, got %s", cj.Name)
	}

	// Not found case
	_, err = client.GetCronjob(ctx, "default", "non-existent")
	if err == nil {
		t.Fatal("expected error for non-existent cronjob, got nil")
	}
}

func TestGetJob(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
	}
	clientSet := fake.NewClientset(job)
	client := NewClientWithInterface(clientSet, WithNamespace("default"))

	ctx := context.Background()

	// Found case
	j, err := client.GetJob(ctx, "default", "test-job")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if j.Name != "test-job" {
		t.Errorf("expected test-job, got %s", j.Name)
	}

	// Not found case
	_, err = client.GetJob(ctx, "default", "non-existent")
	if err == nil {
		t.Fatal("expected error for non-existent job, got nil")
	}
}

func TestGetPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}
	clientSet := fake.NewClientset(pod)
	client := NewClientWithInterface(clientSet, WithNamespace("default"))

	ctx := context.Background()

	// Found case
	p, err := client.GetPod(ctx, "default", "test-pod")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.Name != "test-pod" {
		t.Errorf("expected test-pod, got %s", p.Name)
	}

	// Not found case
	_, err = client.GetPod(ctx, "default", "non-existent")
	if err == nil {
		t.Fatal("expected error for non-existent pod, got nil")
	}
}

func TestGetAllJobs(t *testing.T) {
	job1 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-1",
			Namespace: "default",
		},
	}
	job2 := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-2",
			Namespace: "default",
		},
	}
	clientSet := fake.NewClientset(job1, job2)
	client := NewClientWithInterface(clientSet, WithNamespace("default"))

	ctx := context.Background()
	jobs, err := client.GetAllJobs(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(jobs.Items) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs.Items))
	}
}

func TestWatchMethods(t *testing.T) {
	clientSet := fake.NewClientset()
	client := NewClientWithInterface(clientSet, WithNamespace("default"))

	ctx := context.Background()

	cjWatcher, err := client.WatchCronjobs(ctx)
	if err != nil {
		t.Fatalf("WatchCronjobs failed: %v", err)
	}
	cjWatcher.Stop()

	jobWatcher, err := client.WatchJobs(ctx)
	if err != nil {
		t.Fatalf("WatchJobs failed: %v", err)
	}
	jobWatcher.Stop()

	podWatcher, err := client.WatchPods(ctx)
	if err != nil {
		t.Fatalf("WatchPods failed: %v", err)
	}
	podWatcher.Stop()
}

func TestNamespace(t *testing.T) {
	clientSet := fake.NewClientset()
	client := NewClientWithInterface(clientSet, WithNamespace("custom-ns"))
	if client.Namespace() != "custom-ns" {
		t.Errorf("expected custom-ns, got %s", client.Namespace())
	}
}
