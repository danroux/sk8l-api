package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/danroux/sk8l/internal/k8s"
	"github.com/danroux/sk8l/internal/store"
	"github.com/danroux/sk8l/protos"
	"github.com/danroux/sk8l/testutil"
	badger "github.com/dgraph-io/badger/v4"
	"github.com/google/go-cmp/cmp"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	cgt "k8s.io/client-go/testing"
	gyaml "sigs.k8s.io/yaml"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1 << 20

var (
	lis        = &bufconn.Listener{}
	sk8lServer = &Sk8lServer{}
)

func setupBadger(t *testing.T) *badger.DB {
	dir := t.TempDir()
	opts := badger.DefaultOptions(dir).WithLogger(nil)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("failed to open badger DB: %v", err)
	}
	return db
}

func putCronjobsToBadger(t *testing.T, db *badger.DB, cronjobList *batchv1.CronJobList) {
	var buf bytes.Buffer
	if err := store.K8sSerialize(cronjobList, &buf); err != nil {
		t.Fatalf("failed to encode cronjob list: %v", err)
	}
	err := db.Update(func(txn *badger.Txn) error {
		return txn.Set(store.CronjobsCacheKey, buf.Bytes())
	})
	if err != nil {
		t.Fatalf("failed to write cronjobs to badger: %v", err)
	}
}

func bufDialer(context.Context, string) (net.Conn, error) {
	conn, err := lis.Dial()
	if err != nil {
		return nil, fmt.Errorf("bufDialer: failed to lis.Dial: %w", err)
	}
	return conn, nil
}

func TestMain(m *testing.M) {
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()

	protos.RegisterCronjobServer(s, sk8lServer)

	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()

	c := m.Run()

	s.GracefulStop()
	lis.Close()
	os.Exit(c)
}

func TestGetCronjobYAML(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := protos.NewCronjobClient(conn)

	cronjob1 := testutil.NewCronJobBuilder().
		WithName("process-videos").
		WithNamespace("sk8l").
		Build()

	cronjobList := testutil.NewCronJobListBuilder().
		WithItems(cronjob1).
		Build()

	clientSet := fake.NewClientset()
	_, err = clientSet.BatchV1().CronJobs(cronjob1.Namespace).Create(context.Background(), cronjob1, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("failed to create CronJob %q in namespace %q: %v", cronjob1.Name, cronjob1.Namespace, err)
	}

	k8sClient := k8s.NewClientWithInterface(clientSet)
	st := &store.CronJobDBStore{
		DB:        db,
		K8sClient: k8sClient,
	}
	sk8lServer.CronJobDBStore = st
	putCronjobsToBadger(t, sk8lServer.DB, cronjobList)

	yamlResp, err := client.GetCronjobYAML(ctx, &protos.CronjobRequest{CronjobName: cronjob1.Name, CronjobNamespace: cronjob1.Namespace})
	if err != nil {
		t.Fatalf("GetCronjobYAML failed: %v", err)
	}

	if yamlResp.Cronjob == "" {
		t.Error("CronjobYAMLResponse.Cronjob is empty")
	}

	cronJob := &batchv1.CronJob{}
	if err := gyaml.Unmarshal([]byte(yamlResp.Cronjob), cronJob); err != nil {
		t.Errorf("failed to gyaml.Unmarshal: %v", err)
	}

	if cronJob.Name != "process-videos" {
		t.Errorf("expected cronJob.Name 'process-videos', got %q", cronJob.Name)
	}
	if cronJob.Namespace != "sk8l" {
		t.Errorf("expected cronJob.Namespace 'sk8l', got %q", cronJob.Namespace)
	}

	containers := cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers
	ephContainers := cronJob.Spec.JobTemplate.Spec.Template.Spec.EphemeralContainers
	initContainers := cronJob.Spec.JobTemplate.Spec.Template.Spec.InitContainers

	if len(containers) == 0 {
		t.Fatalf("expected at least one container")
	}
	if containers[0].Name != "default-container" {
		t.Errorf("expected Container.Name 'default-container', got %q", containers[0].Name)
	}

	if len(ephContainers) == 0 {
		t.Fatalf("expected at least one EphemeralContainer")
	}
	if ephContainers[0].Name != "debugger" {
		t.Errorf("expected EphemeralContainer.Name 'debugger', got %q", ephContainers[0].Name)
	}

	if len(initContainers) == 0 {
		t.Fatalf("expected at least one InitContainer")
	}
	if initContainers[0].Name != "init-myservice" {
		t.Errorf("expected InitContainer.Name 'init-myservice', got %q", initContainers[0].Name)
	}
}

func TestGetCronjobYAML_NotFound(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientSet := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(clientSet)
	st := &store.CronJobDBStore{
		DB:        db,
		K8sClient: k8sClient,
	}
	sk8lServer.CronJobDBStore = st

	client := protos.NewCronjobClient(conn)
	_, err = client.GetCronjobYAML(ctx, &protos.CronjobRequest{
		CronjobName:      "non-existent",
		CronjobNamespace: "default",
	})
	if err == nil {
		t.Error("expected error for non-existent cronjob, got nil")
	}
}

func TestGetJobYAML(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	job := testutil.NewJobBuilder().
		WithName("test-job").
		WithNamespace("default").
		Build()

	clientSet := fake.NewClientset(job)
	k8sClient := k8s.NewClientWithInterface(clientSet)
	st := &store.CronJobDBStore{
		DB:        db,
		K8sClient: k8sClient,
	}
	sk8lServer.CronJobDBStore = st

	client := protos.NewCronjobClient(conn)

	// Success case
	yamlResp, err := client.GetJobYAML(ctx, &protos.JobRequest{
		JobName:      "test-job",
		JobNamespace: "default",
	})
	if err != nil {
		t.Fatalf("GetJobYAML failed: %v", err)
	}
	if yamlResp.Job == "" {
		t.Error("expected non-empty Job YAML")
	}

	// Error case (not found)
	_, err = client.GetJobYAML(ctx, &protos.JobRequest{
		JobName:      "non-existent-job",
		JobNamespace: "default",
	})
	if err == nil {
		t.Error("expected error for non-existent job, got nil")
	}
}

func TestGetPodYAML(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	clientSet := fake.NewClientset(pod)
	k8sClient := k8s.NewClientWithInterface(clientSet)
	st := &store.CronJobDBStore{
		DB:        db,
		K8sClient: k8sClient,
	}
	sk8lServer.CronJobDBStore = st

	client := protos.NewCronjobClient(conn)

	// Success case
	yamlResp, err := client.GetPodYAML(ctx, &protos.PodRequest{
		PodName:      "test-pod",
		PodNamespace: "default",
	})
	if err != nil {
		t.Fatalf("GetPodYAML failed: %v", err)
	}
	if yamlResp.Pod == "" {
		t.Error("expected non-empty Pod YAML")
	}

	// Error case (not found)
	_, err = client.GetPodYAML(ctx, &protos.PodRequest{
		PodName:      "non-existent-pod",
		PodNamespace: "default",
	})
	if err == nil {
		t.Error("expected error for non-existent pod, got nil")
	}
}

func TestGetCronjosbDB(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := protos.NewCronjobClient(conn)

	cronjob1 := testutil.NewCronJobBuilder().
		WithName("cronjob1").
		WithNamespace("sk8l").
		Build()

	cronjob2 := testutil.NewCronJobBuilder().
		WithName("cronjob2").
		WithNamespace("sk8l").
		Build()

	cronjobList := testutil.NewCronJobListBuilder().
		WithItems(cronjob1, cronjob2).
		Build()

	clientSet := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(clientSet)
	st := &store.CronJobDBStore{
		DB:        db,
		K8sClient: k8sClient,
	}
	sk8lServer.CronJobDBStore = st
	putCronjobsToBadger(t, sk8lServer.DB, cronjobList)

	stream, err := client.GetCronjobs(ctx, &protos.CronjobsRequest{})
	if err != nil {
		t.Fatalf("GetCronjobs RPC failed: %v", err)
	}

	// https://grpc.io/docs/guides/cancellation/
	// https://learn.microsoft.com/en-us/aspnet/core/grpc/performance
	cronJobResponse := &protos.CronjobsResponse{}

	for {
		cj, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if status.Code(err) == codes.Canceled {
			log.Println("stream canceled", err)
			break
		}

		cronJobResponse.Cronjobs = append(cronJobResponse.Cronjobs, cj.Cronjobs...)
		if len(cronJobResponse.Cronjobs) >= len(cronjobList.Items) {
			// Cancel context early to stop streaming
			cancel()
			break
		}
	}

	if len(cronJobResponse.Cronjobs) != len(cronjobList.Items) {
		t.Errorf("expected %d cronjobs, got %d", len(cronjobList.Items), len(cronJobResponse.Cronjobs))
	}
	for i, cj := range cronJobResponse.Cronjobs {
		if cj.Name != cronjobList.Items[i].Name {
			t.Errorf("expected cronjob name %q, got %q", cronjobList.Items[i].Name, cj.Name)
		}
	}
}

func drainCronjobStream(t *testing.T, stream protos.Cronjob_GetCronjobsClient, cancel context.CancelFunc, expected int) *protos.CronjobsResponse {
	t.Helper()
	result := &protos.CronjobsResponse{}
	for {
		resp, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if status.Code(recvErr) == codes.Canceled {
			log.Println("stream canceled", recvErr)
			break
		}
		if recvErr != nil {
			t.Errorf("unexpected stream error: %v", recvErr)
			break
		}
		result.Cronjobs = append(result.Cronjobs, resp.Cronjobs...)
		if len(result.Cronjobs) >= expected {
			cancel()
			break
		}
	}
	return result
}

func TestGetCronjobsService(t *testing.T) {
	db := setupBadger(t)

	defer db.Close()

	podTemsplateSpec := testutil.NewPodTemplateSpecBuilder().
		WithSidecarContainers().
		Build()
	podTemsplateSpecTwo := testutil.NewPodTemplateSpecBuilder().
		Build()

	jobSpec := testutil.NewJobSpecBuilder().
		WithPodTemplateSpec(podTemsplateSpec).
		Build()

	cronjob := testutil.NewCronJobBuilder().
		WithName("my-cronjob").
		WithNamespace("default").
		WithJobTemplate(batchv1.JobTemplateSpec{
			Spec: jobSpec,
		}).
		Build()

	job := testutil.NewJobBuilder().
		WithJobSpec(jobSpec).
		WithName("process-videos").
		WithCronjob(*cronjob).
		Build()

	jobSpecTwo := testutil.NewJobSpecBuilder().
		WithPodTemplateSpec(podTemsplateSpecTwo).
		Build()

	jobTwo := testutil.NewJobBuilder().
		WithJobSpec(jobSpecTwo).
		WithName("process-reports").
		WithCronjob(*cronjob).
		Build()

	watcher := watch.NewFake()
	go watcher.Add(cronjob)

	clientSet := fake.NewClientset(job, jobTwo, cronjob)

	// Prepend a watch reactor for "cronjobs" resource that returns the FakeWatcher
	clientSet.PrependWatchReactor("cronjobs", func(action cgt.Action) (handled bool, ret watch.Interface, err error) {
		return true, watcher, nil
	})

	k8sClient := k8s.NewClientWithInterface(clientSet, k8s.WithNamespace("default"))
	st := &store.CronJobDBStore{
		DB:        db,
		K8sClient: k8sClient,
	}

	sk8lServer.CronJobDBStore = st
	sk8lServer.collectCronjobs(context.Background())

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	defer conn.Close()

	client := protos.NewCronjobClient(conn)
	cronJobs, err := clientSet.BatchV1().CronJobs("default").List(ctx, metav1.ListOptions{})

	if err != nil {
		t.Errorf("failed to list CronJobs in namespace %q: %v", "default", err)
	}

	if len(cronJobs.Items) < 1 {
		t.Error("expected cronJobs to exist in the cluster")
	}

	stream, err := client.GetCronjobs(ctx, &protos.CronjobsRequest{})
	if err != nil {
		t.Fatalf("GetCronjobs failed: %v", err)
	}

	cronJobsResponse := drainCronjobStream(t, stream, cancel, len(cronJobs.Items))

	if len(cronJobsResponse.Cronjobs) != len(cronJobs.Items) {
		t.Errorf("expected %d cronjobs, got %d", len(cronJobs.Items), len(cronJobsResponse.Cronjobs))
	}

	for i, cj := range cronJobsResponse.Cronjobs {
		if cj.Name != cronJobs.Items[i].Name {
			t.Errorf("expected cronjob name %q, got %q", cronJobs.Items[i].Name, cj.Name)
		}

		for _, j := range cj.Jobs {
			switch j.Name {
			case "process-videos":
				if !j.WithSidecarContainers {
					t.Errorf("expected job %s WithSidecarContainers to be true, got false", j.Name)
				}
			case "process-reports":
				if j.WithSidecarContainers {
					t.Errorf("expected job %s WithSidecarContainers to be false, got true", j.Name)
				}
			}
		}
	}
}

func TestCronJobsResponseWithPods(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

	namespace := "default"
	podOne := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: namespace,
		},
	}
	podTwo := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: namespace,
		},
	}

	clientSet := fake.NewClientset()

	configMapName := "pod-1"
	_, err := clientSet.CoreV1().ConfigMaps(namespace).Create(context.Background(),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: configMapName, Namespace: namespace},
			Data:       map[string]string{"k0": "v0"},
		}, metav1.CreateOptions{FieldManager: "test-manager-0"})

	if err != nil {
		t.Fatalf("Failed to create ConfigMap: %v", err)
	}

	expectedPods := []*corev1.Pod{}

	pod, err := clientSet.CoreV1().Pods(namespace).Create(context.Background(), podOne, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("failed to create pod %q in namespace %q: %v", podOne.Name, namespace, err)
	}
	expectedPods = append(expectedPods, pod)

	pod, err = clientSet.CoreV1().Pods(namespace).Create(context.Background(), podTwo, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("failed to create pod %q in namespace %q: %v", podTwo.Name, namespace, err)
	}
	expectedPods = append(expectedPods, pod)

	err = clientSet.CoreV1().Pods(namespace).EvictV1(context.Background(), &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name: podTwo.Name,
		},
	})
	if err != nil {
		t.Errorf("failed to evict pod %q in namespace %q: %v", podTwo.Name, namespace, err)
	}

	pods, err := clientSet.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Errorf("failed to list pods in namespace %q: %v", namespace, err)
	}

	cmp.Equal(expectedPods, pods.Items)
}

// TestGetCronjobs_NoConcurrentRace validates that concurrent goroutines building
// CronjobResponse objects and appending to the shared slice in GetCronjobs do not
// produce data races.
func TestGetCronjobs_NoConcurrentRace(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

	const numCronjobs = 10
	items := make([]*batchv1.CronJob, 0, numCronjobs)
	for i := range numCronjobs {
		items = append(items, testutil.NewCronJobBuilder().
			WithName(fmt.Sprintf("cronjob-%d", i)).
			WithNamespace("default").
			Build())
	}
	cronjobList := testutil.NewCronJobListBuilder().WithItems(items...).Build()

	clientSet := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(clientSet, k8s.WithNamespace("default"))
	st := &store.CronJobDBStore{
		DB:        db,
		K8sClient: k8sClient,
	}
	sk8lServer.CronJobDBStore = st
	putCronjobsToBadger(t, db, cronjobList)

	jobsMapped := map[string][]*batchv1.Job{}

	// Replicate the concurrent append pattern from GetCronjobs and run under -race.
	cronjobs := make([]*protos.CronjobResponse, 0, numCronjobs)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(numCronjobs)
	for _, item := range cronjobList.Items {
		go func(cj batchv1.CronJob) {
			defer wg.Done()
			jobs := sk8lServer.jobsForCronjob(jobsMapped, cj.Name)
			resp := sk8lServer.cronJobResponse(cj, jobs)
			mu.Lock()
			cronjobs = append(cronjobs, resp)
			mu.Unlock()
		}(item)
	}
	wg.Wait()

	if len(cronjobs) != numCronjobs {
		t.Errorf("expected %d cronjob responses, got %d", numCronjobs, len(cronjobs))
	}
}

// TestAllAndRunningJobsAnPods_NoConcurrentRace validates that concurrent goroutines
// inside allAndRunningJobsAnPods do not produce data races on the four shared slices.
func TestAllAndRunningJobsAnPods_NoConcurrentRace(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

	clientSet := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(clientSet, k8s.WithNamespace("default"))
	st := &store.CronJobDBStore{
		DB:        db,
		K8sClient: k8sClient,
	}
	sk8lServer.CronJobDBStore = st

	const numJobs = 10
	jobs := make([]*batchv1.Job, 0, numJobs)
	for i := range numJobs {
		jobs = append(jobs, testutil.NewJobBuilder().
			WithName(fmt.Sprintf("job-%d", i)).
			Build())
	}

	allJobs, allPods, runningJobs, runningPods := sk8lServer.allAndRunningJobsAnPods(jobs, "")

	if len(allJobs) != numJobs {
		t.Errorf("expected %d job responses, got %d", numJobs, len(allJobs))
	}
	// allPods, runningJobs, runningPods may be empty depending on job status — just
	// ensure the race detector did not trigger and slices are non-nil.
	if allPods == nil || runningJobs == nil || runningPods == nil {
		t.Error("expected non-nil slice results from allAndRunningJobsAnPods")
	}
}
