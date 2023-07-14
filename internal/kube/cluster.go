package kube

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// GetClient creates a dynamic client interface and rest mapper from a
// kubernetes cluster instance.
func GetClient(runtime *v0.KubernetesRuntimeInstance, threeportControlPlane bool) (dynamic.Interface, *meta.RESTMapper, error) {
	restConfig := getRESTConfig(runtime, threeportControlPlane)

	// create new dynamic client
	dynamicKubeClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create dynamic kube client: %w", err)
	}

	// get the discovery client using rest config
	discoveryClient, err := GetDiscoveryClient(runtime, threeportControlPlane)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get discovery client for kube API: %w", err)
	}

	// the rest mapper allows us to deterimine resource types
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get kube API group resources: %w", err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	return dynamicKubeClient, &mapper, nil
}

// GetDiscoveryClient returns a new discovery client for a kubernetes cluster
// instance.
func GetDiscoveryClient(runtime *v0.KubernetesRuntimeInstance, threeportControlPlane bool) (*discovery.DiscoveryClient, error) {
	restConfig := getRESTConfig(runtime, threeportControlPlane)
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new discovery client from rest config: %w", err)
	}

	return discoveryClient, nil
}

// getRESTConfig returns a REST config for a cluster instance.
func getRESTConfig(runtime *v0.KubernetesRuntimeInstance, threeportControlPlane bool) *rest.Config {
	// determine if the client is for a control plane component calling the
	// local kube API and set endpoint as needed
	kubeAPIEndpoint := *runtime.APIEndpoint
	if *runtime.ThreeportControlPlaneHost && threeportControlPlane {
		kubeAPIEndpoint = "kubernetes.default.svc.cluster.local"
	}

	// set tlsConfig according to authN type
	var restConfig rest.Config
	switch {
	case runtime.Certificate != nil && runtime.Key != nil:
		tlsConfig := rest.TLSClientConfig{
			CertData: []byte(*runtime.Certificate),
			KeyData:  []byte(*runtime.Key),
			CAData:   []byte(*runtime.CACertificate),
		}
		restConfig = rest.Config{
			Host:            kubeAPIEndpoint,
			TLSClientConfig: tlsConfig,
		}
	case runtime.ConnectionToken != nil:
		tlsConfig := rest.TLSClientConfig{
			CAData: []byte(*runtime.CACertificate),
		}
		restConfig = rest.Config{
			Host:            kubeAPIEndpoint,
			BearerToken:     *runtime.ConnectionToken,
			TLSClientConfig: tlsConfig,
		}
	}

	return &restConfig
}
