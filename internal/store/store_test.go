package store

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danroux/sk8l/internal/k8s"
	badger "github.com/dgraph-io/badger/v4"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func setupTestDB(t *testing.T) *badger.DB {
	t.Helper()
	opts := badger.DefaultOptions("").WithInMemory(true).WithLoggingLevel(badger.ERROR)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("failed to open test badger DB: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestNewCronJobDBStore_Validation(t *testing.T) {
	db := setupTestDB(t)

	// Missing K8sClient must return ErrK8sClientRequired
	_, err := NewCronJobDBStore(WithDB(db))
	if !errors.Is(err, ErrK8sClientRequired) {
		t.Fatalf("expected ErrK8sClientRequired, got %v", err)
	}

	// Valid setup
	fakeClientset := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(fakeClientset)
	store, err := NewCronJobDBStore(WithDB(db), WithK8sClient(k8sClient))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestFindCronjobs(t *testing.T) {
	db := setupTestDB(t)
	fakeClientset := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(fakeClientset)
	s, err := NewCronJobDBStore(WithDB(db), WithK8sClient(k8sClient))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// Empty store should return empty CronJobList
	list, err := s.FindCronjobs()
	if err != nil {
		t.Fatalf("expected no error on empty store, got %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(list.Items))
	}

	// Store cronjobs and retrieve
	cjList := &batchv1.CronJobList{
		Items: []batchv1.CronJob{
			{ObjectMeta: metav1.ObjectMeta{Name: "cj-1", Namespace: "default"}},
		},
	}
	var buf bytes.Buffer
	if err := K8sSerialize(cjList, &buf); err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	err = db.Update(func(txn *badger.Txn) error {
		return txn.Set(CronjobsCacheKey, buf.Bytes())
	})
	if err != nil {
		t.Fatalf("failed to set cache: %v", err)
	}

	retrieved, err := s.FindCronjobs()
	if err != nil {
		t.Fatalf("expected no error retrieving cronjobs, got %v", err)
	}
	if len(retrieved.Items) != 1 || retrieved.Items[0].Name != "cj-1" {
		t.Errorf("expected cj-1, got %+v", retrieved.Items)
	}
}

func TestFindCronjob(t *testing.T) {
	db := setupTestDB(t)
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj-fallback", Namespace: "default"},
	}
	fakeClientset := fake.NewClientset(cj)
	k8sClient := k8s.NewClientWithInterface(fakeClientset)
	s, err := NewCronJobDBStore(WithDB(db), WithK8sClient(k8sClient))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	found, err := s.FindCronjob(ctx, "default", "cj-fallback")
	if err != nil {
		t.Fatalf("expected fallback to k8s client, got error: %v", err)
	}
	if found.Name != "cj-fallback" {
		t.Errorf("expected cj-fallback, got %s", found.Name)
	}
}

func TestFindJobs(t *testing.T) {
	db := setupTestDB(t)
	fakeClientset := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(fakeClientset)
	s, err := NewCronJobDBStore(WithDB(db), WithK8sClient(k8sClient))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	list, err := s.FindJobs()
	if err != nil {
		t.Fatalf("expected no error on empty store, got %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(list.Items))
	}
}

func TestFindJobsMapped(t *testing.T) {
	db := setupTestDB(t)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job-1",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "parent-cronjob"},
			},
		},
	}
	fakeClientset := fake.NewClientset(job)
	k8sClient := k8s.NewClientWithInterface(fakeClientset, k8s.WithNamespace("default"))
	s, err := NewCronJobDBStore(WithDB(db), WithK8sClient(k8sClient))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	mapped, err := s.FindJobsMapped(ctx)
	if err != nil {
		t.Fatalf("expected no error from FindJobsMapped, got %v", err)
	}
	jobs, ok := mapped["parent-cronjob"]
	if !ok || len(jobs) != 1 {
		t.Fatalf("expected 1 job for parent-cronjob, got %v", jobs)
	}
	if jobs[0].Name != "test-job-1" {
		t.Errorf("expected test-job-1, got %s", jobs[0].Name)
	}
}

func TestFindJobPodsForJob(t *testing.T) {
	db := setupTestDB(t)
	fakeClientset := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(fakeClientset)
	s, err := NewCronJobDBStore(WithDB(db), WithK8sClient(k8sClient))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "my-job"}}

	// No pods stored yet
	pods, err := s.FindJobPodsForJob(job)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(pods.Items) != 0 {
		t.Errorf("expected 0 pods, got %d", len(pods.Items))
	}

	now := metav1.NewTime(time.Now())
	podList := &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "pod-v1",
					ResourceVersion: "1",
					OwnerReferences: []metav1.OwnerReference{{Name: "my-job"}},
				},
				Status: corev1.PodStatus{StartTime: &now},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "pod-v1",
					ResourceVersion: "2",
					OwnerReferences: []metav1.OwnerReference{{Name: "my-job"}},
				},
				Status: corev1.PodStatus{StartTime: &now},
			},
		},
	}
	var buf bytes.Buffer
	if err := K8sSerialize(podList, &buf); err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	key := []byte("jobs_pods_for_job_my-job")
	err = db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, buf.Bytes())
	})
	if err != nil {
		t.Fatalf("failed to set pods in badger: %v", err)
	}

	pods, err = s.FindJobPodsForJob(job)
	if err != nil {
		t.Fatalf("FindJobPodsForJob failed: %v", err)
	}
	if len(pods.Items) != 1 {
		t.Fatalf("expected 1 latest pod, got %d", len(pods.Items))
	}
	if pods.Items[0].ResourceVersion != "2" {
		t.Errorf("expected ResourceVersion 2, got %s", pods.Items[0].ResourceVersion)
	}
}

func TestK8sSerializeAndDeserialize(t *testing.T) {
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "serialize-test", Namespace: "default"},
	}
	var buf bytes.Buffer
	if err := K8sSerialize(cj, &buf); err != nil {
		t.Fatalf("K8sSerialize failed: %v", err)
	}

	target := &batchv1.CronJob{}
	obj, _, err := K8sDeserialize(buf.Bytes(), target)
	if err != nil {
		t.Fatalf("K8sDeserialize failed: %v", err)
	}
	decoded, ok := obj.(*batchv1.CronJob)
	if !ok || decoded.Name != "serialize-test" {
		t.Errorf("expected decoded cronjob serialize-test, got %+v", decoded)
	}
}
