// Package main is the entry point for the sk8l gRPC server.
package main

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/danroux/sk8l/internal/dashboard"
	"github.com/danroux/sk8l/internal/logger"
	"github.com/danroux/sk8l/internal/store"
	"github.com/danroux/sk8l/protos"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	ReadTimeoutSeconds  = 10
	WriteTimeoutSeconds = 30
	ReadTimeout         = time.Duration(ReadTimeoutSeconds)
	WriteTimeout        = time.Duration(WriteTimeoutSeconds)
)

var (
	K8Namespace   = os.Getenv("K8_NAMESPACE")
	APIPort       = os.Getenv("SK8L_SERVICE_PORT_SK8L_API")
	APIHealthPort = os.Getenv("SK8L_SERVICE_PORT_SK8L_API_HEALTH")
	MetricsPort   = os.Getenv("SK8L_SERVICE_PORT_SK8L_API_METRICS")
	certFile      = filepath.Join("/etc", "sk8l-certs", "server-cert.pem")
	certKeyFile   = filepath.Join("/etc", "sk8l-certs", "server-key.pem")
	caFile        = filepath.Join("/etc", "sk8l-certs", "ca-cert.pem")
	MetricPrefix  = fmt.Sprintf("sk8l_%s", K8Namespace)
)

func main() {
	logger.SetupZeroLog()
	certPool := x509.NewCertPool()
	serverTLSConfig, err := setupTLS(certFile, certKeyFile, caFile, certPool)
	if err != nil {
		log.Fatal().Err(err).Msg("setupTLS")
	}
	target := fmt.Sprintf("0.0.0.0:%s", APIPort)
	lc := net.ListenConfig{}
	rootCtx := context.Background()
	ln, err := lc.Listen(rootCtx, "tcp", target)
	if err != nil {
		log.Fatal().Err(err).Msg("tlsListen")
	}
	healthTarget := fmt.Sprintf("0.0.0.0:%s", APIHealthPort)
	healthLn, err := lc.Listen(rootCtx, "tcp", healthTarget)
	if err != nil {
		log.Fatal().Err(err).Msg("Health Probe Listen")
	}
	serverCreds := credentials.NewTLS(serverTLSConfig)
	creds := grpc.Creds(serverCreds)
	grpcS := grpc.NewServer(creds)
	probeS := grpc.NewServer()

	metricsNamesMap := &sync.Map{}
	dashboardGen := dashboard.NewGenerator(
		MetricPrefix,
		K8Namespace,
		TotalMetricNames,
	)

	cronjobDBStore := store.NewCronJobDBStore(store.WithDefaultK8sClient(K8Namespace))
	sk8lServer := NewSk8lServer(
		target,
		cronjobDBStore,
		dashboardGen,
		metricsNamesMap,
		grpc.WithTransportCredentials(serverCreds),
	)

	healthgrpc.RegisterHealthServer(probeS, sk8lServer)
	protos.RegisterCronjobServer(grpcS, sk8lServer)
	mux := &http.ServeMux{}
	mux.Handle("/metrics", promhttp.Handler())
	httpS := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%s", MetricsPort),
		IdleTimeout:  time.Minute,
		ReadTimeout:  ReadTimeout * time.Second,
		WriteTimeout: WriteTimeout * time.Second,
		TLSConfig:    serverTLSConfig,
		Handler:      mux,
	}
	log.Info().
		Msg(fmt.Sprintf("Starting %s server %s on %s", "sk8l", Version(), ln.Addr().String()))
	errCh := make(chan error, 3)
	go func() {
		if err := httpS.ListenAndServeTLS(certFile, certKeyFile); err != nil {
			errCh <- fmt.Errorf("httpS error: %w", err)
		}
	}()
	go func() {
		if err := probeS.Serve(healthLn); err != nil {
			errCh <- fmt.Errorf("probeS error: %w", err)
		}
	}()
	go func() {
		if err := grpcS.Serve(ln); err != nil {
			errCh <- fmt.Errorf("grpcS error: %w", err)
		}
	}()
	metricsCxt, metricsCancel := context.WithCancel(rootCtx)
	sk8lServer.Run(metricsCxt)
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
	shutdownCtx, shutdownCancel := context.WithTimeout(rootCtx, 5*time.Second)
	defer shutdownCancel()
	if err := httpS.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Shutdown: error during httpS shutdown")
	}
	metricsCancel()
	grpcS.GracefulStop()
	probeS.GracefulStop()
	log.Info().Msg("Shutdown: sk8l has stopped")
}
