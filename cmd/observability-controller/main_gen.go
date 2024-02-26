// generated by 'threeport-sdk codegen controller' - do not edit

package main

import (
	"context"
	"errors"
	"fmt"
	logr "github.com/go-logr/logr"
	zapr "github.com/go-logr/zapr"
	uuid "github.com/google/uuid"
	flag "github.com/namsral/flag"
	natsgo "github.com/nats-io/nats.go"
	observability "github.com/threeport/threeport/internal/observability"
	version "github.com/threeport/threeport/internal/version"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	zap "go.uber.org/zap"
	"net/http"
	"os"
	"sync"
	"time"
)

func main() {
	// flags
	var observabilityStackDefinitionConcurrentReconciles = flag.Int(
		"ObservabilityStackDefinition-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for observability stack definitions",
	)
	var observabilityStackInstanceConcurrentReconciles = flag.Int(
		"ObservabilityStackInstance-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for observability stack instances",
	)
	var observabilityDashboardDefinitionConcurrentReconciles = flag.Int(
		"ObservabilityDashboardDefinition-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for observability dashboard definitions",
	)
	var observabilityDashboardInstanceConcurrentReconciles = flag.Int(
		"ObservabilityDashboardInstance-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for observability dashboard instances",
	)
	var metricsDefinitionConcurrentReconciles = flag.Int(
		"MetricsDefinition-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for metrics definitions",
	)
	var metricsInstanceConcurrentReconciles = flag.Int(
		"MetricsInstance-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for metrics instances",
	)
	var loggingDefinitionConcurrentReconciles = flag.Int(
		"LoggingDefinition-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for logging definitions",
	)
	var loggingInstanceConcurrentReconciles = flag.Int(
		"LoggingInstance-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for logging instances",
	)

	var apiServer = flag.String("api-server", "threeport-api-server", "Threepoort REST API server endpoint")
	var msgBrokerHost = flag.String("msg-broker-host", "", "Threeport message broker hostname")
	var msgBrokerPort = flag.String("msg-broker-port", "", "Threeport message broker port")
	var msgBrokerUser = flag.String("msg-broker-user", "", "Threeport message broker user")
	var msgBrokerPassword = flag.String("msg-broker-password", "", "Threeport message broker user password")
	var shutdownPort = flag.String("shutdown-port", "8181", "Port to listen for shutdown calls")
	var verbose = flag.Bool("verbose", false, "Write logs with v(1).InfoLevel and above")
	var help = flag.Bool("help", false, "Show help info")
	var authEnabled = flag.Bool("auth-enabled", true, "Enable client certificate authentication (default is true)")
	flag.Parse()

	var log logr.Logger
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		log.Error(errors.New("environment variable ENCRYPTION_KEY is not set"), "encryption key not found")
	}

	if *help {
		showHelpAndExit(0)
	}

	// controller instance ID
	controllerID := uuid.New()

	// logging setup
	switch *verbose {
	case true:
		zapLog, err := zap.NewDevelopment()
		if err != nil {
			panic(fmt.Sprintf("failed to set up development logging: %v", err))
		}
		log = zapr.NewLogger(zapLog).WithValues("controllerID", controllerID)
	default:
		zapLog, err := zap.NewProduction()
		if err != nil {
			panic(fmt.Sprintf("failed to set up production logging: %v", err))
		}
		log = zapr.NewLogger(zapLog).WithValues("controllerID", controllerID)
	}

	// connect to NATS server
	natsConn := fmt.Sprintf(
		"nats://%s:%s@%s:%s",
		*msgBrokerUser,
		*msgBrokerPassword,
		*msgBrokerHost,
		*msgBrokerPort,
	)
	nc, err := natsgo.Connect(natsConn)
	if err != nil {
		log.Error(err, "failed to connect to NATS message broker", "NATSConnection", natsConn)
		os.Exit(1)
	}

	// create JetStream context
	js, err := nc.JetStream(natsgo.PublishAsyncMaxPending(256))
	if err != nil {
		log.Error(err, "failed to create JetStream context")
		os.Exit(1)
	}

	// JetStream key-value store setup
	kvConfig := natsgo.KeyValueConfig{
		Bucket:      observability.LockBucketName,
		Description: observability.LockBucketDescr,
		TTL:         time.Minute * 20,
	}
	kv, err := controller.CreateLockBucketIfNotExists(js, &kvConfig)
	if err != nil {
		log.Error(err, "failed to bind to JetStream key-value locking bucket", "lockBucketName", observability.LockBucketName)
		os.Exit(1)
	}

	// check to ensure observability stream has been created by API
	observabilityStreamNameFound := false
	for stream := range js.StreamNames() {
		if stream == v0.ObservabilityStreamName {
			observabilityStreamNameFound = true
		}
	}
	if !observabilityStreamNameFound {
		log.Error(errors.New("JetStream stream not found"), "failed to find stream with observability stream name", "observabilityStreamName", v0.ObservabilityStreamName)
		os.Exit(1)
	}

	// create a channel and wait group used for graceful shut downs
	var shutdownChans []chan bool
	var shutdownWait sync.WaitGroup

	// configure http client for calls to threeport API
	apiClient, err := client.GetHTTPClient(*authEnabled, "", "", "", "")
	if err != nil {
		log.Error(err, "failed to create http client")
		os.Exit(1)
	}

	// configure and start reconcilers
	var reconcilerConfigs []controller.ReconcilerConfig
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *observabilityStackDefinitionConcurrentReconciles,
		Name:                 "ObservabilityStackDefinitionReconciler",
		NotifSubject:         v0.ObservabilityStackDefinitionSubject,
		ObjectType:           v0.ObjectTypeObservabilityStackDefinition,
		ReconcileFunc:        observability.ObservabilityStackDefinitionReconciler,
	})
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *observabilityStackInstanceConcurrentReconciles,
		Name:                 "ObservabilityStackInstanceReconciler",
		NotifSubject:         v0.ObservabilityStackInstanceSubject,
		ObjectType:           v0.ObjectTypeObservabilityStackInstance,
		ReconcileFunc:        observability.ObservabilityStackInstanceReconciler,
	})
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *observabilityDashboardDefinitionConcurrentReconciles,
		Name:                 "ObservabilityDashboardDefinitionReconciler",
		NotifSubject:         v0.ObservabilityDashboardDefinitionSubject,
		ObjectType:           v0.ObjectTypeObservabilityDashboardDefinition,
		ReconcileFunc:        observability.ObservabilityDashboardDefinitionReconciler,
	})
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *observabilityDashboardInstanceConcurrentReconciles,
		Name:                 "ObservabilityDashboardInstanceReconciler",
		NotifSubject:         v0.ObservabilityDashboardInstanceSubject,
		ObjectType:           v0.ObjectTypeObservabilityDashboardInstance,
		ReconcileFunc:        observability.ObservabilityDashboardInstanceReconciler,
	})
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *metricsDefinitionConcurrentReconciles,
		Name:                 "MetricsDefinitionReconciler",
		NotifSubject:         v0.MetricsDefinitionSubject,
		ObjectType:           v0.ObjectTypeMetricsDefinition,
		ReconcileFunc:        observability.MetricsDefinitionReconciler,
	})
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *metricsInstanceConcurrentReconciles,
		Name:                 "MetricsInstanceReconciler",
		NotifSubject:         v0.MetricsInstanceSubject,
		ObjectType:           v0.ObjectTypeMetricsInstance,
		ReconcileFunc:        observability.MetricsInstanceReconciler,
	})
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *loggingDefinitionConcurrentReconciles,
		Name:                 "LoggingDefinitionReconciler",
		NotifSubject:         v0.LoggingDefinitionSubject,
		ObjectType:           v0.ObjectTypeLoggingDefinition,
		ReconcileFunc:        observability.LoggingDefinitionReconciler,
	})
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *loggingInstanceConcurrentReconciles,
		Name:                 "LoggingInstanceReconciler",
		NotifSubject:         v0.LoggingInstanceSubject,
		ObjectType:           v0.ObjectTypeLoggingInstance,
		ReconcileFunc:        observability.LoggingInstanceReconciler,
	})

	for _, r := range reconcilerConfigs {

		// create JetStream consumer
		consumer := r.Name + "Consumer"
		js.AddConsumer(v0.ObservabilityStreamName, &natsgo.ConsumerConfig{
			AckPolicy:     natsgo.AckExplicitPolicy,
			Durable:       consumer,
			FilterSubject: r.NotifSubject,
		})

		// create durable pull subscription
		sub, err := js.PullSubscribe(r.NotifSubject, consumer, natsgo.BindStream(v0.ObservabilityStreamName))
		if err != nil {
			log.Error(err, "failed to create pull subscription for reconciler notifications", "reconcilerName", r.Name)
			os.Exit(1)
		}

		// create exit channel
		shutdownChan := make(chan bool, 1)
		shutdownChans = append(shutdownChans, shutdownChan)

		// create reconciler
		reconciler := controller.Reconciler{
			APIClient:        apiClient,
			APIServer:        *apiServer,
			ControllerID:     controllerID,
			EncryptionKey:    encryptionKey,
			JetStreamContext: js,
			KeyValue:         kv,
			Log:              &log,
			Name:             r.Name,
			ObjectType:       r.ObjectType,
			Shutdown:         shutdownChan,
			ShutdownWait:     &shutdownWait,
			Sub:              sub,
		}

		// start reconciler
		go r.ReconcileFunc(&reconciler)
	}

	log.Info(
		"observability controller started",
		"version", version.GetVersion(),
		"controllerID", controllerID.String(),
		"NATSConnection", natsConn,
		"lockBucketName", observability.LockBucketName,
	)

	// add a shutdown endpoint for graceful shutdowns
	mux := http.NewServeMux()
	server := http.Server{
		Addr:    ":" + *shutdownPort,
		Handler: mux,
	}
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		for _, c := range shutdownChans {
			c <- true
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "shutting down\n")
		shutdownWait.Add(1)
		go func() {
			server.Shutdown(context.Background())
			shutdownWait.Done()
		}()
	})

	// set up health check endpoint
	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	go http.ListenAndServe(":8081", nil)

	// run shutdown endpoint server
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error(err, "failed to run server for shutdown endpoint")
	}

	// wait for reconcilers to finish
	shutdownWait.Wait()

	log.Info("observability controller shutting down")
	os.Exit(0)
}
func showHelpAndExit(exitCode int) {
	fmt.Printf("Usage: threeport-observability-controller [options]\n")
	fmt.Println("options:")
	flag.PrintDefaults()
	os.Exit(exitCode)
}
