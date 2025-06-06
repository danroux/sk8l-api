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
	"syscall"
	"time"

	"github.com/danroux/sk8l/protos"
	badger "github.com/dgraph-io/badger/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

type defaultLog struct {
	zerolog.Logger
}

func BadgerNewLog(level zerolog.Level) *defaultLog {
	x := log.With().Str("component", "badger").Logger().Level(level)

	return &defaultLog{
		Logger: x,
	}
}

func (l *defaultLog) Errorf(f string, v ...any) {
	if l.GetLevel() <= zerolog.ErrorLevel {
		l.Printf("ERROR: "+f, v...)
	}
}

func (l *defaultLog) Warningf(f string, v ...any) {
	if l.GetLevel() <= zerolog.WarnLevel {
		l.Printf("WARNING: "+f, v...)
	}
}

func (l *defaultLog) Infof(f string, v ...any) {
	if l.GetLevel() <= zerolog.InfoLevel {
		l.Printf("INFO: "+f, v...)
	}
}

func (l *defaultLog) Debugf(f string, v ...any) {
	if l.GetLevel() <= zerolog.DebugLevel {
		l.Printf("DEBUG: "+f, v...)
	}
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.TimestampFieldName = "t"
	zerolog.LevelFieldName = "l"
	zerolog.MessageFieldName = "m"

	log.Info().Msg(fmt.Sprintf("log_level %d", zerolog.GlobalLevel()))
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	certPool := x509.NewCertPool()
	serverTLSConfig, err := setupTLS(certFile, certKeyFile, caFile, certPool)
	if err != nil {
		log.Fatal().Err(err).Msg("setupTLS")
	}

	target := fmt.Sprintf("0.0.0.0:%s", APIPort)
	conn, err := net.Listen("tcp", target)

	if err != nil {
		log.Fatal().Err(err).Msg("tlsListen")
	}

	healthConn, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", APIHealthPort))

	if err != nil {
		log.Fatal().Err(err).Msg("Health Probe Listen")
	}

	serverCreds := credentials.NewTLS(serverTLSConfig)
	creds := grpc.Creds(serverCreds)

	grpcS := grpc.NewServer(creds)
	probeS := grpc.NewServer()

	log.Info().Msg(fmt.Sprintf("grpcS creds %v", creds))
	k8sClient := NewK8sClient(WithNamespace(K8Namespace))

	badgerOpts := badger.DefaultOptions("/tmp/badger")
	badgerLogger := BadgerNewLog(zerolog.GlobalLevel())
	badgerOpts.Logger = badgerLogger

	db, err := badger.Open(badgerOpts)

	if err != nil {
		log.Fatal().
			Err(err).
			Str("component", "badger").
			Msg("failed to open Badger DB")
	}

	cronjobDBStore := &CronJobDBStore{
		K8sClient: k8sClient,
		DB:        db,
	}

	sk8lServer := &Sk8lServer{
		Target:         target,
		CronJobDBStore: cronjobDBStore,
		Options: []grpc.DialOption{
			grpc.WithTransportCredentials(serverCreds),
		},
	}

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
	// logger := log.New(os.Stdout, "", log.Ldate|log.Ltime)
	log.Info().
		Msg(fmt.Sprintf("Starting %s server %s on %s", "sk8l", Version(), conn.Addr().String()))
	errCh := make(chan error, 3)

	go func() {
		if err := httpS.ListenAndServeTLS(certFile, certKeyFile); err != nil {
			errCh <- fmt.Errorf("httpS error: %w", err)
		}
	}()

	go func() {
		if err := probeS.Serve(healthConn); err != nil {
			errCh <- fmt.Errorf("probeS error: %w", err)
		}
	}()

	go func() {
		if err := grpcS.Serve(conn); err != nil {
			errCh <- fmt.Errorf("grpcS error: %w", err)
		}
	}()

	rootCtx := context.Background()
	metricsCxt, metricsCancel := context.WithCancel(rootCtx)
	sk8lServer.Run(metricsCxt)

	// Servers shutdown
	log.Info().Msg("Shutdown: setting up")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		log.Error().
			Err(err).
			Msg("Shutdown: sk8l shutting down... got error during server startup")
	case sig := <-quit:
		log.Info().
			Msg(fmt.Sprintf("Shutdown: Got %v signal. sk8l will shut down shortly", sig))
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(rootCtx, 5*time.Second)
	defer shutdownCancel()
	if err := httpS.Shutdown(shutdownCtx); err != nil {
		log.Error().
			Err(err).
			Msg(fmt.Sprintf("Shutdown: error during httpS shutdown"))
	}

	metricsCancel()
	grpcS.GracefulStop()
	probeS.GracefulStop()

	log.Info().Msg("Shutdown: sk8l has stopped")
}
