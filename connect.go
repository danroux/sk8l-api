package main

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"github.com/danroux/sk8l/protos"
	"github.com/danroux/sk8l/protos/protosconnect"
	"github.com/rs/zerolog/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrInternal is the generic message returned to clients for unclassified server errors.
// The underlying cause is logged server-side and never sent over the wire.
var ErrInternal = errors.New("internal error")

const (
	corsAllowMethods = "GET, POST, OPTIONS"
	corsAllowHeaders = "Accept-Encoding, Content-Encoding, Content-Type, " +
		"Connect-Accept-Encoding, Connect-Content-Encoding, Connect-Protocol-Version, " +
		"Connect-Timeout-Ms, Grpc-Timeout, X-Grpc-Web, X-User-Agent"
	corsExposeHeaders = "Content-Encoding, Connect-Accept-Encoding, " +
		"Connect-Content-Encoding, Grpc-Message, Grpc-Status, Grpc-Status-Details-Bin"
	corsMaxAge = "1728000"
)

// toConnectError converts an error returned by Sk8lServer into a *connect.Error carrying the
// proper status code, so Connect, gRPC-Web, and gRPC clients can distinguish NotFound,
// PermissionDenied, Unavailable, etc. instead of receiving CodeUnknown for everything.
// Errors that already are *connect.Error are returned unchanged. Unclassified errors become
// CodeInternal with a generic message; the detail is logged to avoid leaking internals.
func toConnectError(operation string, err error) error {
	if err == nil {
		return nil
	}

	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return err
	}

	switch {
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	case apierrors.IsNotFound(err):
		return connect.NewError(connect.CodeNotFound, err)
	case apierrors.IsForbidden(err):
		return connect.NewError(connect.CodePermissionDenied, err)
	case apierrors.IsUnauthorized(err):
		return connect.NewError(connect.CodeUnauthenticated, err)
	case apierrors.IsAlreadyExists(err):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case apierrors.IsConflict(err):
		return connect.NewError(connect.CodeAborted, err)
	case apierrors.IsBadRequest(err), apierrors.IsInvalid(err):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case apierrors.IsServerTimeout(err), apierrors.IsTimeout(err):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	case apierrors.IsServiceUnavailable(err),
		errors.Is(err, ErrStoreNil),
		errors.Is(err, ErrK8sClientNil),
		errors.Is(err, ErrStoreNotReady),
		errors.Is(err, ErrStoreDegraded):
		return connect.NewError(connect.CodeUnavailable, err)
	}

	log.Error().Err(err).Str("operation", operation).Msg("unclassified error; returning internal")
	return connect.NewError(connect.CodeInternal, ErrInternal)
}

// ConnectHandler adapts Sk8lServer to the protosconnect.CronjobHandler interface,
// allowing the same service implementation to serve Connect, gRPC-Web, and gRPC
// over standard net/http.
type ConnectHandler struct {
	protosconnect.UnimplementedCronjobHandler
	server *Sk8lServer
}

// NewConnectHandler creates a ConnectHandler backed by the given Sk8lServer.
func NewConnectHandler(s *Sk8lServer) *ConnectHandler {
	return &ConnectHandler{server: s}
}

func (h *ConnectHandler) GetCronjobs(
	ctx context.Context,
	req *connect.Request[protos.CronjobsRequest],
	stream *connect.ServerStream[protos.CronjobsResponse],
) error {
	return toConnectError("GetCronjobs", h.server.streamCronjobs(ctx, req.Msg, stream))
}

func (h *ConnectHandler) GetCronjob(
	ctx context.Context,
	req *connect.Request[protos.CronjobRequest],
	stream *connect.ServerStream[protos.CronjobResponse],
) error {
	return toConnectError("GetCronjob", h.server.streamCronjob(ctx, req.Msg, stream))
}

func (h *ConnectHandler) GetCronjobPods(
	ctx context.Context,
	req *connect.Request[protos.CronjobPodsRequest],
	stream *connect.ServerStream[protos.CronjobPodsResponse],
) error {
	return toConnectError("GetCronjobPods", h.server.streamCronjobPods(ctx, req.Msg, stream))
}

func (h *ConnectHandler) GetJobs(
	ctx context.Context,
	req *connect.Request[protos.JobsRequest],
	stream *connect.ServerStream[protos.JobsResponse],
) error {
	return toConnectError("GetJobs", h.server.streamJobs(ctx, req.Msg, stream))
}

func (h *ConnectHandler) GetCronjobYAML(
	ctx context.Context,
	req *connect.Request[protos.CronjobRequest],
) (*connect.Response[protos.CronjobYAMLResponse], error) {
	resp, err := h.server.GetCronjobYAML(ctx, req.Msg)
	if err != nil {
		return nil, toConnectError("GetCronjobYAML", err)
	}
	return connect.NewResponse(resp), nil
}

func (h *ConnectHandler) GetJobYAML(
	ctx context.Context,
	req *connect.Request[protos.JobRequest],
) (*connect.Response[protos.JobYAMLResponse], error) {
	resp, err := h.server.GetJobYAML(ctx, req.Msg)
	if err != nil {
		return nil, toConnectError("GetJobYAML", err)
	}
	return connect.NewResponse(resp), nil
}

func (h *ConnectHandler) GetPodYAML(
	ctx context.Context,
	req *connect.Request[protos.PodRequest],
) (*connect.Response[protos.PodYAMLResponse], error) {
	resp, err := h.server.GetPodYAML(ctx, req.Msg)
	if err != nil {
		return nil, toConnectError("GetPodYAML", err)
	}
	return connect.NewResponse(resp), nil
}

func (h *ConnectHandler) GetDashboardAnnotations(
	ctx context.Context,
	req *connect.Request[protos.DashboardAnnotationsRequest],
) (*connect.Response[protos.DashboardAnnotationsResponse], error) {
	resp, err := h.server.GetDashboardAnnotations(ctx, req.Msg)
	if err != nil {
		return nil, toConnectError("GetDashboardAnnotations", err)
	}
	return connect.NewResponse(resp), nil
}

// withCORS wraps an http.Handler with CORS headers required by browser-based
// Connect and gRPC-Web clients when running across origins.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
			w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
			w.Header().Set("Access-Control-Expose-Headers", corsExposeHeaders)
			w.Header().Set("Access-Control-Max-Age", corsMaxAge)
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
