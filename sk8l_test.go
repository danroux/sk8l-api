package main

import (
	"context"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"github.com/danroux/sk8l/protos"
	badger "github.com/dgraph-io/badger/v4"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

type MockK8sClient struct {
	*K8sClient
}

func (kc *MockK8sClient) GetAllJobs() *batchv1.JobList {
	return &batchv1.JobList{}
}

func (kc *MockK8sClient) GetAllJobsMapped() *protos.MappedJobs {
	return &protos.MappedJobs{}
}

func TestMeh(t *testing.T) {
	db := setupBadger(t)
	defer db.Close()

	cronjobList := &batchv1.CronJobList{
		Items: []batchv1.CronJob{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "cronjob1"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "cronjob2"},
			},
		},
	}

	// Put serialized cronjobs into Badger cache
	k8sClient := &MockK8sClient{}
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
	var gotCronjobs []*protos.CronjobResponse
	for {
		cj, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if status.Code(err) == codes.Canceled {
			log.Println("stream canceled", err)
			break
		}

		gotCronjobs = append(gotCronjobs, cj.Cronjobs...)
		if len(gotCronjobs) >= len(cronjobList.Items) {
			// Cancel context early to stop streaming
			cancel()
			break
		}
	}

	// Assert the response contains the cronjobs in cache
	if len(gotCronjobs) != len(cronjobList.Items) {
		t.Errorf("expected %d cronjobs, got %d", len(cronjobList.Items), len(gotCronjobs))
	}
	for i, cj := range gotCronjobs {
		if cj.Name != cronjobList.Items[i].Name {
			t.Errorf("expected cronjob name %q, got %q", cronjobList.Items[i].Name, cj.Name)
		}
	}
}
