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
	"github.com/oracle/oci-go-sdk/v65/common"
	"golang.org/x/oauth2/google"
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
	util "github.com/threeport/threeport/pkg/util/v0"
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

	// build the discovery client from the rest config already computed above.
	// GetDiscoveryClient would call GetRestConfig a second time on the same runtime
	// pointer; if a token refresh occurred above, that pointer now holds the raw
	// (unencrypted) token and a second Decrypt call would fail.
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
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
	if runtime == nil || runtime.APIEndpoint == nil {
		return nil, errors.New("cannot get REST config without API endpoint")
	}

	// determine if the client is for a control plane component calling the
	// local kube API and set endpoint as needed
	kubeAPIEndpoint := *runtime.APIEndpoint
	if *runtime.ThreeportControlPlaneHost && threeportControlPlane {
		kubeAPIEndpoint = "kubernetes.default.svc.cluster.local"
	}

	// OKE and GKE authenticate with a token minted per request rather than a
	// single bearer token persisted on the runtime instance record.  This
	// ensures each caller authenticates as its own identity: OKE tokens are
	// cheap local RSA signatures, and on GKE every controller pod has a distinct
	// Workload Identity principal, so a shared/persisted token would cause
	// callers to authenticate as whichever pod last refreshed it — leading to
	// intermittent, identity-dependent RBAC failures.
	if runtime.KubernetesRuntimeDefinitionID != nil {
		definition, err := client.GetKubernetesRuntimeDefinitionByID(
			threeportAPIClient,
			threeportAPIEndpoint,
			*runtime.KubernetesRuntimeDefinitionID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get kubernetes runtime definition: %w", err)
		}

		var tokenGenerator func() (string, error)
		switch *definition.InfraProvider {
		case v0.KubernetesRuntimeInfraProviderOKE:
			tokenGenerator, err = buildOKETokenGenerator(runtime, threeportAPIClient, threeportAPIEndpoint, encryptionKey)
			if err != nil {
				return nil, fmt.Errorf("failed to build OKE token generator: %w", err)
			}
		case v0.KubernetesRuntimeInfraProviderGKE:
			tokenGenerator, err = buildGKETokenGenerator()
			if err != nil {
				return nil, fmt.Errorf("failed to build GKE token generator: %w", err)
			}
		}

		if tokenGenerator != nil {
			return &rest.Config{
				Host: kubeAPIEndpoint,
				TLSClientConfig: rest.TLSClientConfig{
					CAData: []byte(*runtime.CACertificate),
				},
				WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
					return &tokenRefreshTransport{base: rt, tokenGenerator: tokenGenerator}
				},
			}, nil
		}
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

				// check if KRD is set
				if runtime.KubernetesRuntimeDefinitionID == nil {
					return nil, errors.New("kubernetes runtime definition ID is not set, refresh token from config if bootstrapping a control plane")
				}

				// get kubernetes runtime definition
				definition, err := client.GetKubernetesRuntimeDefinitionByID(
					threeportAPIClient,
					threeportAPIEndpoint,
					*runtime.KubernetesRuntimeDefinitionID,
				)
				if err != nil {
					return nil, fmt.Errorf("failed to get kubernetes runtime definition by ID %d: %w", runtime.KubernetesRuntimeDefinitionID, err)
				}

				var config *rest.Config
				switch *definition.InfraProvider {
				case v0.KubernetesRuntimeInfraProviderEKS:
					if config, err = refreshEKSConnection(
						runtime,
						threeportAPIClient,
						threeportAPIEndpoint,
						encryptionKey,
					); err != nil {
						return nil, fmt.Errorf("failed to refresh connection token for EKS cluster: %w", err)
					}
					restConfig = *config
				default:
					return nil, fmt.Errorf("unable to refresh connection token for unsupported infra provider %s:", *definition.InfraProvider)
				}
			}
		}
	default:
		return nil, errors.New("did not find certificate, key pair or connection token - have no way to authenticate to kubernetes API")
	}

	return &restConfig, nil
}

// tokenRefreshTransport wraps an http.RoundTripper to inject a fresh bearer
// token on every outgoing request. Used during bootstrap when the threeport API
// is not yet available for the normal token refresh path.
type tokenRefreshTransport struct {
	base           http.RoundTripper
	tokenGenerator func() (string, error)
}

// RoundTrip generates a fresh token and sets the Authorization header before
// delegating to the underlying transport.
func (t *tokenRefreshTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.tokenGenerator()
	if err != nil {
		return nil, fmt.Errorf("failed to generate fresh token: %w", err)
	}
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(req)
}

// buildOKETokenGenerator fetches OCI credentials from the threeport API and
// returns a closure that mints a fresh OKE bearer token on each call via
// util.GenerateOkeToken.
func buildOKETokenGenerator(
	runtime *v0.KubernetesRuntimeInstance,
	threeportAPIClient *http.Client,
	threeportAPIEndpoint string,
	encryptionKey string,
) (func() (string, error), error) {
	okeRuntimeInstance, err := client.GetOciOkeKubernetesRuntimeInstanceByK8sRuntimeInst(
		threeportAPIClient,
		threeportAPIEndpoint,
		*runtime.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get OCI OKE kubernetes runtime instance: %w", err)
	}
	ociProvider, err := client.GetOciProviderByID(
		threeportAPIClient,
		threeportAPIEndpoint,
		*okeRuntimeInstance.OciProviderID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get OCI provider: %w", err)
	}
	decryptedPrivateKey, err := encryption.Decrypt(encryptionKey, *ociProvider.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt OCI provider private key: %w", err)
	}
	configProvider := common.NewRawConfigurationProvider(
		*ociProvider.TenancyOCID,
		*ociProvider.UserOCID,
		*ociProvider.DefaultRegion,
		*ociProvider.KeyFingerprint,
		decryptedPrivateKey,
		nil,
	)
	clusterOCID := *okeRuntimeInstance.ClusterOCID
	return func() (string, error) {
		token, _, err := util.GenerateOkeToken(clusterOCID, configProvider)
		return token, err
	}, nil
}

// GetClientWithTokenRefresh creates a dynamic client interface and rest mapper
// that automatically refreshes the bearer token on every request. This is used
// during bootstrap when the threeport API is not yet available to support the
// normal token refresh flow in GetRestConfig.
func GetClientWithTokenRefresh(
	runtime *v0.KubernetesRuntimeInstance,
	tokenGenerator func() (string, error),
) (dynamic.Interface, *meta.RESTMapper, error) {
	if runtime.APIEndpoint == nil {
		return nil, nil, errors.New("cannot create client without API endpoint")
	}

	// build rest config directly — skip GetRestConfig to avoid the API-based
	// token refresh path which requires a threeport API client not available
	// during bootstrap
	baseConfig := &rest.Config{
		Host: *runtime.APIEndpoint,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: []byte(*runtime.CACertificate),
		},
		WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
			return &tokenRefreshTransport{
				base:           rt,
				tokenGenerator: tokenGenerator,
			}
		},
	}

	// create dynamic client
	dynamicKubeClient, err := dynamic.NewForConfig(baseConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create dynamic kube client: %w", err)
	}

	// create discovery client from a copy (NewForConfig mutates the config)
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(rest.CopyConfig(baseConfig))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create new discovery client from rest config: %w", err)
	}

	// the rest mapper allows us to determine resource types
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get kube API group resources: %w", err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	return dynamicKubeClient, &mapper, nil
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

	// get AWS provider
	awsProvider, err := client.GetAwsProviderByID(
		threeportAPIClient,
		threeportAPIEndpoint,
		*eksRuntimeInstance.AwsProviderID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS provider by ID %d: %w", *eksRuntimeInstance.AwsProviderID, err)
	}

	awsConfig, err := GetAwsConfigFromAwsProvider(encryptionKey, *eksRuntimeInstance.Region, awsProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config for EKS cluster token refresh: %w", err)
	}

	// get connection info from AWS
	eksClusterConn := connection.EksClusterConnectionInfo{ClusterName: *eksRuntimeInstance.Name}
	if err := eksClusterConn.Get(awsConfig); err != nil {
		return nil, fmt.Errorf("failed to get EKS cluster connection info for token refresh: %w", err)
	}

	// generate updated rest config
	restConfig := rest.Config{
		Host:        eksClusterConn.APIEndpoint,
		BearerToken: eksClusterConn.Token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: []byte(eksClusterConn.CACertificate),
		},
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

// buildGKETokenGenerator returns a closure that mints a fresh GKE bearer token
// on each call using the caller's Google Application Default Credentials.  The
// underlying token source caches and refreshes the token internally, so
// per-request calls are cheap and only contact the metadata server when the
// cached token is near expiry.
//
// Minting the token from the caller's own ADC — rather than reading a single
// bearer token persisted on the runtime instance record — ensures each
// controller authenticates to the GKE API as its own Workload Identity
// principal.  A shared, persisted token would take on the identity of whichever
// controller last refreshed it, causing intermittent RBAC failures for
// controllers whose grants differ from that identity's.  It also removes the
// write-back to the shared runtime instance row, eliminating a source of
// transaction contention.
//
// TODO(#470): this relies on ambient Google ADC, so it only works from a
// GKE-hosted control plane.  To support a non-GKE-hosted control plane managing
// a GKE cluster, fall back to google.CredentialsFromJSON using the runtime's
// GcpProvider.ServiceAccountCredentials (as EKS/OKE do with their stored
// provider credentials).
func buildGKETokenGenerator() (func() (string, error), error) {
	tokenSource, err := google.DefaultTokenSource(
		context.Background(),
		"https://www.googleapis.com/auth/cloud-platform",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get Google token source: %w", err)
	}
	return func() (string, error) {
		token, err := tokenSource.Token()
		if err != nil {
			return "", fmt.Errorf("failed to get access token from Google: %w", err)
		}
		return token.AccessToken, nil
	}, nil
}

// GetAwsConfigFromAwsProvider returns an aws config from an aws account.
func GetAwsConfigFromAwsProvider(encryptionKey, region string, awsProvider *v0.AwsProvider) (*aws.Config, error) {
	accessKeyId := ""
	secretAccessKey := ""

	// if API keys are provided, decrypt and return aws config
	if awsProvider.AccessKeyID != nil && awsProvider.SecretAccessKey != nil {
		// decrypt access key id and secret access key
		aki, err := encryption.Decrypt(encryptionKey, *awsProvider.AccessKeyID)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt access key id: %w", err)
		}
		sak, err := encryption.Decrypt(encryptionKey, *awsProvider.SecretAccessKey)
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
		*callerIdentity.Account == *awsProvider.AccountID {
		return awsConfig, nil
	}

	roleArn := ""
	externalId := ""

	// if a role arn is provided, use it
	if awsProvider.RoleArn != nil {
		roleArn = *awsProvider.RoleArn

		// if an external ID is provided with role arn, use it
		if awsProvider.ExternalId != nil {
			externalId = *awsProvider.ExternalId
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
