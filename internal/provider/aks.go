package provider

import (
	"context"
	"fmt"

	"github.com/nukleros/azure-builder/pkg/aks"
	azconfig "github.com/nukleros/azure-builder/pkg/config"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubernetesRuntimeInfraAKS represents the infrastructure for a threeport-managed AKS
// cluster.
type KubernetesRuntimeInfraAKS struct {
	// The Azure-Builder AKS config file used to create the cluster
	AksConfig azconfig.AzureResourceConfig

	// The azure credentials config
	AzureCredentialsConfig azconfig.AzureCredentialsConfig
}

// Create install the Kubernetes cluster using Azure AKS for threeport workloads
func (i *KubernetesRuntimeInfraAKS) Create() (*kube.KubeConnectionInfo, error) {
	if err := i.AzureCredentialsConfig.ValidateNotNull(); err != nil {
		return nil, fmt.Errorf("could not validate credentials config: %w", err)
	}

	_, err := aks.CreateAksCluster(&i.AksConfig, &i.AzureCredentialsConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create aks cluster: %w", err)
	}

	return i.GetConnection()
}

// Delete deletes an Azure AKS cluster.
func (i *KubernetesRuntimeInfraAKS) Delete() error {
	if err := i.AzureCredentialsConfig.ValidateNotNull(); err != nil {
		return fmt.Errorf("could not validate credentials config: %w", err)
	}

	err := aks.DeleteAksCluster(&i.AksConfig, &i.AzureCredentialsConfig)
	if err != nil {
		return fmt.Errorf("could not delete aks cluster: %w", err)
	}

	return nil
}

// GetConnection get the latest connection info for authentication to an AKS cluster.
func (i *KubernetesRuntimeInfraAKS) GetConnection() (*kube.KubeConnectionInfo, error) {
	ctx := context.TODO()
	aksClusterKubeConfig, err := aks.GetKubeConfigForCluster(ctx, &i.AksConfig, &i.AzureCredentialsConfig)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve kube config for cluster: %w")
	}

	var clientCfg clientcmd.ClientConfig

	clientCfg, err = clientcmd.NewClientConfigFromBytes(aksClusterKubeConfig)
	if err != nil {
		return nil, fmt.Errorf("could not create kube client config from kube config: %w", err)
	}

	var restCfg *rest.Config

	restCfg, err = clientCfg.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("could not create kube rest config from client config: %w", err)
	}

	kubeConn := &kube.KubeConnectionInfo{
		APIEndpoint:   restCfg.Host,
		CACertificate: string(restCfg.CAData),
		AKSToken:      restCfg.BearerToken,
	}

	return kubeConn, nil
}
