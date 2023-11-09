// GOEXPERIMENT=loopvar

package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/danroux/sk8l/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

var (
	API_PORT        = os.Getenv("SK8L_SERVICE_PORT_SK8L_API")
	API_HEALTH_PORT = os.Getenv("SK8L_SERVICE_PORT_SK8L_API_HEALTH")
	METRICS_PORT    = os.Getenv("SK8L_SERVICE_PORT_SK8L_API_METRICS")
	certFile        = filepath.Join("/etc", "sk8l-certs", "server-cert.pem")
	certKeyFile     = filepath.Join("/etc", "sk8l-certs", "server-key.pem")
	caFile          = filepath.Join("/etc", "sk8l-certs", "ca-cert.pem")
)

func main() {
	serverTLSConfig, err := setupTLS(certFile, certKeyFile, caFile)
	conn, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", API_PORT))

	if err != nil {
		log.Fatal("tlsListen error:", err)
	}

	healthConn, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", API_HEALTH_PORT))

	if err != nil {
		log.Fatal("Health Probe Listen error:", err)
	}

	serverCreds := credentials.NewTLS(serverTLSConfig)
	creds := grpc.Creds(serverCreds)

	grpcS := grpc.NewServer(creds)
	probeS := grpc.NewServer()

	log.Printf("grpcS creds %v", creds)
	k8sClient := NewK8sClient(WithNamespace(os.Getenv("K8_NAMESPACE")))
	sk8lServer := &Sk8lServer{
		K8sClient: k8sClient,
	}

	recordMetrics(sk8lServer)

	healthgrpc.RegisterHealthServer(probeS, sk8lServer)
	protos.RegisterCronjobServer(grpcS, sk8lServer)

	http.Handle("/metrics", promhttp.Handler())

	httpS := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%s", METRICS_PORT),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		TLSConfig:    serverTLSConfig,
	}
	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime)

	logger.Printf("this is not wrong! starting %s server on %s", "sk8l", conn.Addr().String())

	go func() {
		err = httpS.ListenAndServeTLS(certFile, certKeyFile)
	}()

	if err != nil {
		log.Fatal("httpS error", err)
	}

	go func() {
		err = probeS.Serve(healthConn)
	}()

	if err != nil {
		log.Fatal("httpS error", err)
	}

	err = grpcS.Serve(conn)

	if err != nil {
		log.Fatal("grpcS error", err)
	}
}
