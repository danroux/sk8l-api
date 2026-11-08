// Package main is the entry point for the sk8l server.
package main

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/danroux/sk8l/internal/k8s"
	"github.com/danroux/sk8l/internal/logger"
	"github.com/danroux/sk8l/internal/store/informer"
	"github.com/rs/zerolog/log"
)

const (
	ReadTimeoutSeconds = 10
	ReadTimeout        = ReadTimeoutSeconds * time.Second

	shutdownTimeoutSeconds = 5
	shutdownTimeout        = shutdownTimeoutSeconds * time.Second

	defaultHTTPPort      = "8590"
	envSK8LHTTPPort      = "SK8L_HTTP_PORT"
	envServiceHTTPPort   = "SK8L_SERVICE_PORT_HTTP"
	envLegacyMetricsPort = "SK8L_SERVICE_PORT_SK8L_API_METRICS"
)

var (
	K8Namespace = os.Getenv("K8_NAMESPACE")
	UIAPIURL    = os.Getenv("VITE_SK8L_API_URL")
	HTTPPort    = resolveHTTPPort()
	certFile    = filepath.Join("/etc", "sk8l-certs", "server-cert.pem")
	certKeyFile = filepath.Join("/etc", "sk8l-certs", "server-key.pem")
	caFile      = filepath.Join("/etc", "sk8l-certs", "ca-cert.pem")
)

func resolveHTTPPort() string {
	for _, envKey := range []string{
		envSK8LHTTPPort,
		envServiceHTTPPort,
		envLegacyMetricsPort,
	} {
		if port := os.Getenv(envKey); port != "" {
			return port
		}
	}
	return defaultHTTPPort
}

func main() {
	logger.SetupZeroLog()
	certPool := x509.NewCertPool()
	serverTLSConfig, err := setupTLS(certFile, certKeyFile, caFile, certPool)
	if err != nil {
		log.Fatal().Err(err).Msg("setupTLS")
	}

	rootCtx := context.Background()
	metricsNamesMap := &sync.Map{}

	k8sClient, err := k8s.NewClient(
		k8s.WithNamespace(K8Namespace),
		k8s.WithLogger(log.With().Str("component", "k8s").Logger()),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create k8s client")
	}

	informerStore, err := informer.New(k8sClient, K8Namespace)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize informerStore")
	}

	sk8lServer := NewSk8lServer(
		informerStore,
		k8sClient,
		metricsNamesMap,
	)

	mux := &http.ServeMux{}
	setupHTTPRoutes(mux, sk8lServer)
	httpS := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%s", HTTPPort),
		IdleTimeout:       time.Minute,
		ReadHeaderTimeout: ReadTimeout,
		TLSConfig:         serverTLSConfig,
		Handler:           mux,
	}

	log.Info().Msg(fmt.Sprintf("Starting sk8l server %s on %s", Version(), httpS.Addr))
	errCh := startHTTPServer(httpS)
	metricsCtx, metricsCancel := context.WithCancel(rootCtx)
	sk8lServer.Run(metricsCtx)

	log.Info().Msg("Shutdown: setting up")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		log.Error().Err(err).
			Msg("Shutdown: sk8l shutting down... got error during server startup")
	case sig := <-quit:
		log.Info().
			Msg(fmt.Sprintf("Shutdown: Got %v signal. sk8l will shut down shortly", sig))
	}
	shutdownHTTPServer(rootCtx, httpS, metricsCancel)
}

func startHTTPServer(httpS *http.Server) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		if err := httpS.ListenAndServeTLS(certFile, certKeyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("httpS error: %w", err)
		}
	}()
	return errCh
}

func shutdownHTTPServer(
	rootCtx context.Context,
	httpS *http.Server,
	metricsCancel context.CancelFunc,
) {
	shutdownCtx, shutdownCancel := context.WithTimeout(rootCtx, shutdownTimeout)
	defer shutdownCancel()
	if err := httpS.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Shutdown: error during httpS shutdown")
	}
	metricsCancel()
	log.Info().Msg("Shutdown: sk8l has stopped")
}
