package helmworkload

import (
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// CustomRESTClientGetter implements the genericclioptions.RESTClientGetter
// interface.
type CustomRESTClientGetter struct {
	KubernetesRuntimeInstance *v0.KubernetesRuntimeInstance
	ApiClient                 *http.Client
	ApiServer                 string
	EncryptionKey             string
}

// ToRESTConfig returns a REST config for a Kubernetes runtime.
func (c *CustomRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	restConfig, err := kube.GetRestConfig(
		c.KubernetesRuntimeInstance,
		true,
		c.ApiClient,
		c.ApiServer,
		c.EncryptionKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to Kubernetes REST config: %w", err)
	}

	return restConfig, nil
}

// ToDiscoveryClient returns a cached discover interface for a Kubernetes
// runtime.
func (c *CustomRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	discoveryClient, err := kube.GetDiscoveryClient(
		c.KubernetesRuntimeInstance,
		true,
		c.ApiClient,
		c.ApiServer,
		c.EncryptionKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to Kubernetes discover interface: %w", err)
	}

	cachedDiscoveryClient := memory.NewMemCacheClient(discoveryClient)

	return cachedDiscoveryClient, nil
}

// ToRESTMapper returns a REST mapper for a Kubernetes runtime.
func (c *CustomRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	_, restMapper, err := kube.GetClient(
		c.KubernetesRuntimeInstance,
		true,
		c.ApiClient,
		c.ApiServer,
		c.EncryptionKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to Kubernetes REST mapper: %w", err)
	}

	return *restMapper, nil
}

// ToRawKubeConfigLoader returns a raw default client config for a Kubernetes
// runtime.
func (c *CustomRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return clientcmd.NewDefaultClientConfig(*api.NewConfig(), &clientcmd.ConfigOverrides{})
}
