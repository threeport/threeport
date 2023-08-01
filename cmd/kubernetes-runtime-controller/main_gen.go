// generated by 'threeport-codegen controller' - do not edit

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
	kubernetesruntime "github.com/threeport/threeport/internal/kubernetesruntime"
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
	var kubernetesRuntimeDefinitionConcurrentReconciles = flag.Int(
		"KubernetesRuntimeDefinition-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for kubernetes runtime definitions",
	)
	var kubernetesRuntimeInstanceConcurrentReconciles = flag.Int(
		"KubernetesRuntimeInstance-concurrent-reconciles",
		1,
		"Number of concurrent reconcilers to run for kubernetes runtime instances",
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

	if *help {
		showHelpAndExit(0)
	}

	// controller instance ID
	controllerID := uuid.New()

	// logging setup
	var log logr.Logger
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
		Bucket:      kubernetesruntime.LockBucketName,
		Description: kubernetesruntime.LockBucketDescr,
		TTL:         time.Minute * 20,
	}
	kv, err := controller.CreateLockBucketIfNotExists(js, &kvConfig)
	if err != nil {
		log.Error(err, "failed to bind to JetStream key-value locking bucket", "lockBucketName", kubernetesruntime.LockBucketName)
		os.Exit(1)
	}

	// check to ensure kubernetes-runtime stream has been created by API
	kubernetesRuntimeStreamNameFound := false
	for stream := range js.StreamNames() {
		if stream == v0.KubernetesRuntimeStreamName {
			kubernetesRuntimeStreamNameFound = true
		}
	}
	if !kubernetesRuntimeStreamNameFound {
		log.Error(errors.New("JetStream stream not found"), "failed to find stream with kubernetes-runtime stream name", "kubernetesRuntimeStreamName", v0.KubernetesRuntimeStreamName)
		os.Exit(1)
	}

	// create a channel and wait group used for graceful shut downs
	var shutdownChans []chan bool
	var shutdownWait sync.WaitGroup

	// configure http client for calls to threeport API
	apiClient, err := client.GetHTTPClient(*authEnabled, "", "", "")
	if err != nil {
		log.Error(err, "failed to create http client")
		os.Exit(1)
	}

	// configure and start reconcilers
	var reconcilerConfigs []controller.ReconcilerConfig
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *kubernetesRuntimeDefinitionConcurrentReconciles,
		Name:                 "KubernetesRuntimeDefinitionReconciler",
		NotifSubject:         v0.KubernetesRuntimeDefinitionSubject,
		ObjectType:           v0.ObjectTypeKubernetesRuntimeDefinition,
		ReconcileFunc:        kubernetesruntime.KubernetesRuntimeDefinitionReconciler,
	})
	reconcilerConfigs = append(reconcilerConfigs, controller.ReconcilerConfig{
		ConcurrentReconciles: *kubernetesRuntimeInstanceConcurrentReconciles,
		Name:                 "KubernetesRuntimeInstanceReconciler",
		NotifSubject:         v0.KubernetesRuntimeInstanceSubject,
		ObjectType:           v0.ObjectTypeKubernetesRuntimeInstance,
		ReconcileFunc:        kubernetesruntime.KubernetesRuntimeInstanceReconciler,
	})

	for _, r := range reconcilerConfigs {

		// create JetStream consumer
		consumer := r.Name + "Consumer"
		js.AddConsumer(v0.KubernetesRuntimeStreamName, &natsgo.ConsumerConfig{
			AckPolicy:     natsgo.AckExplicitPolicy,
			Durable:       consumer,
			FilterSubject: r.NotifSubject,
		})

		// create durable pull subscription
		sub, err := js.PullSubscribe(r.NotifSubject, consumer, natsgo.BindStream(v0.KubernetesRuntimeStreamName))
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
		"kubernetes-runtime controller started",
		"version", version.GetVersion(),
		"controllerID", controllerID.String(),
		"NATSConnection", natsConn,
		"lockBucketName", kubernetesruntime.LockBucketName,
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
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error(err, "failed to run server for shutdown endpoint")
	}

	// wait for reconcilers to finish
	shutdownWait.Wait()

	log.Info("kubernetes-runtime controller shutting down")
	os.Exit(0)
}
func showHelpAndExit(exitCode int) {
	fmt.Printf("Usage: threeport-kubernetes-runtime-controller [options]\n")
	fmt.Println("options:")
	flag.PrintDefaults()
	os.Exit(exitCode)
}
