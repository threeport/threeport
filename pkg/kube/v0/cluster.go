package v0

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	builder_config "github.com/nukleros/aws-builder/pkg/config"
	"github.com/nukleros/aws-builder/pkg/eks/connection"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
)

// GetInClusterKubeClient creates a kubernetes clientset for an in cluster configuration
func GetInClusterKubeClient() (*kubernetes.Clientset, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// GetKubeClientForGroupNameVersion creates a kubernetes rest client for a given group name/version
func GetKubeClientForGroupNameVersion(groupName string, groupVersion string) (*rest.RESTClient, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("could not retrieve in cluster config: %w", err)
	}

	config := *cfg
	config.ContentConfig.GroupVersion = &schema.GroupVersion{Group: groupName, Version: groupVersion}
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.NewCodecFactory(scheme.Scheme)
	config.UserAgent = rest.DefaultKubernetesUserAgent()
	restClient, err := rest.UnversionedRESTClientFor(&config)
	if err != nil {
		return nil, fmt.Errorf("could not create kube rest client: %w", err)
	}

	return restClient, nil
}

// GetClient creates a dynamic client interface and rest mapper from a
// kubernetes cluster instance.
func GetClient(
	runtime *v0.KubernetesRuntimeInstance,
	threeportControlPlane bool,
	threeportAPIClient *http.Client,
	threeportAPIEndpoint string,
	encryptionKey string,
) (dynamic.Interface, *meta.RESTMapper, error) {
	restConfig, err := GetRestConfig(
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

	// the rest mapper allows us to determine resource types
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
	restConfig, err := GetRestConfig(
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

// GetRestConfig takes a kubernetes runtime instance and returns a REST config
// for the kubernetes API.
func GetRestConfig(
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
	case runtime.Certificate != nil && runtime.CertificateKey != nil:
		var keyData string
		if encryptionKey != "" {
			decryptedKey, err := encryption.Decrypt(encryptionKey, *runtime.CertificateKey)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt kubernetes runtime instance key: %w", err)
			}
			keyData = decryptedKey
		} else {
			keyData = *runtime.CertificateKey
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
	case runtime.ConnectionToken != nil:
		tlsConfig := rest.TLSClientConfig{
			CAData: []byte(*runtime.CACertificate),
		}
		var bearerToken string
		if encryptionKey != "" {
			token, err := encryption.Decrypt(encryptionKey, *runtime.ConnectionToken)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt kubernetes runtime instance connection token: %w", err)
			}
			bearerToken = token
		} else {
			bearerToken = *runtime.ConnectionToken
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
		return nil, fmt.Errorf("failed to get AWS account by ID %d: %w", *eksRuntimeDefinition.AwsAccountID, err)
	}

	awsConfig, err := GetAwsConfigFromAwsAccount(encryptionKey, *eksRuntimeInstance.Region, awsAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config for EKS cluster token refresh: %w", err)
	}

	// get connection info from AWS
	eksClusterConn := connection.EksClusterConnectionInfo{ClusterName: *eksRuntimeInstance.Name}
	if err := eksClusterConn.Get(awsConfig); err != nil {
		return nil, fmt.Errorf("failed to get EKS cluster connection info for token refresh: %w", err)
	}

	// generate updated rest config
	tlsConfig := rest.TLSClientConfig{
		CAData: []byte(eksClusterConn.CACertificate),
	}
	restConfig := rest.Config{
		Host:            eksClusterConn.APIEndpoint,
		BearerToken:     eksClusterConn.Token,
		TLSClientConfig: tlsConfig,
	}

	// update threeport API with new connection info
	runtimeInstance.CACertificate = &eksClusterConn.CACertificate
	runtimeInstance.ConnectionToken = &eksClusterConn.Token
	runtimeInstance.ConnectionTokenExpiration = &eksClusterConn.TokenExpiration
	_, err = client.UpdateKubernetesRuntimeInstance(
		threeportAPIClient,
		threeportAPIEndpoint,
		runtimeInstance,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update kubernetes runtime instance kubernetes connection info: %w", err)
	}

	return &restConfig, nil
}

// GetAwsConfigFromAwsAccount returns an aws config from an aws account.
func GetAwsConfigFromAwsAccount(encryptionKey, region string, awsAccount *v0.AwsAccount) (*aws.Config, error) {
	accessKeyId := ""
	secretAccessKey := ""

	// if API keys are provided, decrypt and return aws config
	if awsAccount.AccessKeyID != nil && awsAccount.SecretAccessKey != nil {
		// decrypt access key id and secret access key
		aki, err := encryption.Decrypt(encryptionKey, *awsAccount.AccessKeyID)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt access key id: %w", err)
		}
		sak, err := encryption.Decrypt(encryptionKey, *awsAccount.SecretAccessKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt secret access key: %w", err)
		}
		accessKeyId = aki
		secretAccessKey = sak
	}

	// load aws config via API key credentials
	awsConfig, err := builder_config.LoadAWSConfigFromAPIKeys(accessKeyId, secretAccessKey, "", region, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	// get caller identity
	svc := sts.NewFromConfig(*awsConfig)
	callerIdentity, err := svc.GetCallerIdentity(
		context.Background(),
		&sts.GetCallerIdentityInput{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %w", err)
	}

	// if caller identity is an assumed role in the current AWS account,
	// return the default aws config. This will always be the case when
	// this function is called within a control plane hosted in EKS, as the
	// pod will be authenticated via IRSA to an IAM role.
	// https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html
	if strings.Contains(*callerIdentity.Arn, "assumed-role") &&
		*callerIdentity.Account == *awsAccount.AccountID {
		return awsConfig, nil
	}

	roleArn := ""
	externalId := ""

	// if a role arn is provided, use it
	if awsAccount.RoleArn != nil {
		roleArn = *awsAccount.RoleArn

		// if an external ID is provided with role arn, use it
		if awsAccount.ExternalId != nil {
			externalId = *awsAccount.ExternalId
		}
	}

	// construct aws config given values
	awsConfig, err = builder_config.LoadAWSConfigFromAPIKeys(
		accessKeyId,
		secretAccessKey,
		"",
		region,
		roleArn,
		"",
		externalId,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	return awsConfig, nil
}
