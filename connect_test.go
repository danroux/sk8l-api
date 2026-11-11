package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/danroux/sk8l/internal/k8s"
	"github.com/danroux/sk8l/protos"
	"github.com/danroux/sk8l/protos/protosconnect"
	"github.com/danroux/sk8l/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	bufSize                   = 1 << 20
	testConnectNamespace      = "sk8l"
	testConnectCronjobName    = "process-videos"
	testConnectJobName        = "standalone-job"
	testConnectPodName        = "standalone-pod"
	testConnectBaseURL        = "http://bufnet"
	testConnectOrigin         = "https://localhost:8001"
	testTLSServerName         = "localhost"
	testConnectTimeoutSeconds = 5
	testConnectTimeout        = testConnectTimeoutSeconds * time.Second
)

var (
	errTestAlreadyConnect = errors.New("already connect")
	errTestUnclassified   = errors.New("boom")
)

// newBufconnHTTPClient serves handler over an in-memory listener and returns a client wired to it.
func newBufconnHTTPClient(t *testing.T, handler http.Handler) *http.Client {
	t.Helper()

	bl := bufconn.Listen(bufSize)
	httpSrv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: ReadTimeout,
	}
	go func() {
		_ = httpSrv.Serve(bl)
	}()
	t.Cleanup(func() {
		_ = httpSrv.Close()
		_ = bl.Close()
	})

	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				conn, err := bl.DialContext(ctx)
				if err != nil {
					return nil, fmt.Errorf("connect_test: failed to dial bufconn: %w", err)
				}
				return conn, nil
			},
		},
	}
}

func newConnectTestEnvWithServer(t *testing.T, srv *Sk8lServer) (*http.ServeMux, *http.Client) {
	t.Helper()

	mux := &http.ServeMux{}
	setupHTTPRoutes(mux, srv, nil)
	return mux, newBufconnHTTPClient(t, mux)
}

func newConnectTestEnv(t *testing.T, objs ...runtime.Object) (*http.ServeMux, *http.Client) {
	t.Helper()
	clientSet := fake.NewClientset(objs...)
	k8sClient := k8s.NewClientWithInterface(clientSet)
	st := setupTestInformerStore(t, clientSet, testConnectNamespace)
	srv := NewSk8lServer(st, k8sClient, nil)
	return newConnectTestEnvWithServer(t, srv)
}

func TestConnectUnaryRPCs(t *testing.T) {
	cronjob := testutil.NewCronJobBuilder().
		WithName(testConnectCronjobName).
		WithNamespace(testConnectNamespace).
		Build()
	job := testutil.NewJobBuilder().
		WithName(testConnectJobName).
		WithNamespace(testConnectNamespace).
		Build()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConnectPodName,
			Namespace: testConnectNamespace,
		},
	}

	protocols := []struct {
		name string
		opts []connect.ClientOption
	}{
		{name: "Connect", opts: nil},
		{name: "gRPC-Web", opts: []connect.ClientOption{connect.WithGRPCWeb()}},
	}

	for _, protoTC := range protocols {
		t.Run(protoTC.name, func(t *testing.T) {
			_, httpClient := newConnectTestEnv(t, cronjob, job, pod)
			client := protosconnect.NewCronjobClient(httpClient, testConnectBaseURL, protoTC.opts...)
			ctx, cancel := context.WithTimeout(context.Background(), testConnectTimeout)
			defer cancel()

			annResp, err := client.GetDashboardAnnotations(ctx, connect.NewRequest(&protos.DashboardAnnotationsRequest{}))
			if err != nil {
				t.Fatalf("GetDashboardAnnotations failed: %v", err)
			}
			if annResp.Msg.GetAnnotations() == "" {
				t.Error("expected non-empty dashboard annotations")
			}

			cjResp, err := client.GetCronjobYAML(ctx, connect.NewRequest(&protos.CronjobRequest{
				CronjobName:      testConnectCronjobName,
				CronjobNamespace: testConnectNamespace,
			}))
			if err != nil {
				t.Fatalf("GetCronjobYAML failed: %v", err)
			}
			if !strings.Contains(cjResp.Msg.GetCronjob(), testConnectCronjobName) {
				t.Errorf("expected cronjob YAML to contain %s, got %q", testConnectCronjobName, cjResp.Msg.GetCronjob())
			}

			jobResp, err := client.GetJobYAML(ctx, connect.NewRequest(&protos.JobRequest{
				JobName:      testConnectJobName,
				JobNamespace: testConnectNamespace,
			}))
			if err != nil {
				t.Fatalf("GetJobYAML failed: %v", err)
			}
			if !strings.Contains(jobResp.Msg.GetJob(), testConnectJobName) {
				t.Errorf("expected job YAML to contain %s, got %q", testConnectJobName, jobResp.Msg.GetJob())
			}

			podResp, err := client.GetPodYAML(ctx, connect.NewRequest(&protos.PodRequest{
				PodName:      testConnectPodName,
				PodNamespace: testConnectNamespace,
			}))
			if err != nil {
				t.Fatalf("GetPodYAML failed: %v", err)
			}
			if !strings.Contains(podResp.Msg.GetPod(), testConnectPodName) {
				t.Errorf("expected pod YAML to contain %s, got %q", testConnectPodName, podResp.Msg.GetPod())
			}
		})
	}
}

func TestConnectStreamingRPCs(t *testing.T) {
	cronjob := testutil.NewCronJobBuilder().
		WithName(testConnectCronjobName).
		WithNamespace(testConnectNamespace).
		Build()
	standaloneJob := testutil.NewJobBuilder().
		WithName(testConnectJobName).
		WithNamespace(testConnectNamespace).
		Build()
	standaloneJob.OwnerReferences = nil

	_, httpClient := newConnectTestEnv(t, cronjob, standaloneJob)

	protocols := []struct {
		name string
		opts []connect.ClientOption
	}{
		{name: "Connect", opts: nil},
		{name: "gRPC-Web", opts: []connect.ClientOption{connect.WithGRPCWeb()}},
	}

	for _, protoTC := range protocols {
		t.Run(protoTC.name, func(t *testing.T) {
			client := protosconnect.NewCronjobClient(httpClient, testConnectBaseURL, protoTC.opts...)

			t.Run("GetCronjobs", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), testConnectTimeout)
				defer cancel()

				stream, err := client.GetCronjobs(ctx, connect.NewRequest(&protos.CronjobsRequest{}))
				if err != nil {
					t.Fatalf("GetCronjobs failed: %v", err)
				}
				defer stream.Close()

				if !stream.Receive() {
					t.Fatalf("expected stream message, err: %v", stream.Err())
				}
				if len(stream.Msg().GetCronjobs()) != 1 || stream.Msg().GetCronjobs()[0].GetName() != testConnectCronjobName {
					t.Errorf("unexpected cronjobs response: %+v", stream.Msg().GetCronjobs())
				}
				cancel()
			})

			t.Run("GetCronjob", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), testConnectTimeout)
				defer cancel()

				stream, err := client.GetCronjob(ctx, connect.NewRequest(&protos.CronjobRequest{
					CronjobName:      testConnectCronjobName,
					CronjobNamespace: testConnectNamespace,
				}))
				if err != nil {
					t.Fatalf("GetCronjob failed: %v", err)
				}
				defer stream.Close()

				if !stream.Receive() {
					t.Fatalf("expected stream message, err: %v", stream.Err())
				}
				if stream.Msg().GetName() != testConnectCronjobName {
					t.Errorf("expected %s, got %q", testConnectCronjobName, stream.Msg().GetName())
				}
				cancel()
			})

			t.Run("GetCronjobPods", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), testConnectTimeout)
				defer cancel()

				stream, err := client.GetCronjobPods(ctx, connect.NewRequest(&protos.CronjobPodsRequest{
					CronjobName:      testConnectCronjobName,
					CronjobNamespace: testConnectNamespace,
				}))
				if err != nil {
					t.Fatalf("GetCronjobPods failed: %v", err)
				}
				defer stream.Close()

				if !stream.Receive() {
					t.Fatalf("expected stream message, err: %v", stream.Err())
				}
				if stream.Msg().GetCronjob().GetName() != testConnectCronjobName {
					t.Errorf("expected %s, got %q", testConnectCronjobName, stream.Msg().GetCronjob().GetName())
				}
				cancel()
			})

			t.Run("GetJobs", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), testConnectTimeout)
				defer cancel()

				stream, err := client.GetJobs(ctx, connect.NewRequest(&protos.JobsRequest{}))
				if err != nil {
					t.Fatalf("GetJobs failed: %v", err)
				}
				defer stream.Close()

				if !stream.Receive() {
					t.Fatalf("expected stream message, err: %v", stream.Err())
				}
				if len(stream.Msg().GetJobs()) != 1 || stream.Msg().GetJobs()[0].GetName() != testConnectJobName {
					t.Errorf("unexpected jobs response: %+v", stream.Msg().GetJobs())
				}
				cancel()
			})
		})
	}
}

func TestConnectErrorPropagation(t *testing.T) {
	_, httpClient := newConnectTestEnvWithServer(t, NewSk8lServer(nil, nil, nil))

	client := protosconnect.NewCronjobClient(httpClient, testConnectBaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), testConnectTimeout)
	defer cancel()

	if _, err := client.GetCronjobYAML(ctx, connect.NewRequest(&protos.CronjobRequest{})); err == nil {
		t.Error("expected error from GetCronjobYAML when k8sClient is nil")
	} else if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("expected CodeUnavailable from GetCronjobYAML, got %v", connect.CodeOf(err))
	}

	if _, err := client.GetJobYAML(ctx, connect.NewRequest(&protos.JobRequest{})); err == nil {
		t.Error("expected error from GetJobYAML when k8sClient is nil")
	} else if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("expected CodeUnavailable from GetJobYAML, got %v", connect.CodeOf(err))
	}

	if _, err := client.GetPodYAML(ctx, connect.NewRequest(&protos.PodRequest{})); err == nil {
		t.Error("expected error from GetPodYAML when k8sClient is nil")
	} else if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("expected CodeUnavailable from GetPodYAML, got %v", connect.CodeOf(err))
	}

	stream, err := client.GetCronjobs(ctx, connect.NewRequest(&protos.CronjobsRequest{}))
	if err == nil {
		defer stream.Close()
		if stream.Receive() || stream.Err() == nil {
			t.Error("expected error from GetCronjobs stream when store is nil")
		} else if connect.CodeOf(stream.Err()) != connect.CodeUnavailable {
			t.Errorf("expected CodeUnavailable from GetCronjobs stream, got %v", connect.CodeOf(stream.Err()))
		}
	}
}

func TestConnectNotFoundErrorPropagation(t *testing.T) {
	fakeClientSet := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(fakeClientSet)
	srv := NewSk8lServer(nil, k8sClient, nil)
	_, httpClient := newConnectTestEnvWithServer(t, srv)

	client := protosconnect.NewCronjobClient(httpClient, testConnectBaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), testConnectTimeout)
	defer cancel()

	t.Run("GetCronjobYAML_NotFound", func(t *testing.T) {
		_, err := client.GetCronjobYAML(ctx, connect.NewRequest(&protos.CronjobRequest{
			CronjobName:      "nonexistent-cj",
			CronjobNamespace: testConnectNamespace,
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
		}
	})

	t.Run("GetJobYAML_NotFound", func(t *testing.T) {
		_, err := client.GetJobYAML(ctx, connect.NewRequest(&protos.JobRequest{
			JobName:      "nonexistent-job",
			JobNamespace: testConnectNamespace,
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
		}
	})

	t.Run("GetPodYAML_NotFound", func(t *testing.T) {
		_, err := client.GetPodYAML(ctx, connect.NewRequest(&protos.PodRequest{
			PodName:      "nonexistent-pod",
			PodNamespace: testConnectNamespace,
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
		}
	})
}

func TestToConnectError(t *testing.T) {
	if err := toConnectError("op", nil); err != nil {
		t.Errorf("expected nil for nil input, got %v", err)
	}

	alreadyConnect := connect.NewError(connect.CodeNotFound, errTestAlreadyConnect)
	if got := toConnectError("op", alreadyConnect); !errors.Is(got, alreadyConnect) {
		t.Errorf("expected already connect error returned as-is, got %v", got)
	}

	cases := []struct {
		err      error
		name     string
		wantCode connect.Code
	}{
		{name: "Canceled", err: context.Canceled, wantCode: connect.CodeCanceled},
		{name: "DeadlineExceeded", err: context.DeadlineExceeded, wantCode: connect.CodeDeadlineExceeded},
		{name: "NotFound", err: apierrors.NewNotFound(batchv1.Resource("cronjobs"), "foo"), wantCode: connect.CodeNotFound},
		{name: "Forbidden", err: apierrors.NewForbidden(batchv1.Resource("cronjobs"), "foo", nil), wantCode: connect.CodePermissionDenied},
		{name: "Unauthorized", err: apierrors.NewUnauthorized("reason"), wantCode: connect.CodeUnauthenticated},
		{name: "AlreadyExists", err: apierrors.NewAlreadyExists(batchv1.Resource("cronjobs"), "foo"), wantCode: connect.CodeAlreadyExists},
		{name: "Conflict", err: apierrors.NewConflict(batchv1.Resource("cronjobs"), "foo", nil), wantCode: connect.CodeAborted},
		{name: "BadRequest", err: apierrors.NewBadRequest("bad request"), wantCode: connect.CodeInvalidArgument},
		{name: "ServerTimeout", err: apierrors.NewServerTimeout(batchv1.Resource("cronjobs"), "op", 1), wantCode: connect.CodeDeadlineExceeded},
		{name: "StoreNil", err: ErrStoreNil, wantCode: connect.CodeUnavailable},
		{name: "K8sClientNil", err: ErrK8sClientNil, wantCode: connect.CodeUnavailable},
		{name: "StoreNotReady", err: ErrStoreNotReady, wantCode: connect.CodeUnavailable},
		{name: "StoreDegraded", err: ErrStoreDegraded, wantCode: connect.CodeUnavailable},
		{name: "Unclassified", err: errTestUnclassified, wantCode: connect.CodeInternal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toConnectError(tc.name, tc.err)
			if connect.CodeOf(got) != tc.wantCode {
				t.Errorf("toConnectError() code = %v, want %v", connect.CodeOf(got), tc.wantCode)
			}
		})
	}
}

func newCORSTestEnv(t *testing.T, allowed []string) *http.ServeMux {
	t.Helper()
	clientSet := fake.NewClientset()
	st := setupTestInformerStore(t, clientSet, testConnectNamespace)
	srv := NewSk8lServer(st, k8s.NewClientWithInterface(clientSet), nil)
	mux := &http.ServeMux{}
	setupHTTPRoutes(mux, srv, allowed)
	return mux
}

func corsRequest(method, origin string, preflight bool, body string) *http.Request {
	req := httptest.NewRequestWithContext(
		context.Background(),
		method,
		protosconnect.CronjobGetDashboardAnnotationsProcedure,
		strings.NewReader(body),
	)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if preflight {
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestWithCORS_DisabledByDefault(t *testing.T) {
	mux := newCORSTestEnv(t, nil)

	for _, preflight := range []bool{true, false} {
		method, body := http.MethodPost, "{}"
		if preflight {
			method, body = http.MethodOptions, ""
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, corsRequest(method, testConnectOrigin, preflight, body))

		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%s: expected no Access-Control-Allow-Origin, got %q", method, got)
		}
		if preflight && rr.Code == http.StatusNoContent {
			t.Errorf("preflight must not be answered with 204 when CORS is disabled")
		}
	}
}

func TestWithCORS_AllowList(t *testing.T) {
	mux := newCORSTestEnv(t, []string{testConnectOrigin})

	t.Run("preflight from allowed origin", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, corsRequest(http.MethodOptions, testConnectOrigin, true, ""))

		if rr.Code != http.StatusNoContent {
			t.Errorf("expected 204 No Content, got %d", rr.Code)
		}
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != testConnectOrigin {
			t.Errorf("expected Access-Control-Allow-Origin %s, got %q", testConnectOrigin, got)
		}
		if got := rr.Header().Get("Access-Control-Allow-Methods"); got != corsAllowMethods {
			t.Errorf("expected Access-Control-Allow-Methods %q, got %q", corsAllowMethods, got)
		}
		if got := rr.Header().Get("Vary"); got != "Origin" {
			t.Errorf("expected Vary: Origin, got %q", got)
		}
	})

	t.Run("POST from allowed origin", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, corsRequest(http.MethodPost, testConnectOrigin, false, "{}"))

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rr.Code)
		}
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != testConnectOrigin {
			t.Errorf("expected Access-Control-Allow-Origin %s, got %q", testConnectOrigin, got)
		}
	})

	t.Run("origin not in allow-list gets no CORS headers", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, corsRequest(http.MethodOptions, "https://evil.example", true, ""))

		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("expected no Access-Control-Allow-Origin, got %q", got)
		}
		if rr.Code == http.StatusNoContent {
			t.Error("preflight from a disallowed origin must not be answered with 204")
		}
		if got := rr.Header().Get("Vary"); got != "Origin" {
			t.Errorf("expected Vary: Origin even for rejected origins, got %q", got)
		}
	})

	t.Run("request without Origin passes through", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, corsRequest(http.MethodPost, "", false, "{}"))

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rr.Code)
		}
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("expected no Access-Control-Allow-Origin, got %q", got)
		}
	})
}

func TestParseCORSAllowedOrigins(t *testing.T) {
	testCases := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{name: "empty", raw: "", want: nil},
		{name: "blank entries", raw: " , ,", want: nil},
		{name: "single", raw: "https://localhost:5173", want: []string{"https://localhost:5173"}},
		{
			name: "multiple trimmed lowercased deduped",
			raw:  " HTTP://Localhost:5173 ,https://sk8l.example.com,http://localhost:5173",
			want: []string{"http://localhost:5173", "https://sk8l.example.com"},
		},
		{name: "wildcard rejected", raw: "*", wantErr: true},
		{name: "missing scheme", raw: "localhost:5173", wantErr: true},
		{name: "unsupported scheme", raw: "ftp://localhost", wantErr: true},
		{name: "path rejected", raw: "https://localhost:5173/app", wantErr: true},
		{name: "trailing slash rejected", raw: "https://localhost:5173/", wantErr: true},
		{name: "userinfo rejected", raw: "https://user@localhost", wantErr: true},
		{name: "one bad entry fails all", raw: "https://ok.example,*", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCORSAllowedOrigins(tc.raw)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidCORSOrigin) {
					t.Fatalf("expected ErrInvalidCORSOrigin, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestConnectRejectsOversizedRequest(t *testing.T) {
	_, httpClient := newConnectTestEnv(t)
	client := protosconnect.NewCronjobClient(httpClient, testConnectBaseURL)

	ctx, cancel := context.WithTimeout(context.Background(), testConnectTimeout)
	defer cancel()

	_, err := client.GetCronjobYAML(ctx, connect.NewRequest(&protos.CronjobRequest{
		CronjobName: strings.Repeat("a", maxRequestBytes+1),
	}))
	if got := connect.CodeOf(err); got != connect.CodeResourceExhausted {
		t.Fatalf("expected CodeResourceExhausted, got %v (err=%v)", got, err)
	}
}

func TestRecoverHandlerPanic(t *testing.T) {
	err := recoverHandlerPanic(
		context.Background(),
		connect.Spec{Procedure: protosconnect.CronjobGetCronjobsProcedure},
		http.Header{},
		"boom",
	)
	if got := connect.CodeOf(err); got != connect.CodeInternal {
		t.Fatalf("expected CodeInternal, got %v", got)
	}
	if strings.Contains(err.Error(), "boom") {
		t.Errorf("panic value must not leak to clients: %v", err)
	}
}

func TestConnectHandlerRecoversFromPanic(t *testing.T) {
	// Serve a handler that always panics through the same options used in production.
	path, handler := protosconnect.NewCronjobHandler(panicCronjobHandler{}, connectHandlerOptions()...)
	mux := &http.ServeMux{}
	mux.Handle(path, handler)

	client := protosconnect.NewCronjobClient(newBufconnHTTPClient(t, mux), testConnectBaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), testConnectTimeout)
	defer cancel()

	_, err := client.GetDashboardAnnotations(ctx, connect.NewRequest(&protos.DashboardAnnotationsRequest{}))
	if got := connect.CodeOf(err); got != connect.CodeInternal {
		t.Fatalf("expected CodeInternal after panic, got %v (err=%v)", got, err)
	}
}

type panicCronjobHandler struct {
	protosconnect.UnimplementedCronjobHandler
}

func (panicCronjobHandler) GetDashboardAnnotations(
	context.Context,
	*connect.Request[protos.DashboardAnnotationsRequest],
) (*connect.Response[protos.DashboardAnnotationsResponse], error) {
	panic("handler bug")
}

// TestNativeGRPCOverTLSHTTPServer verifies that native gRPC clients (such as grpcurl)
// negotiate HTTP/2 ("h2" ALPN) and communicate directly with the unified Go http.Server.
func TestNativeGRPCOverTLSHTTPServer(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: testTLSServerName},
		DNSNames:     []string{testTLSServerName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create cert: %v", err)
	}
	parsedCert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("failed to parse cert: %v", err)
	}
	certPool := x509.NewCertPool()
	certPool.AddCert(parsedCert)

	serverTLS := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS13,
		NextProtos:   []string{"h2", "http/1.1"},
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: priv}},
	}

	cronjob := testutil.NewCronJobBuilder().
		WithName(testConnectCronjobName).
		WithNamespace(testConnectNamespace).
		Build()
	clientSet := fake.NewClientset(cronjob)
	k8sClient := k8s.NewClientWithInterface(clientSet)
	st := setupTestInformerStore(t, clientSet, testConnectNamespace)
	srv := NewSk8lServer(st, k8sClient, nil)

	mux := &http.ServeMux{}
	setupHTTPRoutes(mux, srv, nil)

	bl := bufconn.Listen(bufSize)
	httpSrv := newHTTPServer("", mux, serverTLS)
	go func() {
		_ = httpSrv.Serve(tls.NewListener(bl, serverTLS))
	}()
	t.Cleanup(func() {
		_ = httpSrv.Close()
		_ = bl.Close()
	})

	conn, err := grpc.NewClient(
		"passthrough:///"+testTLSServerName,
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			c, dialErr := bl.DialContext(ctx)
			if dialErr != nil {
				return nil, fmt.Errorf("connect_test: failed to dial tls bufconn: %w", dialErr)
			}
			return c, nil
		}),
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(certPool, testTLSServerName)),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient failed: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testConnectTimeout)
	defer cancel()

	grpcClient := protos.NewCronjobClient(conn)
	annResp, err := grpcClient.GetDashboardAnnotations(ctx, &protos.DashboardAnnotationsRequest{})
	if err != nil {
		t.Fatalf("native gRPC GetDashboardAnnotations over http.Server failed: %v", err)
	}
	if annResp.GetAnnotations() == "" {
		t.Error("expected non-empty annotations over native gRPC")
	}

	stream, err := grpcClient.GetCronjobs(ctx, &protos.CronjobsRequest{})
	if err != nil {
		t.Fatalf("native gRPC GetCronjobs over http.Server failed: %v", err)
	}
	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("native gRPC stream.Recv failed: %v", err)
	}
	if len(msg.GetCronjobs()) != 1 || msg.GetCronjobs()[0].GetName() != testConnectCronjobName {
		t.Errorf("unexpected cronjobs over native gRPC: %+v", msg.GetCronjobs())
	}
}

func TestParseTLSEnabled(t *testing.T) {
	cases := []struct {
		raw     string
		want    bool
		wantErr bool
	}{
		{raw: "", want: false, wantErr: false},
		{raw: "   ", want: false, wantErr: false},
		{raw: "false", want: false, wantErr: false},
		{raw: "0", want: false, wantErr: false},
		{raw: "true", want: true, wantErr: false},
		{raw: "TRUE", want: true, wantErr: false},
		{raw: "1", want: true, wantErr: false},
		{raw: "not-a-bool", want: false, wantErr: true},
	}

	for _, tc := range cases {
		got, err := parseTLSEnabled(tc.raw)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseTLSEnabled(%q) error = %v, wantErr %v", tc.raw, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("parseTLSEnabled(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// TestPlaintextH2CHTTPServer verifies that when TLS is disabled (tlsConfig == nil),
// newHTTPServer serves both HTTP/1.1 requests and unencrypted HTTP/2 (h2c) native gRPC
// on the same listener.
func TestPlaintextH2CHTTPServer(t *testing.T) {
	cronjob := testutil.NewCronJobBuilder().
		WithName(testConnectCronjobName).
		WithNamespace(testConnectNamespace).
		Build()
	clientSet := fake.NewClientset(cronjob)
	k8sClient := k8s.NewClientWithInterface(clientSet)
	st := setupTestInformerStore(t, clientSet, testConnectNamespace)
	srv := NewSk8lServer(st, k8sClient, nil)

	mux := &http.ServeMux{}
	setupHTTPRoutes(mux, srv, nil)

	bl := bufconn.Listen(bufSize)
	httpSrv := newHTTPServer("", mux, nil)
	go func() {
		_ = httpSrv.Serve(bl)
	}()
	t.Cleanup(func() {
		_ = httpSrv.Close()
		_ = bl.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), testConnectTimeout)
	defer cancel()

	// 1. Verify native gRPC over plaintext h2c (prior-knowledge HTTP/2).
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(dialCtx context.Context, _ string) (net.Conn, error) {
			c, dialErr := bl.DialContext(dialCtx)
			if dialErr != nil {
				return nil, fmt.Errorf("connect_test: failed to dial h2c bufconn: %w", dialErr)
			}
			return c, nil
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient failed: %v", err)
	}
	defer conn.Close()

	grpcClient := protos.NewCronjobClient(conn)
	annResp, err := grpcClient.GetDashboardAnnotations(ctx, &protos.DashboardAnnotationsRequest{})
	if err != nil {
		t.Fatalf("plaintext h2c gRPC GetDashboardAnnotations failed: %v", err)
	}
	if annResp.GetAnnotations() == "" {
		t.Error("expected non-empty annotations over plaintext h2c gRPC")
	}

	stream, err := grpcClient.GetCronjobs(ctx, &protos.CronjobsRequest{})
	if err != nil {
		t.Fatalf("plaintext h2c gRPC GetCronjobs failed: %v", err)
	}
	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("plaintext h2c gRPC stream.Recv failed: %v", err)
	}
	if len(msg.GetCronjobs()) != 1 || msg.GetCronjobs()[0].GetName() != testConnectCronjobName {
		t.Errorf("unexpected cronjobs over plaintext h2c gRPC: %+v", msg.GetCronjobs())
	}

	// 2. Verify standard HTTP/1.1 requests (kubelet probes + Connect JSON) on the same listener.
	http1Client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
				c, dialErr := bl.DialContext(dialCtx)
				if dialErr != nil {
					return nil, fmt.Errorf("connect_test: failed to dial http1 bufconn: %w", dialErr)
				}
				return c, nil
			},
		},
	}

	for _, probePath := range []string{"/healthz", "/readyz"} {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, testConnectBaseURL+probePath, nil)
		if reqErr != nil {
			t.Fatalf("NewRequestWithContext(%s) failed: %v", probePath, reqErr)
		}
		resp, doErr := http1Client.Do(req)
		if doErr != nil {
			t.Fatalf("GET %s over plaintext HTTP/1.1 failed: %v", probePath, doErr)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: expected 200 OK, got %d", probePath, resp.StatusCode)
		}
	}

	connectReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		testConnectBaseURL+protosconnect.CronjobGetDashboardAnnotationsProcedure,
		strings.NewReader("{}"),
	)
	if err != nil {
		t.Fatalf("NewRequestWithContext Connect JSON failed: %v", err)
	}
	connectReq.Header.Set("Content-Type", "application/json")
	connectResp, err := http1Client.Do(connectReq)
	if err != nil {
		t.Fatalf("POST Connect JSON over plaintext HTTP/1.1 failed: %v", err)
	}
	_ = connectResp.Body.Close()
	if connectResp.StatusCode != http.StatusOK {
		t.Errorf("POST Connect JSON: expected 200 OK, got %d", connectResp.StatusCode)
	}
}
