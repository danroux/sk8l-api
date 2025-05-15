package main

import (
	"context"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"github.com/danroux/sk8l/protos"
	"github.com/danroux/sk8l/testutil"
	badger "github.com/dgraph-io/badger/v4"
	gyaml "github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	cgt "k8s.io/client-go/testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
)

const bufSize = 1 << 20

var lis *bufconn.Listener

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
	cronjobListV2 := protoadapt.MessageV2Of(cronjobList)
	data, err := proto.Marshal(cronjobListV2)
	if err != nil {
		t.Fatalf("failed to marshal cronjob list: %v", err)
	}

	err = db.Update(func(txn *badger.Txn) error {
		return txn.Set(cronjobsCacheKey, data)
	})
	if err != nil {
		t.Fatalf("failed to write cronjobs to badger: %v", err)
	}
}

var sk8lServer Sk8lServer

func init() {
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()

	protos.RegisterCronjobServer(s, &sk8lServer)

	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()
}

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

func TestGetCronjobYAML(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	defer conn.Close()

	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

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

	k8sClient := &K8sClient{
		Interface: clientSet,
	}
	store := &CronjobDBStore{
		DB:        db,
		K8sClient: k8sClient,
	}
	sk8lServer.CronjobDBStore = store
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

func TestGetCronjosbDB(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	defer conn.Close()

	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

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

		// Put serialized cronjobs into Badger cache
	clientSet := fake.NewClientset()
	k8sClient := &K8sClient{
		Interface: clientSet,
	}
	store := &CronjobDBStore{
		DB:        db,
		K8sClient: k8sClient,
	}
	sk8lServer.CronjobDBStore = store
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
		if err == io.EOF {
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

	// Assert the response contains the cronjobs in cache
	if len(cronJobResponse.Cronjobs) != len(cronjobList.Items) {
		t.Errorf("expected %d cronjobs, got %d", len(cronjobList.Items), len(cronJobResponse.Cronjobs))
	}
	for i, cj := range cronJobResponse.Cronjobs {
		if cj.Name != cronjobList.Items[i].Name {
			t.Errorf("expected cronjob name %q, got %q", cronjobList.Items[i].Name, cj.Name)
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

	clientSet := fake.NewClientset(podOne, podTwo)

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

	expectedPods = append(expectedPods, pod)

	pod, err = clientSet.CoreV1().Pods(namespace).Create(context.Background(), podTwo, metav1.CreateOptions{})

	expectedPods = append(expectedPods, pod)

	err = clientSet.CoreV1().Pods(namespace).EvictV1(context.Background(), &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name: podTwo.Name,
		},
	})

	pods, err := clientSet.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})

	cmp.Equal(expectedPods, pods.Items)
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

	// watcher setup

	// Prepend a watch reactor for "cronjobs" resource that returns the FakeWatcher
	clientSet.PrependWatchReactor("cronjobs", func(action cgt.Action) (handled bool, ret watch.Interface, err error) {
		return true, watcher, nil
	})
	// clientSet.PrependWatchReactor("cronjobs", cgt.DefaultWatchReactor(watcher, nil))
	// watcher setup

	k8sClient := &K8sClient{
		namespace: "default",
		Interface: clientSet,
	}
	store := &CronjobDBStore{
		DB:        db,
		K8sClient: k8sClient,
	}

	sk8lServer.CronjobDBStore = store
	sk8lServer.collectCronjobs()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	ctx = context.Background()
	cronJobs, err := clientSet.BatchV1().CronJobs("default").List(context.Background(), metav1.ListOptions{})

	if len(cronJobs.Items) < 1 {
		t.Error("expected cronJobs to exist in the cluster")
	}

	// jobs, err := clientSet.BatchV1().Jobs("default").List(ctx, metav1.ListOptions{})
	stream, err := client.GetCronjobs(ctx, &protos.CronjobsRequest{})
	if err != nil {
		t.Fatalf("GetCronjobs RPC failed: %v", err)
	}

	cronJobsResponse := &protos.CronjobsResponse{}

	for {
		cronJobsResponse, err = stream.Recv()
		if err == io.EOF {
			break
		}

		if status.Code(err) == codes.Canceled {
			log.Println("stream canceled", err)
			break
		}

		// cronJobResponse.Cronjobs = append(cronJobResponse.Cronjobs, cronJobsResponse.Cronjobs...)
		if len(cronJobsResponse.Cronjobs) >= len(cronJobs.Items) {
			// Cancel context early to stop streaming
			cancel()
			break
		}
	}

	// Assert the response contains the cronjobs in cache
	if len(cronJobsResponse.Cronjobs) != len(cronJobs.Items) {
		t.Errorf("expected %d cronjobs, got %d", len(cronJobs.Items), len(cronJobsResponse.Cronjobs))
	}

	for i, cj := range cronJobsResponse.Cronjobs {
		if cj.Name != cronJobs.Items[i].Name {
			t.Errorf("expected cronjob name %q, got %q", cronJobs.Items[i].Name, cj.Name)
		}

		if cj.Jobs[0].WithSidecarContainers != true {
			t.Error("April true", cj.Jobs[0].WithSidecarContainers)
		}

		if cj.Jobs[1].WithSidecarContainers != false {
			t.Error("April true", cj.Jobs[1].WithSidecarContainers)
		}

	}

}
