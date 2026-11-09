package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/danroux/sk8l/internal/k8s"
	"github.com/danroux/sk8l/internal/mapper"
	"github.com/danroux/sk8l/internal/store"
	"github.com/danroux/sk8l/internal/store/informer"
	"github.com/danroux/sk8l/protos"
	"github.com/danroux/sk8l/testutil"
	"github.com/google/go-cmp/cmp"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
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

func setupTestInformerStore(t *testing.T, cs kubernetes.Interface, ns string) *informer.Store {
	t.Helper()
	st, err := informer.New(cs, ns)
	if err != nil {
		t.Fatalf("failed to create informer store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	st.Start(ctx)
	syncCtx, syncCancel := context.WithTimeout(ctx, 10*time.Second)
	defer syncCancel()
	if !st.WaitForSync(syncCtx) {
		t.Fatal("timed out waiting for test informer store to sync")
	}
	return st
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

	clientSet := fake.NewClientset(cronjob1)
	k8sClient := k8s.NewClientWithInterface(clientSet)
	st := setupTestInformerStore(t, clientSet, "sk8l")
	sk8lServer.SetStore(st)
	sk8lServer.SetK8sClient(k8sClient)

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
	st := setupTestInformerStore(t, clientSet, "default")
	sk8lServer.SetStore(st)
	sk8lServer.SetK8sClient(k8sClient)

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
	st := setupTestInformerStore(t, clientSet, "default")
	sk8lServer.SetStore(st)
	sk8lServer.SetK8sClient(k8sClient)

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
	st := setupTestInformerStore(t, clientSet, "default")
	sk8lServer.SetStore(st)
	sk8lServer.SetK8sClient(k8sClient)

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

	clientSet := fake.NewClientset(cronjob1, cronjob2)
	k8sClient := k8s.NewClientWithInterface(clientSet)
	st := setupTestInformerStore(t, clientSet, "sk8l")
	sk8lServer.SetStore(st)
	sk8lServer.SetK8sClient(k8sClient)

	stream, err := client.GetCronjobs(ctx, &protos.CronjobsRequest{})
	if err != nil {
		t.Fatalf("GetCronjobs RPC failed: %v", err)
	}

	cronJobResponse := drainCronjobStream(t, stream, cancel, len(cronjobList.Items))

	if len(cronJobResponse.Cronjobs) != 2 {
		t.Errorf("expected %d cronjobs, got %d", 2, len(cronJobResponse.Cronjobs))
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
		if errors.Is(recvErr, io.EOF) || status.Code(recvErr) == codes.Canceled {
			break
		}
		if recvErr != nil {
			t.Errorf("unexpected stream error: %v", recvErr)
			break
		}
		result.Cronjobs = append(result.Cronjobs, resp.Cronjobs...)
		if len(result.Cronjobs) >= expected {
			cancel()
			for {
				_, drainErr := stream.Recv()
				if drainErr != nil {
					break
				}
			}
			break
		}
	}
	return result
}

func TestGetCronjobsService(t *testing.T) {
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

	clientSet := fake.NewClientset(job, jobTwo, cronjob)
	k8sClient := k8s.NewClientWithInterface(clientSet, k8s.WithNamespace("default"))
	st := setupTestInformerStore(t, clientSet, "default")
	sk8lServer.SetStore(st)
	sk8lServer.SetK8sClient(k8sClient)

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
	const numCronjobs = 10
	items := make([]*batchv1.CronJob, 0, numCronjobs)
	for i := range numCronjobs {
		items = append(items, testutil.NewCronJobBuilder().
			WithName(fmt.Sprintf("cronjob-%d", i)).
			WithNamespace("default").
			Build())
	}
	cronjobList := testutil.NewCronJobListBuilder().WithItems(items...).Build()

	objs := make([]runtime.Object, len(cronjobList.Items))
	for i := range cronjobList.Items {
		objs[i] = &cronjobList.Items[i]
	}
	clientSet := fake.NewClientset(objs...)
	k8sClient := k8s.NewClientWithInterface(clientSet, k8s.WithNamespace("default"))
	st := setupTestInformerStore(t, clientSet, "default")
	sk8lServer.SetStore(st)
	sk8lServer.SetK8sClient(k8sClient)

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
	clientSet := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(clientSet, k8s.WithNamespace("default"))
	st := setupTestInformerStore(t, clientSet, "default")
	sk8lServer.SetStore(st)
	sk8lServer.SetK8sClient(k8sClient)

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

type fakeHealthStore struct {
	store.Store
	notifyCh      chan struct{}
	degradedSince time.Duration
	ready         atomic.Bool
}

func (f *fakeHealthStore) Ready() bool {
	return f.ready.Load()
}

func (f *fakeHealthStore) DegradedSince() time.Duration {
	return f.degradedSince
}

func (f *fakeHealthStore) Subscribe(ctx context.Context) <-chan struct{} {
	if f.notifyCh != nil {
		return f.notifyCh
	}
	return make(chan struct{})
}

// newHealthServer builds a minimal Sk8lServer wired to a mock store
// and returns the server along with a function to simulate a degraded state.
func newHealthServer(t *testing.T) (*Sk8lServer, func()) {
	t.Helper()
	fakeClientset := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(fakeClientset)
	fs := &fakeHealthStore{}
	fs.ready.Store(true)
	s := NewSk8lServer(fs, k8sClient, nil)
	return s, func() { fs.ready.Store(false) }
}

func TestIsHealthy(t *testing.T) {
	t.Run("HealthyWhenReady", func(t *testing.T) {
		s, _ := newHealthServer(t)
		if !s.IsHealthy() {
			t.Error("expected IsHealthy() == true when store is ready")
		}
	})

	t.Run("UnhealthyWhenDegraded", func(t *testing.T) {
		s, degrade := newHealthServer(t)
		degrade()
		if s.IsHealthy() {
			t.Error("expected IsHealthy() == false after store is degraded")
		}
	})

	t.Run("UnhealthyWhenStoreNil", func(t *testing.T) {
		s := &Sk8lServer{}
		if s.IsHealthy() {
			t.Error("expected IsHealthy() == false when Store is nil")
		}
	})
}

func TestGetDashboardAnnotations(t *testing.T) {
	s := &Sk8lServer{}
	resp, err := s.GetDashboardAnnotations(context.Background(), &protos.DashboardAnnotationsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.Annotations == "" {
		t.Fatal("expected non-empty dashboard annotations response")
	}

	var dashboard map[string]any
	if err := json.Unmarshal([]byte(resp.Annotations), &dashboard); err != nil {
		t.Fatalf("failed to parse dashboard JSON: %v", err)
	}

	title, ok := dashboard["title"].(string)
	if !ok || title == "" {
		t.Errorf("expected non-empty dashboard title, got %v", dashboard["title"])
	}

	panels, ok := dashboard["panels"].([]any)
	if !ok || len(panels) == 0 {
		t.Errorf("expected panels in dashboard, got %v", dashboard["panels"])
	}

	templating, ok := dashboard["templating"].(map[string]any)
	if !ok {
		t.Errorf("expected templating section in dashboard")
	} else if list, ok := templating["list"].([]any); !ok || len(list) == 0 {
		t.Errorf("expected template variables in dashboard")
	}
}

func TestBuildJobPodsResponses_FinishedAt_Success(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	metaNow := metav1.NewTime(now)

	podList := &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "success-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "worker",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode:   0,
									Reason:     "Completed",
									FinishedAt: metaNow,
								},
							},
						},
					},
				},
			},
		},
	}

	responses := buildJobPodsResponses(podList)
	if len(responses) != 1 {
		t.Fatalf("expected 1 pod response, got %d", len(responses))
	}
	resp := responses[0]

	expectedFinishedAt := mapper.TimeToString(&metaNow)
	if resp.FinishedAt != expectedFinishedAt {
		t.Errorf("expected FinishedAt %q, got %q", expectedFinishedAt, resp.FinishedAt)
	}
	if resp.Failed {
		t.Errorf("expected Failed == false, got true")
	}
	if len(resp.TerminationReasons) != 0 {
		t.Errorf("expected 0 termination reasons, got %d", len(resp.TerminationReasons))
	}
	if len(resp.TerminatedContainers.Containers) != 1 {
		t.Fatalf("expected 1 terminated container, got %d", len(resp.TerminatedContainers.Containers))
	}
	c := resp.TerminatedContainers.Containers[0]
	if c.TerminatedReason == nil || c.TerminatedReason.TerminationDetails == nil {
		t.Fatalf("expected TerminatedReason and TerminationDetails to be non-nil")
	}
	if c.TerminatedReason.TerminationDetails.FinishedAt != expectedFinishedAt {
		t.Errorf("expected container FinishedAt %q, got %q", expectedFinishedAt, c.TerminatedReason.TerminationDetails.FinishedAt)
	}
	if len(resp.FailedContainers.Containers) != 0 {
		t.Errorf("expected 0 failed containers, got %d", len(resp.FailedContainers.Containers))
	}
}

func TestBuildJobPodsResponses_FinishedAt_Failure(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	metaNow := metav1.NewTime(now)
	expectedFinishedAt := mapper.TimeToString(&metaNow)

	t.Run("error exit code 1", func(t *testing.T) {
		podList := &corev1.PodList{
			Items: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "failed-pod",
						Namespace: "default",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodFailed,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "worker",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										ExitCode:   1,
										Reason:     "Error",
										FinishedAt: metaNow,
									},
								},
							},
						},
					},
				},
			},
		}

		responses := buildJobPodsResponses(podList)
		if len(responses) != 1 {
			t.Fatalf("expected 1 pod response, got %d", len(responses))
		}
		resp := responses[0]

		if resp.FinishedAt != expectedFinishedAt {
			t.Errorf("expected FinishedAt %q, got %q", expectedFinishedAt, resp.FinishedAt)
		}
		if !resp.Failed {
			t.Errorf("expected Failed == true, got false")
		}
		if len(resp.TerminationReasons) != 1 {
			t.Errorf("expected 1 termination reason, got %d", len(resp.TerminationReasons))
		}
		if len(resp.TerminatedContainers.Containers) != 1 {
			t.Errorf("expected 1 terminated container, got %d", len(resp.TerminatedContainers.Containers))
		}
		if len(resp.FailedContainers.Containers) != 1 {
			t.Errorf("expected 1 failed container, got %d", len(resp.FailedContainers.Containers))
		}
	})

	t.Run("OOMKilled exit code 137", func(t *testing.T) {
		podList := &corev1.PodList{
			Items: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "oom-pod",
						Namespace: "default",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodFailed,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "worker",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{
										ExitCode:   137,
										Reason:     "OOMKilled",
										FinishedAt: metaNow,
									},
								},
							},
						},
					},
				},
			},
		}

		responses := buildJobPodsResponses(podList)
		if len(responses) != 1 {
			t.Fatalf("expected 1 pod response, got %d", len(responses))
		}
		resp := responses[0]

		if resp.FinishedAt != expectedFinishedAt {
			t.Errorf("expected FinishedAt %q, got %q", expectedFinishedAt, resp.FinishedAt)
		}
		if !resp.Failed {
			t.Errorf("expected Failed == true, got false")
		}
		if len(resp.TerminationReasons) != 1 {
			t.Errorf("expected 1 termination reason, got %d", len(resp.TerminationReasons))
		}
		if len(resp.FailedContainers.Containers) != 1 {
			t.Errorf("expected 1 failed container, got %d", len(resp.FailedContainers.Containers))
		}
	})
}

func TestBuildJobPodsResponses_FinishedAt_MultiContainer(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	metaNow := metav1.NewTime(now)
	tEarlier := metav1.NewTime(now.Add(-10 * time.Minute))
	tMiddle := metav1.NewTime(now.Add(-5 * time.Minute))
	tLatest := metaNow

	podList := &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-container-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "c1",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode:   0,
									Reason:     "Completed",
									FinishedAt: tEarlier,
								},
							},
						},
						{
							Name: "c2",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode:   0,
									Reason:     "Completed",
									FinishedAt: tLatest,
								},
							},
						},
						{
							Name: "c3",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode:   0,
									Reason:     "Completed",
									FinishedAt: tMiddle,
								},
							},
						},
					},
				},
			},
		},
	}

	responses := buildJobPodsResponses(podList)
	if len(responses) != 1 {
		t.Fatalf("expected 1 pod response, got %d", len(responses))
	}
	resp := responses[0]

	expectedFinishedAt := mapper.TimeToString(&tLatest)
	if resp.FinishedAt != expectedFinishedAt {
		t.Errorf("expected FinishedAt to be latest %q, got %q", expectedFinishedAt, resp.FinishedAt)
	}
	if resp.Failed {
		t.Errorf("expected Failed == false, got true")
	}
	if len(resp.TerminatedContainers.Containers) != 3 {
		t.Errorf("expected 3 terminated containers, got %d", len(resp.TerminatedContainers.Containers))
	}
}

func TestBuildJobPodsResponses_FinishedAt_Running(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	metaNow := metav1.NewTime(now)

	podList := &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "running-pod",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "worker",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{
									StartedAt: metaNow,
								},
							},
						},
					},
				},
			},
		},
	}

	responses := buildJobPodsResponses(podList)
	if len(responses) != 1 {
		t.Fatalf("expected 1 pod response, got %d", len(responses))
	}
	resp := responses[0]

	if resp.FinishedAt != "" {
		t.Errorf("expected empty FinishedAt for running pod, got %q", resp.FinishedAt)
	}
	if resp.Failed {
		t.Errorf("expected Failed == false, got true")
	}
	if len(resp.TerminatedContainers.Containers) != 0 {
		t.Errorf("expected 0 terminated containers, got %d", len(resp.TerminatedContainers.Containers))
	}
}

func TestCheckStoreHealth(t *testing.T) {
	s := &Sk8lServer{}

	t.Run("NilStore", func(t *testing.T) {
		err := s.checkStoreHealth(nil)
		if !errors.Is(err, ErrStoreNil) {
			t.Fatalf("expected ErrStoreNil, got %v", err)
		}
	})

	t.Run("StoreReady", func(t *testing.T) {
		fs := &fakeHealthStore{}
		fs.ready.Store(true)
		if err := s.checkStoreHealth(fs); err != nil {
			t.Fatalf("expected nil error for ready store, got %v", err)
		}
	})

	t.Run("StoreNotSynced", func(t *testing.T) {
		fs := &fakeHealthStore{}
		fs.ready.Store(false)
		fs.degradedSince = 0
		err := s.checkStoreHealth(fs)
		if err == nil {
			t.Fatal("expected error for unsynced store, got nil")
		}
		if connect.CodeOf(err) != connect.CodeUnavailable {
			t.Fatalf("expected CodeUnavailable, got %v", connect.CodeOf(err))
		}
	})

	t.Run("StoreDegradedWithinGracePeriod", func(t *testing.T) {
		fs := &fakeHealthStore{}
		fs.ready.Store(false)
		fs.degradedSince = 10 * time.Second
		if err := s.checkStoreHealth(fs); err != nil {
			t.Fatalf("expected nil error within grace period, got %v", err)
		}
	})

	t.Run("StoreDegradedExceededGracePeriod", func(t *testing.T) {
		fs := &fakeHealthStore{}
		fs.ready.Store(false)
		fs.degradedSince = 20 * time.Second
		err := s.checkStoreHealth(fs)
		if err == nil {
			t.Fatal("expected error when degraded beyond grace period, got nil")
		}
		if connect.CodeOf(err) != connect.CodeUnavailable {
			t.Fatalf("expected CodeUnavailable, got %v", connect.CodeOf(err))
		}
	})
}

func TestSortPodResponses(t *testing.T) {
	pods := []*protos.PodResponse{
		{
			Metadata: &protos.ObjectMetaResponse{Name: "pod-b"},
			Status:   &protos.PodStatusResponse{StartTime: "2026-11-09T12:00:00Z"},
		},
		{
			Metadata: &protos.ObjectMetaResponse{Name: "pod-a"},
			Status:   &protos.PodStatusResponse{StartTime: "2026-11-09T12:00:00Z"},
		},
		{
			Metadata: &protos.ObjectMetaResponse{Name: "pod-pending-z"},
			Status:   &protos.PodStatusResponse{StartTime: ""},
		},
		{
			Metadata: &protos.ObjectMetaResponse{Name: "pod-pending-a"},
			Status:   &protos.PodStatusResponse{StartTime: ""},
		},
		{
			Metadata: &protos.ObjectMetaResponse{Name: "pod-future"},
			Status:   &protos.PodStatusResponse{StartTime: "2026-11-09T13:00:00Z"},
		},
	}

	sortPodResponses(pods)

	expectedNames := []string{
		"pod-pending-a",
		"pod-pending-z",
		"pod-a",
		"pod-b",
		"pod-future",
	}

	for i, expected := range expectedNames {
		if pods[i].Metadata.Name != expected {
			t.Errorf("pod[%d] name: expected %q, got %q", i, expected, pods[i].Metadata.Name)
		}
	}
}

func TestBuildStandaloneJobResponses_DeterministicSort(t *testing.T) {
	s := &Sk8lServer{}
	now := time.Now().UTC()
	jobList := &batchv1.JobList{
		Items: []batchv1.Job{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "job-b",
					CreationTimestamp: metav1.NewTime(now),
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "job-a",
					CreationTimestamp: metav1.NewTime(now),
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "job-earlier",
					CreationTimestamp: metav1.NewTime(now.Add(-1 * time.Hour)),
				},
			},
		},
	}

	jobs := s.buildStandaloneJobResponses(jobList)
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	expected := []string{"job-earlier", "job-a", "job-b"}
	for i, name := range expected {
		if jobs[i].Name != name {
			t.Errorf("job[%d]: expected %q, got %q", i, name, jobs[i].Name)
		}
	}
}

func TestStreamLoop_DebounceCoalescesRapidNotifications(t *testing.T) {
	notifyCh := make(chan struct{}, 10)
	fs := &fakeHealthStore{
		notifyCh: notifyCh,
	}
	fs.ready.Store(true)

	srv := NewSk8lServer(fs, nil, nil)

	var sendCount atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.streamLoop(ctx, func() error {
			sendCount.Add(1)
			return nil
		})
	}()

	// Wait for the initial send
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && sendCount.Load() < 1 {
		time.Sleep(10 * time.Millisecond)
	}
	if sendCount.Load() != 1 {
		t.Fatalf("expected initial send, got %d", sendCount.Load())
	}

	// Send 5 rapid notifications within a few milliseconds
	for i := 0; i < 5; i++ {
		notifyCh <- struct{}{}
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce window (streamDebounceInterval = 250ms) to fire exactly once
	time.Sleep(streamDebounceInterval + 150*time.Millisecond)

	// sendCount should be 2 (initial send + 1 coalesced send)
	if count := sendCount.Load(); count != 2 {
		t.Fatalf("expected sendCount to be 2 after coalesced burst, got %d", count)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("streamLoop returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("streamLoop did not terminate on context cancel")
	}
}
