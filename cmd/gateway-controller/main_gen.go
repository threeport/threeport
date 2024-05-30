// generated by 'threeport-sdk gen' - do not edit

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
	gateway "github.com/threeport/threeport/internal/gateway"
	notif "github.com/threeport/threeport/internal/gateway/notif"
	version "github.com/threeport/threeport/internal/version"
	tpclient_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	zap "go.uber.org/zap"
	"net/http"
	"os"
	"sync"
	"time"
)

func main() {
	// flags
	var gatewayDefinitionConcurrentReconciles = flag.Int(
		"gateway-definition-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for gateway definitions",
	)
	var gatewayInstanceConcurrentReconciles = flag.Int(
		"gateway-instance-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for gateway instances",
	)
	var domainNameInstanceConcurrentReconciles = flag.Int(
		"domain-name-instance-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for domain name instances",
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
		Bucket:      gateway.LockBucketName,
		Description: gateway.LockBucketDescr,
		TTL:         time.Minute * 20,
	}
	kv, err := controller.CreateLockBucketIfNotExists(js, &kvConfig)
	if err != nil {
		log.Error(err, "failed to bind to JetStream key-value locking bucket", "lockBucketName", gateway.LockBucketName)
		os.Exit(1)
	}

	// check to ensure gateway stream has been created by API
	gatewayStreamNameFound := false
	for stream := range js.StreamNames() {
		if stream == notif.GatewayStreamName {
			gatewayStreamNameFound = true
		}
	}
	if !gatewayStreamNameFound {
		log.Error(errors.New("JetStream stream not found"), "failed to find stream with gateway stream name", "gatewayStreamName", notif.GatewayStreamName)
		os.Exit(1)
	}

	// create a channel and wait group used for graceful shut downs
	var shutdownChans []chan bool
	var shutdownWait sync.WaitGroup

	// configure http client for calls to threeport API
	apiClient, err := tpclient_lib.GetHTTPClient(*authEnabled, "", "", "", "")
	if err != nil {
		log.Error(err, "failed to create http client")
		os.Exit(1)
	}

	// configure and start reconcilers
	var reconcilerConfigs []controller.ReconcilerConfig
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *gatewayDefinitionConcurrentReconciles,
		Name:                 "GatewayDefinitionReconciler",
		NotifSubject:         notif.GatewayDefinitionSubject,
		ReconcileFunc:        gateway.GatewayDefinitionReconciler,
	})
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *gatewayInstanceConcurrentReconciles,
		Name:                 "GatewayInstanceReconciler",
		NotifSubject:         notif.GatewayInstanceSubject,
		ReconcileFunc:        gateway.GatewayInstanceReconciler,
	})
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *domainNameInstanceConcurrentReconciles,
		Name:                 "DomainNameInstanceReconciler",
		NotifSubject:         notif.DomainNameInstanceSubject,
		ReconcileFunc:        gateway.DomainNameInstanceReconciler,
	})

	for _, r := range reconcilerConfigs {

		// create JetStream consumer
		consumer := r.Name + "Consumer"
		js.AddConsumer(notif.GatewayStreamName, &natsgo.ConsumerConfig{
			AckPolicy:     natsgo.AckExplicitPolicy,
			Durable:       consumer,
			FilterSubject: r.NotifSubject,
		})

		// create durable pull subscription
		sub, err := js.PullSubscribe(r.NotifSubject, consumer, natsgo.BindStream(notif.GatewayStreamName))
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
			Shutdown:         shutdownChan,
			ShutdownWait:     &shutdownWait,
			Sub:              sub,
		}

		// start reconciler
		go r.ReconcileFunc(&reconciler)
	}

	log.Info(
		"gateway controller started",
		"version", version.GetVersion(),
		"controllerID", controllerID.String(),
		"NATSConnection", natsConn,
		"lockBucketName", gateway.LockBucketName,
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

	log.Info("gateway controller shutting down")
	os.Exit(0)
}
func showHelpAndExit(exitCode int) {
	fmt.Printf("Usage: threeport-gateway-controller [options]\n")
	fmt.Println("options:")
	flag.PrintDefaults()
	os.Exit(exitCode)
}
