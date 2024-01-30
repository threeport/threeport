/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"
	"sync"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/labstack/gommon/log"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/threeport/threeport/internal/agent/controller"
	"github.com/threeport/threeport/internal/agent/notify"
	controlplanev1alpha1 "github.com/threeport/threeport/pkg/agent/api/v1alpha1"
	tpapiclient "github.com/threeport/threeport/pkg/client/v0"
	//+kubebuilder:scaffold:imports
)

var (
	scheme          = runtime.NewScheme()
	setupLog        = ctrl.Log.WithName("setup")
	notificationLog = ctrl.Log.WithName("notification")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(controlplanev1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var threeportAPIServer string
	var authEnabled bool
	flag.StringVar(&threeportAPIServer, "threeport-api-server", "threeport-api-server", "Threepoort REST API server endpoint")
	flag.BoolVar(&authEnabled, "auth-enabled", true, "Enable client certificate authentication (default is true)")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "563c770d.threeport.io",
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// create a context to pass to manager as well as the reconciler so that
	// interrupt signals can be caught so as to stop watch
	managerContext := ctrl.SetupSignalHandler()

	// create REST mapper to discover mappings for resources that are provided
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "failed to get discovery client")
		os.Exit(1)
	}
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		setupLog.Error(err, "failed to get API group resources")
		os.Exit(1)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	// create a dynamic client for controller to use to create watches on
	// threeport-managed resources
	dynamicClient, err := dynamic.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "failed to create dynamic client")
	}

	// create a typed kubernetes client to use for watching known resources like
	// pods and replicasets
	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())

	// create an notify channel for controllers to pass information to
	// notification function that updates threeport API
	notifChan := make(chan notify.ThreeportNotif, 10000)

	// configure http client for calls to threeport API
	threeportAPIClient, err := tpapiclient.GetHTTPClient(authEnabled, "", "", "", "")
	if err != nil {
		log.Error(err, "failed to create http client")
		os.Exit(1)
	}

	// start the notify function and send the manager context to close the
	// channel on interrupt
	var notifyWaitGroup sync.WaitGroup
	go notify.Notify(
		notifChan,
		threeportAPIServer,
		threeportAPIClient,
		notificationLog,
		&notifyWaitGroup,
	)
	go func() {
		select {
		case <-managerContext.Done():
			close(notifChan)
			return
		}
	}()

	// set up controller for threeport workload resources
	if err = (&controller.ThreeportWorkloadReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		ManagerContext: managerContext,
		RESTMapper:     mapper,
		KubeClient:     clientset,
		DynamicClient:  dynamicClient,
		RESTConfig:     mgr.GetConfig(),
		NotifChan:      &notifChan,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ThreeportWorkload")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(managerContext); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	// wait for notify function to complete to flush any pending updates to
	// threeport API
	notifyWaitGroup.Wait()
}
