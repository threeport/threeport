package kube

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/nukleros/eks-cluster/pkg/connection"
	"github.com/nukleros/eks-cluster/pkg/resource"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// GetClient creates a dynamic client interface and rest mapper from a
// kubernetes cluster instance.
func GetClient(
	runtime *v0.KubernetesRuntimeInstance,
	threeportControlPlane bool,
	threeportAPIClient *http.Client,
	threeportAPIEndpoint string,
) (dynamic.Interface, *meta.RESTMapper, error) {
	restConfig, err := getRESTConfig(runtime, threeportControlPlane, threeportAPIClient, threeportAPIEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get REST config for kubernetes runtime instance: %w", err)
	}

	// create new dynamic client
	dynamicKubeClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create dynamic kube client: %w", err)
	}

	// get the discovery client using rest config
	discoveryClient, err := GetDiscoveryClient(
		runtime,
		threeportControlPlane,
		threeportAPIClient,
		threeportAPIEndpoint,
	)
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
func GetDiscoveryClient(
	runtime *v0.KubernetesRuntimeInstance,
	threeportControlPlane bool,
	threeportAPIClient *http.Client,
	threeportAPIEndpoint string,
) (*discovery.DiscoveryClient, error) {
	restConfig, err := getRESTConfig(
		runtime,
		threeportControlPlane,
		threeportAPIClient,
		threeportAPIEndpoint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get REST config for kubernetes runtime instance: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new discovery client from rest config: %w", err)
	}

	return discoveryClient, nil
}

// getRESTConfig returns a REST config for a cluster instance.
func getRESTConfig(
	runtime *v0.KubernetesRuntimeInstance,
	threeportControlPlane bool,
	threeportAPIClient *http.Client,
	threeportAPIEndpoint string,
) (*rest.Config, error) {
	if runtime.APIEndpoint == nil {
		return nil, errors.New("cannot get REST config without API endpoint")
	}

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

	// ensure rest config is valid
	if err := testRESTConfig(&restConfig); err != nil {
		if kubeerrors.IsUnauthorized(err) {
			// in unauthorized, refresh token
			definition, err := client.GetKubernetesRuntimeDefinitionByID(
				threeportAPIClient,
				threeportAPIEndpoint,
				*runtime.KubernetesRuntimeDefinitionID,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to get kubernetes runtime definition by ID %d: %w", runtime.KubernetesRuntimeDefinitionID, err)
			}

			switch *definition.InfraProvider {
			case v0.KubernetesRuntimeInfraProviderEKS:
				config, err := refreshEKSConnection(
					runtime,
					threeportAPIClient,
					threeportAPIEndpoint,
				)
				if err != nil {
					return nil, fmt.Errorf("failed to refresh connection token for EKS cluster: %w", err)
				}
				restConfig = *config
			default:
				return nil, errors.New(
					fmt.Sprintf("unable to refresh connection token for unsupported infra provider %s:", definition.InfraProvider),
				)
			}
		} else {
			return nil, err
		}
	}

	return &restConfig, nil
}

// testRESTConfig calls the target kubernetes API using a rest.Config to ensure
// it works before use.
func testRESTConfig(restConfig *rest.Config) error {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	_, err = clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	return nil
}

// refreshEKSConnection retrieves a new EKS token when it expires.
func refreshEKSConnection(
	runtimeInstance *v0.KubernetesRuntimeInstance,
	threeportAPIClient *http.Client,
	threeportAPIEndpoint string,
) (*rest.Config, error) {
	// get EKS runtime instance
	eksRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(
		threeportAPIClient,
		threeportAPIEndpoint,
		*runtimeInstance.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS EKS kubernetes runtime instance by kubernetes runtime instance ID %d: %w", runtimeInstance.ID, err)
	}

	// get EKS runtime definition
	eksRuntimeDefinition, err := client.GetAwsEksKubernetesRuntimeDefinitionByID(
		threeportAPIClient,
		threeportAPIEndpoint,
		*eksRuntimeInstance.AwsEksKubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to AWS EKS kubernetes runtime definition by ID %d: %w", eksRuntimeInstance.AwsEksKubernetesRuntimeDefinitionID, err)
	}

	// get AWS account
	awsAccount, err := client.GetAwsAccountByID(
		threeportAPIClient,
		threeportAPIEndpoint,
		*eksRuntimeDefinition.AwsAccountID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS account by ID %d: %w", eksRuntimeDefinition.AwsAccountID, err)
	}

	// create AWS config to get new token
	awsConfig, err := resource.LoadAWSConfigFromAPIKeys(
		*awsAccount.AccessKeyID,
		*awsAccount.SecretAccessKey,
		"",
		*eksRuntimeInstance.Region,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config for EKS cluster token refresh: %w", err)
	}

	// get connection info from AWS
	eksClusterConn := connection.EKSClusterConnectionInfo{ClusterName: *eksRuntimeInstance.Name}
	if err := eksClusterConn.Get(awsConfig); err != nil {
		return nil, fmt.Errorf("failed to get EKS cluster connection info for token refresh: %w", err)
	}

	// update threeport API with new conncetion info
	runtimeInstance.CACertificate = &eksClusterConn.CACertificate
	runtimeInstance.ConnectionToken = &eksClusterConn.Token
	_, err = client.UpdateKubernetesRuntimeInstance(
		threeportAPIClient,
		threeportAPIEndpoint,
		runtimeInstance,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update kubernetes runtime instance kubernetes connection info: %w", err)
	}

	// generate rest config
	tlsConfig := rest.TLSClientConfig{
		CAData: []byte(eksClusterConn.CACertificate),
	}
	restConfig := rest.Config{
		Host:            eksClusterConn.APIEndpoint,
		BearerToken:     eksClusterConn.Token,
		TLSClientConfig: tlsConfig,
	}

	return &restConfig, nil
}
