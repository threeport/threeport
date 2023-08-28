package kube

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nukleros/eks-cluster/pkg/connection"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
)

// GetClient creates a dynamic client interface and rest mapper from a
// kubernetes cluster instance.
func GetClient(
	runtime *v0.KubernetesRuntimeInstance,
	threeportControlPlane bool,
	threeportAPIClient *http.Client,
	threeportAPIEndpoint string,
	encryptionKey string,
) (dynamic.Interface, *meta.RESTMapper, error) {
	restConfig, err := getRESTConfig(
		runtime,
		threeportControlPlane,
		threeportAPIClient,
		threeportAPIEndpoint,
		encryptionKey,
	)
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
		encryptionKey,
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
	encryptionKey string,
) (*discovery.DiscoveryClient, error) {
	restConfig, err := getRESTConfig(
		runtime,
		threeportControlPlane,
		threeportAPIClient,
		threeportAPIEndpoint,
		encryptionKey,
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

// getRESTConfig takes a kubernetes runtime instance and returns a REST config
// for the kubernetes API.
func getRESTConfig(
	runtime *v0.KubernetesRuntimeInstance,
	threeportControlPlane bool,
	threeportAPIClient *http.Client,
	threeportAPIEndpoint string,
	encryptionKey string,
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
	case runtime.Certificate != nil && runtime.EncryptedKey != nil:
		var keyData string
		if encryptionKey != "" {
			decryptedKey, err := encryption.Decrypt(encryptionKey, *runtime.EncryptedKey)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt kubernetes runtime instance key: %w", err)
			}
			keyData = decryptedKey
		} else {
			keyData = *runtime.EncryptedKey
		}
		tlsConfig := rest.TLSClientConfig{
			CertData: []byte(*runtime.Certificate),
			KeyData:  []byte(keyData),
			CAData:   []byte(*runtime.CACertificate),
		}
		restConfig = rest.Config{
			Host:            kubeAPIEndpoint,
			TLSClientConfig: tlsConfig,
		}
	case runtime.EncryptedConnectionToken != nil:
		tlsConfig := rest.TLSClientConfig{
			CAData: []byte(*runtime.CACertificate),
		}
		var bearerToken string
		if encryptionKey != "" {
			token, err := encryption.Decrypt(encryptionKey, *runtime.EncryptedConnectionToken)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt kubernetes runtime instance connection token: %w", err)
			}
			bearerToken = token
		} else {
			bearerToken = *runtime.EncryptedConnectionToken
		}
		restConfig = rest.Config{
			Host:            kubeAPIEndpoint,
			BearerToken:     bearerToken,
			TLSClientConfig: tlsConfig,
		}
		// if there is a connection token expiration, make sure that token is
		// not expired nor will it expire within 3 minutes
		if runtime.ConnectionTokenExpiration != nil {
			expiring, err := checkTokenExpiring(runtime)
			if err != nil {
				return nil, fmt.Errorf("failed to check connection token expiration: %w", err)
			}

			// if it is expired, or will within 3 minutes, get a new token
			if expiring {
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
						encryptionKey,
					)
					if err != nil {
						return nil, fmt.Errorf("failed to refresh connection token for EKS cluster: %w", err)
					}
					restConfig = *config
				default:
					return nil, errors.New(
						fmt.Sprintf("unable to refresh connection token for unsupported infra provider %s:", *definition.InfraProvider),
					)
				}
			}
		}
	default:
		return nil, errors.New("did not find certificate, key pair or connection token - have no way to authenticate to kubernetes API")
	}

	return &restConfig, nil
}

// checkTokenExpiring checks the expiration datetime for a token.  It returns
// true if it is expired or will expire within 3 minutes.
func checkTokenExpiring(
	runtimeInstance *v0.KubernetesRuntimeInstance,
) (bool, error) {
	if runtimeInstance.ConnectionTokenExpiration == nil {
		return true, errors.New("runtime instance has no token expiration value set")
	}

	expiration := time.Now().Add(time.Minute * 3)
	expiring := runtimeInstance.ConnectionTokenExpiration.Before(expiration)

	return expiring, nil
}

// refreshEKSConnection retrieves a new EKS token when it expires.
func refreshEKSConnection(
	runtimeInstance *v0.KubernetesRuntimeInstance,
	threeportAPIClient *http.Client,
	threeportAPIEndpoint string,
	encryptionKey string,
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

	// decrypt access key id and secret access key
	accessKeyID, err := encryption.Decrypt(encryptionKey, *awsAccount.EncryptedAccessKeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt access key id: %w", err)
	}
	secretAccessKey, err := encryption.Decrypt(encryptionKey, *awsAccount.EncryptedSecretAccessKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret access key: %w", err)
	}

	// create AWS config to get new token
	awsConfig, err := resource.LoadAWSConfigFromAPIKeys(
		accessKeyID,
		secretAccessKey,
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

	// encrypt connection token
	encryptedConnectionToken, err := encryption.Encrypt(encryptionKey, eksClusterConn.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt EKS cluster connection token: %w", err)
	}

	// update threeport API with new connection info
	runtimeInstance.CACertificate = &eksClusterConn.CACertificate
	runtimeInstance.EncryptedConnectionToken = &encryptedConnectionToken
	runtimeInstance.ConnectionTokenExpiration = &eksClusterConn.TokenExpiration
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
