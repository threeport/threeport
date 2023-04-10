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
func GetClient(cluster *v0.ClusterInstance) (dynamic.Interface, *meta.RESTMapper, error) {
	tlsConfig := rest.TLSClientConfig{
		CertData: []byte(*cluster.Certificate),
		KeyData:  []byte(*cluster.Key),
		CAData:   []byte(*cluster.CACertificate),
	}
	restConfig := rest.Config{
		Host:            *cluster.APIEndpoint,
		TLSClientConfig: tlsConfig,
	}

	// create new dynamic client
	dynamicKubeClient, err := dynamic.NewForConfig(&restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create dynamic kube client: %w", err)
	}

	// the rest mapper allows us to deterimine resource types
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(&restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get discovery client: %w", err)
	}
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get kube API group resources: %w", err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	return dynamicKubeClient, &mapper, nil
}
