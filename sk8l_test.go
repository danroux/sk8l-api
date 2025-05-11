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

// func TestGetCronjobYAML(t *testing.T) {
//      ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//      defer cancel()

//      conn, err := grpc.NewClient(
//              "passthrough:///bufnet",
//              grpc.WithContextDialer(bufDialer),
//              grpc.WithTransportCredentials(insecure.NewCredentials()),
//      )
//      if err != nil {
//              t.Fatalf("Failed to create client: %v", err)
//      }
//      defer conn.Close()

//      client := protos.NewCronjobClient(conn)

//      resp, err := client.GetCronjobYAML(ctx, &protos.CronjobRequest{CronjobName: "test", CronjobNamespace: "sk8l"})
//      if err != nil {
//              t.Fatalf("GetCronjobYAML failed: %v", err)
//      }

//      if resp.Cronjob == "" {
//              t.Error("YamlContent is empty")
//      }
// }

func TestGetCronjosbDB(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

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

func TestGetCronjobsService(t *testing.T) {
	db := setupBadger(t)

	defer db.Close()

	podOne := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "default",
		},
	}
	podTwo := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: "default",
		},
	}

	// Build JobSpec with containers
	jobSpec := testutil.NewJobSpecBuilder().
		Build()

	job := testutil.NewJobBuilder().WithJobSpec(jobSpec).Build()

	cronjob := testutil.NewCronJobBuilder().
		WithName("my-cronjob").
		WithNamespace("default").
		WithJobTemplate(batchv1.JobTemplateSpec{
			Spec: jobSpec,
		}).
		Build()

	watcher := watch.NewFake()
	go watcher.Add(cronjob)

	clientSet := fake.NewClientset(podOne, podTwo, job, cronjob)

	// watcher setup

	// Prepend a watch reactor for "cronjobs" resource that returns the FakeWatcher
	clientSet.PrependWatchReactor("cronjobs", func(action cgt.Action) (handled bool, ret watch.Interface, err error) {
		return true, watcher, nil
	})
	// clientSet.PrependWatchReactor("cronjobs", cgt.DefaultWatchReactor(watcher, nil))
	// watcher setup

	name := "pod-1"
	namespace := "default"
	_, err := clientSet.CoreV1().ConfigMaps("default").Create(context.Background(),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Data:       map[string]string{"k0": "v0"},
		}, metav1.CreateOptions{FieldManager: "test-manager-0"})

	expectedPods := []*corev1.Pod{}

	pod, err := clientSet.CoreV1().Pods("default").Create(context.Background(), podOne, metav1.CreateOptions{})

	expectedPods = append(expectedPods, pod)

	pod, err = clientSet.CoreV1().Pods("default").Create(context.Background(), podTwo, metav1.CreateOptions{})

	expectedPods = append(expectedPods, pod)

	err = clientSet.CoreV1().Pods("default").EvictV1(context.Background(), &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name: podTwo.Name,
		},
	})

	pods, err := clientSet.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})

	cmp.Equal(expectedPods, pods.Items)

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
		if len(cronJobResponse.Cronjobs) >= len(cronJobs.Items) {
			// Cancel context early to stop streaming
			cancel()
			break
		}
	}

	// Assert the response contains the cronjobs in cache
	if len(cronJobResponse.Cronjobs) != len(cronJobs.Items) {
		t.Errorf("expected %d cronjobs, got %d", len(cronJobs.Items), len(cronJobResponse.Cronjobs))
	}
	for i, cj := range cronJobResponse.Cronjobs {
		if cj.Name != cronJobs.Items[i].Name {
			t.Errorf("expected cronjob name %q, got %q", cronJobs.Items[i].Name, cj.Name)
		}
	}

}
