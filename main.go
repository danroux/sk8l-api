package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/template"
	"time"

	"github.com/danroux/sk8l/protos"
	badger "github.com/dgraph-io/badger/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

var (
	K8_NAMESPACE    = os.Getenv("K8_NAMESPACE")
	API_PORT        = os.Getenv("SK8L_SERVICE_PORT_SK8L_API")
	API_HEALTH_PORT = os.Getenv("SK8L_SERVICE_PORT_SK8L_API_HEALTH")
	METRICS_PORT    = os.Getenv("SK8L_SERVICE_PORT_SK8L_API_METRICS")
	certFile        = filepath.Join("/etc", "sk8l-certs", "server-cert.pem")
	certKeyFile     = filepath.Join("/etc", "sk8l-certs", "server-key.pem")
	caFile          = filepath.Join("/etc", "sk8l-certs", "ca-cert.pem")
	METRIC_PREFIX   = fmt.Sprintf("sk8l_%s", K8_NAMESPACE)
)

func main() {
	serverTLSConfig, err := setupTLS(certFile, certKeyFile, caFile)
	target := fmt.Sprintf("0.0.0.0:%s", API_PORT)
	conn, err := net.Listen("tcp", target)

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
	k8sClient := NewK8sClient(WithNamespace(K8_NAMESPACE))

	db, err := badger.Open(badger.DefaultOptions("/tmp/badger"))

	sk8lServer := &Sk8lServer{
		K8sClient: k8sClient,
		DB:        db,
		Target:    target,
		Options: []grpc.DialOption{
			grpc.WithTransportCredentials(serverCreds),
		},
	}

	healthgrpc.RegisterHealthServer(probeS, sk8lServer)
	protos.RegisterCronjobServer(grpcS, sk8lServer)

	sk8lServer.WatchPods()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/cronjobs", nvidiaHandler)

	httpS := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%s", METRICS_PORT),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		TLSConfig:    serverTLSConfig,
	}
	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime)

	logger.Printf("Starting %s server %s on %s", "sk8l", Version(), conn.Addr().String())

	go func() {
		err = httpS.ListenAndServeTLS(certFile, certKeyFile)
	}()

	if err != nil {
		log.Fatal("httpS error", err)
	}

	go func() {
		err = probeS.Serve(healthConn)

		if err != nil {
			log.Fatal("httpS error", err)
		}

	}()

	go func() {
		err = grpcS.Serve(conn)
		if err != nil {
			log.Fatal("grpcS error", err)
		}
	}()

	x := context.Background()
	metricsCxt, metricsCancel := context.WithCancel(x)
	recordMetrics(metricsCxt, sk8lServer)

	// Servers shutdown
	log.Printf("Shutdown: setting up")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	sig := <-c

	log.Printf("Shutdown: Got %v signal. sk8l will shut down in x seconds\n", sig)

	ctx, cancel := context.WithTimeout(x, 5*time.Second)
	defer cancel()
	if err := httpS.Shutdown(ctx); err != nil {
		log.Printf("Shutdown: Server forced to shutdown: %v\n", err)
	}

	metricsCancel()
	grpcS.GracefulStop()

	log.Println("Shutdown: sk8l has stopped")
}

func nvidiaHandler(w http.ResponseWriter, r *http.Request) {
	type DataSource struct {
		Type, Uid string
	}

	type Target struct {
		Expr, LegendFormat string
		DataSource         *DataSource
	}

	type Panel struct {
		Targets    []*Target
		DataSource *DataSource
	}

	var targets = []*Target{
		&Target{
			Expr:         fmt.Sprintf("%s_%s", METRIC_PREFIX, "completed_cronjobs_total"),
			LegendFormat: "{{__name__}}",
			DataSource: &DataSource{
				Type: "prometheus",
				Uid:  "${DS_PROMETHEUS}",
			},
		},
		&Target{
			Expr:         fmt.Sprintf("%s_%s", METRIC_PREFIX, "failing_cronjobs_total"),
			LegendFormat: "{{__name__}}",
			DataSource: &DataSource{
				Type: "prometheus",
				Uid:  "${DS_PROMETHEUS}",
			},
		},
		&Target{
			Expr:         fmt.Sprintf("%s_%s", METRIC_PREFIX, "registered_cronjobs_total"),
			LegendFormat: "{{__name__}}",
			DataSource: &DataSource{
				Type: "prometheus",
				Uid:  "${DS_PROMETHEUS}",
			},
		},
	}

	var panels = []Panel{
		Panel{
			Targets: targets,
			DataSource: &DataSource{
				Type: "prometheus",
				Uid:  "${DS_PROMETHEUS}",
			},
		},
	}

	// Create a new template and parse the letter into it.
	var tmplFile = "annotations.tmpl"
	t := template.Must(template.New(tmplFile).ParseFiles(tmplFile))
	// t = t.Funcs(template.FuncMap{"StringsJoin": strings.Join})

	var b bytes.Buffer
	err := t.Execute(&b, panels)
	if err != nil {
		log.Println("executing template:", err)
	}

	// Set the "Content-Type: application/json" header on the response. If you forget to // this, Go will default to sending a "Content-Type: text/plain; charset=utf-8"
	// header instead.
	w.Header().Set("Content-Type", "application/json")
	// Write the JSON as the HTTP response body.
	w.Write(b.Bytes())
}
