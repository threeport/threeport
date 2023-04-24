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

// GetClient creates a dynamic client interface and rest mapper from a cluster
// instance.
func GetClient(cluster *v0.ClusterInstance, threeportControlPlane bool) (dynamic.Interface, *meta.RESTMapper, error) {
	restConfig := GetRESTConfig(cluster, threeportControlPlane)

	// create new dynamic client
	dynamicKubeClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create dynamic kube client: %w", err)
	}

	// get the discovery client using rest config
	discoveryClient, err := GetDiscoveryClient(cluster, threeportControlPlane)

	// the rest mapper allows us to deterimine resource types
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get kube API group resources: %w", err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	return dynamicKubeClient, &mapper, nil
}

// GetDiscoveryClient returns a new discovery client for a cluster instance.
func GetDiscoveryClient(cluster *v0.ClusterInstance, threeportControlPlane bool) (*discovery.DiscoveryClient, error) {
	restConfig := GetRESTConfig(cluster, threeportControlPlane)
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new discovery client from rest config: %w", err)
	}

	return discoveryClient, nil
}

// GetRESTConfig returns a REST config for a cluster instance.
func GetRESTConfig(cluster *v0.ClusterInstance, threeportControlPlane bool) *rest.Config {
	// determine if the client is for a control plane component calling the
	// local kube API and set endpoint as needed
	kubeAPIEndpoint := *cluster.APIEndpoint
	if *cluster.ThreeportControlPlaneCluster && threeportControlPlane {
		kubeAPIEndpoint = "kubernetes.default.svc.cluster.local"
	}
	tlsConfig := rest.TLSClientConfig{
		CertData: []byte(*cluster.Certificate),
		KeyData:  []byte(*cluster.Key),
		CAData:   []byte(*cluster.CACertificate),
	}

	return &rest.Config{
		Host:            kubeAPIEndpoint,
		TLSClientConfig: tlsConfig,
	}
}
