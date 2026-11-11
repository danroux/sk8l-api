package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/danroux/sk8l/protos"
	"github.com/danroux/sk8l/protos/protosconnect"
	"github.com/rs/zerolog/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrInternal is the generic message returned to clients for unclassified server errors.
// The underlying cause is logged server-side and never sent over the wire.
var ErrInternal = errors.New("internal error")

// ErrInvalidCORSOrigin is returned when SK8L_CORS_ALLOWED_ORIGINS contains an unusable entry.
var ErrInvalidCORSOrigin = errors.New("invalid CORS origin")

const (
	corsAllowMethods = "GET, POST, OPTIONS"
	corsAllowHeaders = "Accept-Encoding, Content-Encoding, Content-Type, " +
		"Connect-Accept-Encoding, Connect-Content-Encoding, Connect-Protocol-Version, " +
		"Connect-Timeout-Ms, Grpc-Timeout, X-Grpc-Web, X-User-Agent"
	corsExposeHeaders = "Content-Encoding, Connect-Accept-Encoding, " +
		"Connect-Content-Encoding, Grpc-Message, Grpc-Status, Grpc-Status-Details-Bin"
	corsMaxAge = "1728000"

	// maxRequestBytes bounds the size of a single request message. Every sk8l RPC takes a
	// handful of short string fields, so 64 KiB is orders of magnitude above legitimate use.
	maxRequestBytes = 64 << 10
)

// connectHandlerOptions returns the hardening options applied to the Cronjob service handler:
// a per-message request-size limit (connect-go allows any size by default) and RPC-aware panic
// recovery. net/http already recovers a panicking handler per request; WithRecover additionally
// logs the procedure and stack and answers with a proper Internal status instead of a dropped stream.
func connectHandlerOptions() []connect.HandlerOption {
	return []connect.HandlerOption{
		connect.WithReadMaxBytes(maxRequestBytes),
		connect.WithRecover(recoverHandlerPanic),
	}
}

// recoverHandlerPanic converts a panic inside an RPC handler into CodeInternal. The panic value
// and stack are logged server-side only.
func recoverHandlerPanic(_ context.Context, spec connect.Spec, _ http.Header, p any) error {
	log.Error().
		Str("procedure", spec.Procedure).
		Interface("panic", p).
		Bytes("stack", debug.Stack()).
		Msg("panic recovered in RPC handler")
	return connect.NewError(connect.CodeInternal, ErrInternal)
}

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

// parseCORSAllowedOrigins parses a comma-separated list of exact origins (scheme://host[:port]).
// An empty value yields no origins, which keeps CORS disabled. Wildcards are rejected on purpose:
// sk8l exposes cluster workload data without authentication, so a blanket allow-list would let any
// web page the user visits read it.
func parseCORSAllowedOrigins(raw string) ([]string, error) {
	var origins []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		u, err := url.Parse(entry)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return nil, fmt.Errorf("%w: %q must look like scheme://host[:port]", ErrInvalidCORSOrigin, entry)
		}
		origin := strings.ToLower(u.Scheme + "://" + u.Host)
		if !slices.Contains(origins, origin) {
			origins = append(origins, origin)
		}
	}
	return origins, nil
}

// withCORS wraps an http.Handler with the CORS headers browser-based Connect and gRPC-Web clients
// need when they run on a different origin than sk8l-api (for example a local Vite dev server).
// It is opt-in: with no allowed origins the handler is returned untouched, because the embedded
// UI is served same-origin and needs no CORS. Only origins in allowedOrigins receive CORS headers;
// requests from any other origin are passed through without them, so browsers block the response.
func withCORS(allowedOrigins []string, next http.Handler) http.Handler {
	if len(allowedOrigins) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin == "" || !slices.Contains(allowedOrigins, strings.ToLower(origin)) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
		w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
		w.Header().Set("Access-Control-Expose-Headers", corsExposeHeaders)
		w.Header().Set("Access-Control-Max-Age", corsMaxAge)
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
