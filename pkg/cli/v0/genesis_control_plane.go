package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/nukleros/aws-builder/pkg/eks"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	util "github.com/threeport/threeport/pkg/util/v0"
)

var ErrThreeportConfigAlreadyExists = errors.New("threeport config already contains deployed control planes")

// GenesisControlPlaneCLIArgs is the set of control plane arguments passed to one of
// the CLI tools.
type GenesisControlPlaneCLIArgs struct {
	AuthEnabled           bool
	AwsConfigProfile      string
	AwsConfigEnv          bool
	AwsRegion             string
	OciRegion             string
	OciConfigProfile      string
	GcpProjectId          string
	GcpRegion             string
	CfgFile               string
	ControlPlaneImageRepo string
	ControlPlaneImageTag  string
	CreateRootDomain      string
	CreateAdminEmail      string
	DevEnvironment        bool
	ForceOverwriteConfig  bool
	ControlPlaneName      string
	InfraProvider         string
	KubeconfigPath        string
	NumWorkerNodes        int
	ProviderConfigDir     string
	ThreeportPath         string
	Debug                 bool
	Verbose               bool
	TeardownOnFailure     bool
	ControlPlaneOnly      bool
	ClusterName           string
	InfraOnly             bool
	KindPortMappings      []string
	LocalRegistry         bool
}

// Uninstaller contains the necessary information to uninstall a control plane
// via its cleanOnCreate method.
type Uninstaller struct {
	createErrMsg           string
	createErr              error
	controlPlane           *threeport.ControlPlane
	kubernetesRuntimeInfra provider.KubernetesRuntimeInfra
	dynamicKubeClient      *dynamic.Interface
	mapper                 *meta.RESTMapper
	cleanConfig            *bool
	cpi                    *threeport.ControlPlaneInstaller
	awsConfig              *aws.Config
	teardownOnFailure      *bool
}

const tier = threeport.ControlPlaneTierDev

// InitArgs sets the default provider config directory, kubeconfig path and path
// to threeport repo as needed in the CLI arguments.
func InitArgs(args *GenesisControlPlaneCLIArgs) {
	// provider config dir
	if args.ProviderConfigDir == "" {
		providerConf, err := DefaultProviderConfigDir()
		if err != nil {
			Error("failed to set infra provider config directory", err)
			os.Exit(1)
		}
		args.ProviderConfigDir = providerConf
	}

	// fall back to client-go's standard kubeconfig precedence
	// ($KUBECONFIG, then ~/.kube/config) when --kubeconfig isn't supplied
	if args.KubeconfigPath == "" {
		args.KubeconfigPath = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}

	// set default threeport repo path if not provided
	// this is needed to map the container path to the host path for live
	// reloads of the code
	if args.ThreeportPath == "" {
		tp, err := os.Getwd()
		if err != nil {
			Error("failed to get current working directory", err)
			os.Exit(1)
		}
		args.ThreeportPath = tp
	}
}

// GetControlPlaneEnvVars updates cli args based on env vars
func (cliArgs *GenesisControlPlaneCLIArgs) GetControlPlaneEnvVars() {
	// get control plane image repo and tag from env vars
	controlPlaneImageRepo := os.Getenv("CONTROL_PLANE_IMAGE_REPO")
	controlPlaneImageTag := os.Getenv("CONTROL_PLANE_IMAGE_TAG")

	// configure control plane image repo via env var if not provided by cli
	if cliArgs.ControlPlaneImageRepo == "" && controlPlaneImageRepo != "" {
		cliArgs.ControlPlaneImageRepo = controlPlaneImageRepo
	}

	// configure control plane image tag via env var if not provided by cli
	if cliArgs.ControlPlaneImageTag == "" && controlPlaneImageTag != "" {
		cliArgs.ControlPlaneImageTag = controlPlaneImageTag
	}
}

func (a *GenesisControlPlaneCLIArgs) CreateInstaller() (*threeport.ControlPlaneInstaller, error) {
	cpi := threeport.NewInstaller()

	if a.ControlPlaneImageRepo != "" {
		cpi.SetAllImageRepo(a.ControlPlaneImageRepo)
	}

	if a.ControlPlaneImageTag != "" {
		cpi.SetAllImageTags(a.ControlPlaneImageTag)
	}

	cpi.Opts.AuthEnabled = a.AuthEnabled
	cpi.Opts.AwsConfigProfile = a.AwsConfigProfile
	cpi.Opts.AwsConfigEnv = a.AwsConfigEnv
	cpi.Opts.AwsRegion = a.AwsRegion
	cpi.Opts.OciRegion = a.OciRegion
	cpi.Opts.OciConfigProfile = a.OciConfigProfile
	cpi.Opts.GcpProjectId = a.GcpProjectId
	cpi.Opts.GcpRegion = a.GcpRegion
	cpi.Opts.CfgFile = a.CfgFile
	cpi.Opts.CreateRootDomain = a.CreateRootDomain
	cpi.Opts.CreateAdminEmail = a.CreateAdminEmail
	cpi.Opts.DevEnvironment = a.DevEnvironment
	cpi.Opts.ForceOverwriteConfig = a.ForceOverwriteConfig
	cpi.Opts.ControlPlaneName = a.ControlPlaneName
	cpi.Opts.InfraProvider = a.InfraProvider
	cpi.Opts.KubeconfigPath = a.KubeconfigPath
	cpi.Opts.NumWorkerNodes = a.NumWorkerNodes
	cpi.Opts.ProviderConfigDir = a.ProviderConfigDir
	cpi.Opts.ThreeportPath = a.ThreeportPath
	cpi.Opts.Debug = a.Debug
	cpi.Opts.Verbose = a.Verbose
	cpi.Opts.CreateOrUpdateKubeResources = false
	cpi.Opts.ControlPlaneOnly = a.ControlPlaneOnly
	cpi.Opts.ClusterName = a.ClusterName
	cpi.Opts.InfraOnly = a.InfraOnly
	cpi.Opts.RestApiLoadBalancer = true
	cpi.Opts.TeardownOnFailure = a.TeardownOnFailure
	cpi.Opts.LocalRegistry = a.LocalRegistry
	cpi.Opts.KindPortMappings = a.KindPortMappings

	return cpi, nil
}

// CreateGenesisControlPlane uses the CLI arguments to create a new threeport control
// plane.
func CreateGenesisControlPlane(customInstaller *threeport.ControlPlaneInstaller) error {
	// get the threeport config
	threeportConfig, _, err := GetThreeportConfig("")
	if err != nil {
		return fmt.Errorf("failed to get threeport config: %w", err)
	}

	// configure installer
	cpi := customInstaller
	if customInstaller == nil {
		cpi = threeport.NewInstaller()
	}

	// emit warning if auth is disabled
	if !cpi.Opts.AuthEnabled {
		Warning("Auth and HTTPS are disabled. Commands will be sent over HTTP. Use --auth-enabled=true to enable auth and HTTPS.")
	}

	// configure uninstaller
	uninstaller := &Uninstaller{
		cpi:               cpi,
		teardownOnFailure: &cpi.Opts.TeardownOnFailure,
		cleanConfig:       util.Ptr(true),
	}

	// check threeport config to see if it is empty
	threeportInstanceConfigEmpty := threeportConfig.CheckThreeportConfigEmpty()

	var threeportControlPlaneConfig *ControlPlane
	genesis := true

	if cpi.Opts.ControlPlaneOnly {
		// load the matching entry if one exists from a prior run;
		// otherwise fall through to a new entry, which is the case when
		// deploying to infrastructure that was provisioned outside of
		// tptctl.
		if existingConfig, err := threeportConfig.GetControlPlaneConfig(cpi.Opts.ControlPlaneName); err == nil {
			threeportControlPlaneConfig = existingConfig
		} else {
			threeportControlPlaneConfig = &ControlPlane{}
		}
	} else {
		// for fresh installs, check if config already exists
		if !threeportInstanceConfigEmpty && !cpi.Opts.ForceOverwriteConfig {
			return ErrThreeportConfigAlreadyExists
		}
		// reset config and create fresh control plane config
		threeportConfig.ControlPlanes = []ControlPlane{}
		threeportControlPlaneConfig = &ControlPlane{}
	}

	// create local registry if requested
	if cpi.Opts.LocalRegistry {
		if err := tptdev.CreateLocalRegistry(); err != nil {
			return fmt.Errorf("failed to create local container registry: %w", err)
		}
	}

	// create threeport config for new instance
	if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *ControlPlane) {
		c.Name = cpi.Opts.ControlPlaneName
		c.Provider = cpi.Opts.InfraProvider
		c.Genesis = genesis
	}); err != nil {
		return fmt.Errorf("failed to update threeport config: %w", err)
	}

	// configure the control plane
	controlPlane := threeport.ControlPlane{
		InfraProvider: v0.KubernetesRuntimeInfraProvider(cpi.Opts.InfraProvider),
		Tier:          tier,
	}
	uninstaller.controlPlane = &controlPlane

	var kubernetesRuntimeInfra provider.KubernetesRuntimeInfra
	var tokenGenerator func() (string, error)
	var threeportAPIEndpoint string
	var callerIdentity *sts.GetCallerIdentityOutput
	kubeConnectionInfo := &kube.KubeConnectionInfo{}
	awsConfigUser := aws.Config{}
	uninstaller.awsConfig = &awsConfigUser
	awsConfigResourceManager := &aws.Config{}
	switch controlPlane.InfraProvider {
	// deploy infrastructure
	case v0.KubernetesRuntimeInfraProviderKind:
		if err := DeployKindInfra(
			cpi,
			threeportControlPlaneConfig,
			threeportConfig,
			&kubernetesRuntimeInfra,
			kubeConnectionInfo,
			uninstaller,
		); err != nil {
			return fmt.Errorf("failed to deploy kind infrastructure: %w", err)
		}
	case v0.KubernetesRuntimeInfraProviderEKS:
		if err := DeployEksInfra(
			cpi,
			threeportControlPlaneConfig,
			threeportConfig,
			&kubernetesRuntimeInfra,
			kubeConnectionInfo,
			uninstaller,
			&awsConfigUser,
			callerIdentity,
			awsConfigResourceManager,
		); err != nil {
			return fmt.Errorf("failed to deploy eks infrastructure: %w", err)
		}
	case v0.KubernetesRuntimeInfraProviderOKE:
		if err := DeployOkeInfra(
			cpi,
			threeportControlPlaneConfig,
			threeportConfig,
			&kubernetesRuntimeInfra,
			kubeConnectionInfo,
			uninstaller,
		); err != nil {
			return fmt.Errorf("failed to deploy oke infrastructure: %w", err)
		}
	case v0.KubernetesRuntimeInfraProviderGKE:
		kubernetesRuntimeInfraGKE := provider.KubernetesRuntimeInfraGKE{
			PulumiWorkspace: provider.PulumiWorkspace{
				RuntimeInstanceName: runtimeInstanceName(cpi.Opts),
			},
			Version:                kube.KubernetesDefaultVersion,
			WorkerNodeInitialCount: int32(2),
			ProjectID:              cpi.Opts.GcpProjectId,
			Region:                 cpi.Opts.GcpRegion,
		}
		kubernetesRuntimeInfra = &kubernetesRuntimeInfraGKE
		uninstaller.kubernetesRuntimeInfra = &kubernetesRuntimeInfraGKE

		if cpi.Opts.ControlPlaneOnly {
			kubeConnectionInfo, err = kubernetesRuntimeInfraGKE.GetConnection()
			if err != nil {
				return fmt.Errorf("failed to get connection info for GKE kubernetes runtime: %w", err)
			}
		} else {
			kubeConnectionInfo, err = kubernetesRuntimeInfra.Create()
			if err != nil {
				return uninstaller.cleanOnCreateError("failed to create control plane infra for threeport", err)
			}

			// pass the GCP service account email to the installer for Workload Identity configuration
			cpi.Opts.GcpServiceAccountEmail = kubernetesRuntimeInfraGKE.ServiceAccountEmail
		}
	}

	// update threeport config with kube API info
	if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *ControlPlane) {
		c.KubeAPI = KubeAPI{
			APIEndpoint:   kubeConnectionInfo.APIEndpoint,
			CACertificate: util.Base64Encode(kubeConnectionInfo.CACertificate),
			Certificate:   util.Base64Encode(kubeConnectionInfo.Certificate),
			Key:           util.Base64Encode(kubeConnectionInfo.Key),
			Token:         util.Base64Encode(kubeConnectionInfo.Token),
		}
	}); err != nil {
		return uninstaller.cleanOnCreateError("failed to update threeport config", err)
	}

	// if infra only, do not deploy control plane
	if cpi.Opts.InfraOnly {
		return nil
	}

	// generate encryption key
	encryptionKey, err := encryption.GenerateKey()
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to generate encryption key", err)
	}

	// update threeport config with encryption key
	if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *ControlPlane) {
		c.EncryptionKey = encryptionKey
	}); err != nil {
		return uninstaller.cleanOnCreateError("failed to update threeport config", err)
	}

	// the kubernetes runtime instance is the default compute space kubernetes runtime to be added
	// to the API
	kubernetesRuntimeInstName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
	controlPlaneHost := true
	defaultRuntime := true
	instReconciled := true // this instance exists already - we don't need the k8s runtime instance doing anything
	var kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance
	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderKind:
		location := "Local"
		kubernetesRuntimeInstance = &v0.KubernetesRuntimeInstance{
			Instance: v0.Instance{
				Name: &kubernetesRuntimeInstName,
			},
			Reconciliation: v0.Reconciliation{
				Reconciled: &instReconciled,
			},
			ThreeportControlPlaneHost: &controlPlaneHost,
			APIEndpoint:               &kubeConnectionInfo.APIEndpoint,
			CACertificate:             &kubeConnectionInfo.CACertificate,
			Certificate:               &kubeConnectionInfo.Certificate,
			CertificateKey:            &kubeConnectionInfo.Key,
			DefaultRuntime:            &defaultRuntime,
			Location:                  &location,
		}
	case v0.KubernetesRuntimeInfraProviderEKS:
		if err := ConfigureEksKubernetesRuntimeInstance(
			cpi,
			kubeConnectionInfo,
			uninstaller,
			&awsConfigUser,
			callerIdentity,
			awsConfigResourceManager,
			kubernetesRuntimeInstance,
			kubernetesRuntimeInstName,
			instReconciled,
			controlPlaneHost,
			defaultRuntime,
		); err != nil {
			return uninstaller.cleanOnCreateError("failed to configure eks kubernetes runtime instance", err)
		}
	case v0.KubernetesRuntimeInfraProviderOKE:
		kubernetesRuntimeInfraOKE := kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraOKE)

		// build token generator for automatic refresh during bootstrap
		okeClusterOCID, err := kubernetesRuntimeInfraOKE.GetClusterOCID(
			provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName),
		)
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to get OKE cluster OCID for token refresh", err)
		}
		okeConfigProvider := kubernetesRuntimeInfraOKE.ConfigProvider
		tokenGenerator = func() (string, error) {
			t, _, err := util.GenerateOkeToken(okeClusterOCID, okeConfigProvider)
			return t, err
		}

		location, err := mapping.GetLocationForOciRegion(kubernetesRuntimeInfraOKE.Region)
		if err != nil {
			return uninstaller.cleanOnCreateError(
				fmt.Sprintf("failed to get threeport location for OKE region %s", kubernetesRuntimeInfraOKE.Region),
				err,
			)
		}
		kubernetesRuntimeInstance = &v0.KubernetesRuntimeInstance{
			Instance: v0.Instance{
				Name: &kubernetesRuntimeInstName,
			},
			Reconciliation: v0.Reconciliation{
				Reconciled: &instReconciled,
			},
			ThreeportControlPlaneHost: &controlPlaneHost,
			APIEndpoint:               &kubeConnectionInfo.APIEndpoint,
			CACertificate:             &kubeConnectionInfo.CACertificate,
			ConnectionToken:           &kubeConnectionInfo.Token,
			ConnectionTokenExpiration: &kubeConnectionInfo.TokenExpiration,
			DefaultRuntime:            &defaultRuntime,
			Location:                  &location,
		}
	case v0.KubernetesRuntimeInfraProviderGKE:
		kubernetesRuntimeInfraGKE := kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraGKE)
		location, err := mapping.GetLocationForGcpRegion(kubernetesRuntimeInfraGKE.Region)
		if err != nil {
			return uninstaller.cleanOnCreateError(
				fmt.Sprintf("failed to get threeport location for GKE region %s", kubernetesRuntimeInfraGKE.Region),
				err,
			)
		}
		kubernetesRuntimeInstance = &v0.KubernetesRuntimeInstance{
			Instance: v0.Instance{
				Name: &kubernetesRuntimeInstName,
			},
			Reconciliation: v0.Reconciliation{
				Reconciled: &instReconciled,
			},
			ThreeportControlPlaneHost: &controlPlaneHost,
			APIEndpoint:               &kubeConnectionInfo.APIEndpoint,
			CACertificate:             &kubeConnectionInfo.CACertificate,
			ConnectionToken:           &kubeConnectionInfo.Token,
			ConnectionTokenExpiration: &kubeConnectionInfo.TokenExpiration,
			DefaultRuntime:            &defaultRuntime,
			Location:                  &location,
		}
	}

	// get kubernetes client and mapper for use with kube API
	// if a token generator is available (OKE), use it to automatically refresh
	// the bearer token on each request, avoiding expiration during bootstrap
	var dynamicKubeClient dynamic.Interface
	var mapper *meta.RESTMapper
	if tokenGenerator != nil {
		dynamicKubeClient, mapper, err = kube.GetClientWithTokenRefresh(
			kubernetesRuntimeInstance,
			tokenGenerator,
		)
	} else {
		dynamicKubeClient, mapper, err = kube.GetClient(
			kubernetesRuntimeInstance,
			false,
			nil,
			"",
			"",
		)
	}
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to get a Kubernetes client and mapper", err)
	}
	uninstaller.dynamicKubeClient = &dynamicKubeClient
	uninstaller.mapper = mapper

	// generate new DB client credentials
	dbCreds, err := auth.GenerateDbCreds(cpi.Opts.Namespace)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to generated DB client credentials", err)
	}

	// install the threeport control plane dependencies — deployed immediately
	// so NATS and CockroachDB start initializing while certs are generated
	if err := cpi.InstallThreeportControlPlaneDependencies(
		dynamicKubeClient,
		mapper,
		encryptionKey,
		dbCreds,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport control plane dependencies", err)
	}

	// if auth is enabled, generate client certificate and add to local config
	var authConfig *auth.AuthConfig
	var clientCredentials *Credential
	if cpi.Opts.AuthEnabled {
		// generate certificate authority
		Info("Generating certificate authority for threeport API")
		authConfig, err = auth.GetAuthConfig()
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to get auth config", err)
		}

		// generate client certificate
		Info("Generating client certificate")
		clientCertificate, clientPrivateKey, err := auth.GenerateCertificate(
			authConfig.CAConfig,
			&authConfig.CAPrivateKey,
			"threeport-owner",
			"",
			"",
		)
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to generate client certificate and private key", err)
		}

		clientCredentials = &Credential{
			Name:       cpi.Opts.ControlPlaneName,
			ClientCert: util.Base64Encode(clientCertificate),
			ClientKey:  util.Base64Encode(clientPrivateKey),
		}

		// update threeport config with auth info, replacing any existing
		// credential with the same name to avoid stale certs
		if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *ControlPlane) {
			c.AuthEnabled = true
			replaced := false
			for i, cred := range c.Credentials {
				if cred.Name == clientCredentials.Name {
					c.Credentials[i] = *clientCredentials
					replaced = true
					break
				}
			}
			if !replaced {
				c.Credentials = append(c.Credentials, *clientCredentials)
			}
			c.CACert = authConfig.CABase64Encoded
		}); err != nil {
			return uninstaller.cleanOnCreateError("failed to update threeport config", err)
		}
	} else {
		// update threeport config with auth info
		if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *ControlPlane) {
			c.AuthEnabled = false
		}); err != nil {
			return uninstaller.cleanOnCreateError("failed to update threeport config", err)
		}
	}

	// get threeport API client
	apiClient, err := threeportConfig.GetHTTPClient(cpi.Opts.ControlPlaneName)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to get threeport certificates from config", err)
	}

	err = cpi.Opts.PreInstallFunction(kubernetesRuntimeInstance, cpi)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to run custom preInstall function", err)
	}

	// for kind, the API endpoint is known upfront so we can install TLS
	// secrets before deploying the API server to avoid mount failures
	if controlPlane.InfraProvider == v0.KubernetesRuntimeInfraProviderKind {
		// update threeport config with api endpoint
		var err error
		threeportAPIEndpoint = threeport.GetLocalThreeportAPIEndpoint(cpi.Opts.AuthEnabled)
		if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *ControlPlane) {
			c.APIServer = threeportAPIEndpoint
		}); err != nil {
			return fmt.Errorf("failed to update threeport config: %w", err)
		}

		// install TLS assets before API deployment so secrets exist when
		// the pod mounts them
		if cpi.Opts.AuthEnabled {
			threeportApiAltNames := threeport.ThreeportApiAltNames(cpi.Opts.Namespace)
			threeportApiAltNames = append(threeportApiAltNames, threeportAPIEndpoint)

			Info("Generating server certificate and installing TLS assets")
			if err := cpi.InstallThreeportAPITLS(
				dynamicKubeClient,
				mapper,
				authConfig,
				threeportApiAltNames...,
			); err != nil {
				return uninstaller.cleanOnCreateError("failed to install threeport API TLS assets", err)
			}
		}
	}

	// install the API
	if err := cpi.UpdateThreeportAPIDeployment(
		dynamicKubeClient,
		mapper,
		dbCreds,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport API server", err)
	}

	// for non-kind providers, get the API endpoint from the load balancer
	// and install TLS after since the endpoint isn't known until deployment
	if controlPlane.InfraProvider != v0.KubernetesRuntimeInfraProviderKind {
		threeportAPIEndpoint, err = cpi.GetThreeportAPIEndpoint(dynamicKubeClient, *mapper)
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to get threeport API's public endpoint", err)
		}
		if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *ControlPlane) {
			c.APIServer = fmt.Sprintf("%s:%d", threeportAPIEndpoint, threeport.GetThreeportAPIPort(cpi.Opts.AuthEnabled))
		}); err != nil {
			return uninstaller.cleanOnCreateError("failed to update threeport config", err)
		}

		// install provider-specific kubernetes resources
		switch controlPlane.InfraProvider {
		case v0.KubernetesRuntimeInfraProviderEKS:
			if err := InstallEksKubernetesResources(
				cpi,
				uninstaller,
				callerIdentity,
				&dynamicKubeClient,
				mapper,
			); err != nil {
				return uninstaller.cleanOnCreateError("failed to install eks kubernetes resources", err)
			}
		}

		// install TLS assets after endpoint is known
		if cpi.Opts.AuthEnabled {
			threeportApiAltNames := threeport.ThreeportApiAltNames(cpi.Opts.Namespace)
			threeportApiAltNames = append(threeportApiAltNames, threeportAPIEndpoint)

			Info("Generating server certificate and installing TLS assets")
			if err := cpi.InstallThreeportAPITLS(
				dynamicKubeClient,
				mapper,
				authConfig,
				threeportApiAltNames...,
			); err != nil {
				return uninstaller.cleanOnCreateError("failed to install threeport API TLS assets", err)
			}
		}
	}

	if err := cpi.InstallThreeportControllers(
		dynamicKubeClient,
		mapper,
		authConfig,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport controllers", err)
	}

	if err = cpi.Opts.PostInstallFunction(kubernetesRuntimeInstance, cpi); err != nil {
		return uninstaller.cleanOnCreateError("failed to run custom postInstall function", err)
	}

	if err := cpi.InstallThreeportAgent(
		dynamicKubeClient,
		mapper,
		authConfig,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport agent", err)
	}

	// wait for API server to start running
	Info(fmt.Sprintf("Waiting for threeport API to start running at %s", threeportAPIEndpoint))
	attemptsMax := 60
	waitDurationSeconds := 5
	if err = util.Retry(attemptsMax, waitDurationSeconds, func() error {
		_, err := client_lib.GetResponse(
			apiClient,
			fmt.Sprintf("%s/version", threeportAPIEndpoint),
			http.MethodGet,
			new(bytes.Buffer),
			map[string]string{},
			http.StatusOK,
		)
		if err != nil {
			Info(fmt.Sprintf("Connection attempt result: %s", err))
			return fmt.Errorf("failed to get threeport API version: %w", err)
		}
		return nil
	}); err != nil {
		return uninstaller.cleanOnCreateError(
			fmt.Sprintf("timed out after %d seconds waiting for 200 response from threeport API", attemptsMax*waitDurationSeconds),
			err,
		)
	}
	Info("Threeport API is running")

	// install support services CRDs separately (package-level function)
	if err = threeport.InstallThreeportCRDs(dynamicKubeClient, mapper); err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport support services CRDs", err)
	}

	// create the default compute space kubernetes runtime definition in threeport API
	kubernetesRuntimeDefName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
	defReconciled := true // this definition for the bootstrap cluster does not require reconcilation
	kubernetesRuntimeDefinition := v0.KubernetesRuntimeDefinition{
		Definition: v0.Definition{
			Name: &kubernetesRuntimeDefName,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: &defReconciled,
		},
		InfraProvider: &cpi.Opts.InfraProvider,
	}
	kubernetesRuntimeDefResult, err := client.CreateKubernetesRuntimeDefinition(
		apiClient,
		threeportAPIEndpoint,
		&kubernetesRuntimeDefinition,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new kubernetes runtime definition for default compute space", err)
	}

	// create default compute space kubernetes runtime instance in threeport API
	kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID = kubernetesRuntimeDefResult.ID
	kubernetesRuntimeInstResult, err := client.CreateKubernetesRuntimeInstance(
		apiClient,
		threeportAPIEndpoint,
		kubernetesRuntimeInstance,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new kubernetes runtime instance for default compute space", err)
	}

	// configure control plane with provider-specific details by adding the
	// infra provider-specific objects to the Threeport API
	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		if err := ConfigureControlPlaneWithEksConfig(
			cpi,
			uninstaller,
			&awsConfigUser,
			callerIdentity,
			awsConfigResourceManager,
			apiClient,
			threeportAPIEndpoint,
			&kubernetesRuntimeInfra,
			kubernetesRuntimeDefResult,
			kubernetesRuntimeInstResult,
		); err != nil {
			return uninstaller.cleanOnCreateError("failed to add AWS objects to Threeport API", err)
		}
	case v0.KubernetesRuntimeInfraProviderOKE:
		if err := ConfigureControlPlaneWithOkeConfig(
			cpi,
			uninstaller,
			apiClient,
			threeportAPIEndpoint,
			kubernetesRuntimeDefResult,
			kubernetesRuntimeInstResult,
			&kubernetesRuntimeInfra,
		); err != nil {
			return uninstaller.cleanOnCreateError("failed to add OCI objects to Threeport API", err)
		}
	case v0.KubernetesRuntimeInfraProviderGKE:
		if err := ConfigureControlPlaneWithGkeConfig(
			cpi,
			uninstaller,
			apiClient,
			threeportAPIEndpoint,
			kubernetesRuntimeDefResult,
			kubernetesRuntimeInstResult,
			&kubernetesRuntimeInfra,
		); err != nil {
			return uninstaller.cleanOnCreateError("failed to add GCP objects to Threeport API", err)
		}
	}

	// wait for kube API to persist the change and refresh the client and mapper
	// this is necessary to have the updated REST mapping for the CRDs as the
	// support services operator install includes one of those custom resources
	// NOTE: creating the k8s runtime instances and definitions (above) must happen
	// before this step as kube.GetClient() may require a refresh of the connection
	// token, which depends on those objects existing in the Threeport API
	time.Sleep(time.Second * 10)
	if tokenGenerator != nil {
		dynamicKubeClient, mapper, err = kube.GetClientWithTokenRefresh(
			kubernetesRuntimeInstResult,
			tokenGenerator,
		)
	} else {
		dynamicKubeClient, mapper, err = kube.GetClient(
			kubernetesRuntimeInstResult,
			false,
			apiClient,
			threeportAPIEndpoint,
			encryptionKey,
		)
	}
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to refresh the Kubernetes client and mapper", err)
	}

	// install the support services operator
	err = threeport.InstallThreeportSupportServicesOperator(dynamicKubeClient, mapper)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport support services operator", err)
	}

	// install provider-specific system services
	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		if err := threeport.InstallEksThreeportSystemServices(
			dynamicKubeClient,
			mapper,
			cpi.Opts.Name+"-"+cpi.Opts.ControlPlaneName,
			*callerIdentity.Account,
		); err != nil {
			return uninstaller.cleanOnCreateError("failed to install system services", err)
		}
	}

	reconciled := true
	// create control plane definitons and instance for the newly create control plane
	controlPlaneDefinition := v0.ControlPlaneDefinition{
		Definition: v0.Definition{
			Name: &cpi.Opts.ControlPlaneName,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: &reconciled,
		},
		AuthEnabled: &cpi.Opts.AuthEnabled,
	}
	_, err = client.CreateControlPlaneDefinition(apiClient, threeportAPIEndpoint, &controlPlaneDefinition)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create control plane definition in threeport API", err)
	}

	selfInstance := true
	var caCert *string
	var clientCert *string
	var clientKey *string
	if cpi.Opts.AuthEnabled {
		caCert = &authConfig.CABase64Encoded
		clientCert = &clientCredentials.ClientCert
		clientKey = &clientCredentials.ClientKey
	} else {
		caCert = nil
		clientCert = nil
		clientKey = nil
	}

	componentList := cpi.Opts.ControllerList
	componentList = append(componentList, cpi.Opts.RestApiInfo)
	componentList = append(componentList, cpi.Opts.AgentInfo)

	// construct control plane instance object
	controlPlaneInstance := v0.ControlPlaneInstance{
		Instance: v0.Instance{
			Name: &cpi.Opts.ControlPlaneName,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: &reconciled,
		},
		Namespace:                   &cpi.Opts.Namespace,
		KubernetesRuntimeInstanceID: kubernetesRuntimeInstance.ID,
		Genesis:                     &genesis,
		IsSelf:                      &selfInstance,
		ApiServerEndpoint:           &threeportAPIEndpoint,
		CACert:                      caCert,
		ClientCert:                  clientCert,
		ClientKey:                   clientKey,
		CustomComponentInfo:         componentList,
		ControlPlaneDefinitionID:    controlPlaneDefinition.ID,
	}

	// create control plane instance
	_, err = client.CreateControlPlaneInstance(apiClient, threeportAPIEndpoint, &controlPlaneInstance)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create control plane instance in threeport API", err)
	}

	Info("Threeport control plane installed")
	Info("Threeport config updated")

	Complete(fmt.Sprintf("Threeport control plane %s created", cpi.Opts.ControlPlaneName))

	return nil
}

// DeleteGenesisControlPlane deletes a threeport control plane.
func DeleteGenesisControlPlane(customInstaller *threeport.ControlPlaneInstaller) error {
	// get threeport config
	threeportConfig, requestedControlPlane, err := GetThreeportConfig(customInstaller.Opts.ControlPlaneName)
	if err != nil {
		return fmt.Errorf("failed to get threeport config: %w", err)
	}

	genesis, err := threeportConfig.CheckThreeportGenesisControlPlane(requestedControlPlane)
	if err != nil {
		return fmt.Errorf("could not check for genesis info: %w", err)
	}

	if !genesis {
		return errors.New("could not delete current control plane because it is not a genesis control plane")
	}

	// configure installer
	cpi := customInstaller
	if customInstaller == nil {
		cpi = threeport.NewInstaller()
	}

	// get threeport control plane config
	threeportControlPlaneConfig, err := threeportConfig.GetControlPlaneConfig(requestedControlPlane)
	if err != nil {
		return fmt.Errorf("failed to get threeport control plane config: %w", err)
	}

	// get threeport certificates
	ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForControlPlane(cpi.Opts.ControlPlaneName)
	if err != nil {
		return fmt.Errorf("failed to get threeport certificates from config: %w", err)
	}

	// get http client
	apiClient, err := client_lib.GetHTTPClient(threeportControlPlaneConfig.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}

	// get the kubernetes runtime instance object
	var kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance
	if !cpi.Opts.InfraOnly {
		kubernetesRuntimeInstance, err = client.GetThreeportControlPlaneKubernetesRuntimeInstance(
			apiClient,
			threeportControlPlaneConfig.APIServer,
		)
		if err != nil {
			return fmt.Errorf("failed to retrieve kubernetes runtime instance from threeport API: %w", err)
		}
	}

	var kubernetesRuntimeInfra provider.KubernetesRuntimeInfra
	var awsConfigUser *aws.Config
	var awsConfigResourceManager *aws.Config
	var kubeConnection *kube.KubeConnectionInfo
	infraAlreadyDestroyed := false

	// perform provider-specific deletion prep
	switch threeportControlPlaneConfig.Provider {
	case v0.KubernetesRuntimeInfraProviderKind:
		kubernetesRuntimeInfraKind := provider.KubernetesRuntimeInfraKind{
			RuntimeInstanceName: provider.ThreeportRuntimeName(threeportControlPlaneConfig.Name),
			KubeconfigPath:      cpi.Opts.KubeconfigPath,
		}
		kubernetesRuntimeInfra = &kubernetesRuntimeInfraKind
	case v0.KubernetesRuntimeInfraProviderEKS:
		var kubernetesRuntimeInfraEKS *provider.KubernetesRuntimeInfraEKS
		if kubernetesRuntimeInfraEKS, err = PrepForEksDeletion(
			cpi,
			threeportControlPlaneConfig,
			threeportConfig,
			awsConfigUser,
			awsConfigResourceManager,
			requestedControlPlane,
		); err != nil {
			return fmt.Errorf("")
		}
		kubernetesRuntimeInfra = kubernetesRuntimeInfraEKS
	case v0.KubernetesRuntimeInfraProviderOKE:
		kubernetesRuntimeInfraOKE := provider.KubernetesRuntimeInfraOKE{
			PulumiWorkspace: provider.PulumiWorkspace{
				RuntimeInstanceName: provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName),
				ProjectName:         "oke",
				ProjectDescription:  "Oracle Kubernetes Engine (OKE) cluster for Threeport",
				// StackConfigs set by LoadOCIConfig after region is resolved
			},
		}
		if err := kubernetesRuntimeInfraOKE.LoadOCIConfig(
			threeportControlPlaneConfig.OKEProviderConfig.OciRegion,
			threeportControlPlaneConfig.OKEProviderConfig.OciConfigProfile,
			threeportControlPlaneConfig.OKEProviderConfig.OciCompartmentOcid,
		); err != nil {
			return fmt.Errorf("failed to load OCI config: %w", err)
		}
		kubernetesRuntimeInfra = &kubernetesRuntimeInfraOKE

		// check if infrastructure has already been destroyed (no local
		// Pulumi state dir). This happens when a previous teardown
		// destroyed infra but IAM cleanup failed.
		infraAlreadyDestroyed = !kubernetesRuntimeInfraOKE.HasStateDir()
		if infraAlreadyDestroyed {
			Info("Infrastructure already destroyed, skipping to IAM cleanup")
		}

		// pull OKE stack state from OKE runtime instance
		// if not infra-only, as this flag implies the user
		// does not want to depend on control plane state
		if !cpi.Opts.InfraOnly && !infraAlreadyDestroyed {
			if kubeConnection, err = kubernetesRuntimeInfraOKE.GetConnection(); err != nil {
				return fmt.Errorf("failed to get connection for OKE kubernetes runtime infra: %w", err)
			}

			// pull OKE stack state from OKE runtime instance
			// and save to local pulumi state directory
			okeRuntimeInstance, err := client.GetOciOkeKubernetesRuntimeInstanceByK8sRuntimeInst(
				apiClient,
				threeportControlPlaneConfig.APIServer,
				*kubernetesRuntimeInstance.ID,
			)
			if err != nil {
				return fmt.Errorf(
					"failed to get OCI OKE kubernetes runtime instance by kubernetes runtime instance ID %d: %w",
					*kubernetesRuntimeInstance.ID,
					err,
				)
			}

			// restore pulumi state from DB — this is the only source of
			// state, so failure here means destroy would be a no-op and
			// leave orphaned cloud resources
			if okeRuntimeInstance.ResourceInventory == nil || len(*okeRuntimeInstance.ResourceInventory) == 0 {
				return fmt.Errorf("no resource inventory found for OKE runtime instance %d, cannot destroy infrastructure", *okeRuntimeInstance.ID)
			}
			if !json.Valid(*okeRuntimeInstance.ResourceInventory) {
				return fmt.Errorf("stored resource inventory for OKE runtime instance %d is corrupt, cannot destroy infrastructure", *okeRuntimeInstance.ID)
			}
			if err := kubernetesRuntimeInfraOKE.SetStackState(okeRuntimeInstance.ResourceInventory); err != nil {
				return fmt.Errorf("failed to restore state for OKE runtime instance %d: %w", *okeRuntimeInstance.ID, err)
			}
		}
	case v0.KubernetesRuntimeInfraProviderGKE:
		kubernetesRuntimeInfraGKE := provider.KubernetesRuntimeInfraGKE{
			PulumiWorkspace: provider.PulumiWorkspace{
				RuntimeInstanceName: provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName),
			},
		}
		// load the GCP project ID and region from the existing Pulumi stack configuration
		if err := kubernetesRuntimeInfraGKE.LoadConfigFromStack(); err != nil {
			return fmt.Errorf("failed to load GKE config from Pulumi stack: %w", err)
		}
		if kubeConnection, err = kubernetesRuntimeInfraGKE.GetConnection(); err != nil {
			return fmt.Errorf("failed to get connection for GKE kubernetes runtime infra: %w", err)
		}
		kubernetesRuntimeInfra = &kubernetesRuntimeInfraGKE

		// pull GKE stack state from GKE runtime instance
		// if not infra-only, as this flag implies the user
		// does not want to depend on control plane state
		if !cpi.Opts.InfraOnly {
			// pull GKE stack state from GKE runtime instance
			// and save to local pulumi state directory
			gkeRuntimeInstance, err := client.GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst(
				apiClient,
				threeportControlPlaneConfig.APIServer,
				*kubernetesRuntimeInstance.ID,
			)
			if err != nil {
				return fmt.Errorf(
					"failed to get OCI OKE kubernetes runtime instance by kubernetes runtime instance ID %d: %w",
					*kubernetesRuntimeInstance.ID,
					err,
				)
			}

			if err := kubernetesRuntimeInfraGKE.SetStackState(gkeRuntimeInstance.ResourceInventory); err != nil {
				return fmt.Errorf("failed to set OKE stack state: %w", err)
			}
		}
	}

	// Only tear down control plane first if not infra-only
	// and infrastructure hasn't already been destroyed
	if !cpi.Opts.InfraOnly && !infraAlreadyDestroyed {

		// check for kubernetes workload instances on non-kind kubernetes runtimes -
		// halt delete if any are present
		if threeportControlPlaneConfig.Provider != v0.KubernetesRuntimeInfraProviderKind {
			workloadInstances, err := client.GetKubernetesWorkloadInstances(
				apiClient,
				threeportControlPlaneConfig.APIServer,
			)
			if err != nil {
				return fmt.Errorf("failed to retrieve kubernetes workload instances from threeport API: %w", err)
			}
			if len(*workloadInstances) > 0 {
				return errors.New("found kubernetes workload instances that could prevent control plane deletion - delete all kubernetes workload instances before deleting control plane")
			}

			// get control plane instances
			controlPlaneInstances, err := client.GetControlPlaneInstances(
				apiClient,
				threeportControlPlaneConfig.APIServer,
			)
			if err != nil {
				return fmt.Errorf("failed to retrieve control plane instances from threeport API: %w", err)
			}
			if len(*controlPlaneInstances) > 1 {
				return errors.New("found non-genesis control plane instance(s) that could prevent control plane deletion - delete all non-genesis control plane instances before deleting genesis control plane")
			}
		}

		// for providers that use auth tokens, ensure we have the latest token
		switch threeportControlPlaneConfig.Provider {
		case v0.KubernetesRuntimeInfraProviderEKS:
			kubernetesRuntimeInstance, err = RefreshEKSConnectionWithLocalConfig(awsConfigResourceManager, kubernetesRuntimeInstance, apiClient, threeportControlPlaneConfig.APIServer)
			if err != nil {
				return fmt.Errorf("failed to refresh EKS connection with local config: %w", err)
			}
		case v0.KubernetesRuntimeInfraProviderOKE:
			kubernetesRuntimeInstance.ConnectionToken = &kubeConnection.Token
			kubernetesRuntimeInstance.ConnectionTokenExpiration = &kubeConnection.TokenExpiration
			kubernetesRuntimeInstance, err = client.UpdateKubernetesRuntimeInstance(
				apiClient,
				threeportControlPlaneConfig.APIServer,
				kubernetesRuntimeInstance,
			)
			if err != nil {
				return fmt.Errorf("failed to update OKE token on kubernetes runtime instance: %w", err)
			}
		}

		// create a client and resource mapper to connect to kubernetes cluster
		// API for deleting resources
		var dynamicKubeClient dynamic.Interface
		var mapper *meta.RESTMapper
		dynamicKubeClient, mapper, err = kube.GetClient(
			kubernetesRuntimeInstance,
			false,
			apiClient,
			threeportControlPlaneConfig.APIServer,
			threeportControlPlaneConfig.EncryptionKey,
		)
		if err != nil {
			return fmt.Errorf("failed to get a Kubernetes client and mapper: %w", err)
		}

		if err := cpi.UnInstallThreeportControlPlaneComponents(dynamicKubeClient, mapper); err != nil {
			return fmt.Errorf("failed to delete control plane components for threeport: %w", err)
		}
	} else {
		Info("Skipping control plane teardown")
	}

	if cpi.Opts.ControlPlaneOnly {
		Info("Skipping infra teardown")
	} else if infraAlreadyDestroyed {
		// infrastructure already destroyed in a previous run — skip
		// Pulumi destroy and go straight to IAM cleanup
		Info("Skipping infra destroy (already completed)")
		if err := kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraOKE).DeleteOCIResources(); err != nil {
			return fmt.Errorf("failed to delete OCI IAM resources: %w", err)
		}
	} else {

		// delete control plane infra
		if err := kubernetesRuntimeInfra.Delete(); err != nil {
			return fmt.Errorf("failed to delete control plane infra: %w", err)
		}

		// perform provider-specfic post-deletion cleanup
		switch threeportControlPlaneConfig.Provider {
		case v0.KubernetesRuntimeInfraProviderEKS:

			// remove inventory file
			invFile := provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.ControlPlaneName)
			if err := os.Remove(invFile); err != nil {
				Warning(fmt.Sprintf("failed to remove inventory file %s", invFile))
			}

			// delete AWS IAM resources
			err = provider.DeleteResourceManagerRole(cpi.Opts.ControlPlaneName, *awsConfigUser)
			if err != nil {
				return fmt.Errorf("failed to delete threeport AWS IAM resources: %w", err)
			}
		case v0.KubernetesRuntimeInfraProviderOKE:
			// delete Pulumi stack state directory
			if err := kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraOKE).DeleteStackState(); err != nil {
				Warning(fmt.Sprintf("failed to delete Pulumi stack state: %v", err))
			}

			// delete OCI user, groups, and compartment
			if err := kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraOKE).DeleteOCIResources(); err != nil {
				return fmt.Errorf("failed to delete OCI IAM resources: %w", err)
			}
		}
	}

	// if control plane only flag is set, hold onto the control plane in the threeport
	// config so we may tear-down with infra-only flag later
	if !cpi.Opts.ControlPlaneOnly || cpi.Opts.InfraOnly {
		// update threeport config to remove deleted threeport instance
		DeleteThreeportConfigControlPlane(threeportConfig, cpi.Opts.ControlPlaneName)
		Info("Threeport config updated")
	}

	Complete(fmt.Sprintf("Threeport control plane %s deleted", cpi.Opts.ControlPlaneName))

	return nil
}

// validateCreateControlPlaneFlags validates flag inputs as needed
func ValidateCreateGenesisControlPlaneFlags(
	instanceName string,
	infraProvider string,
	createRootDomain string,
	authEnabled bool,
	kindPortMappings []string,
	controlPlaneOnly bool,
	clusterName string,
) error {
	// ensure name length doesn't exceed maximum
	if utf8.RuneCountInString(instanceName) > threeport.InstanceNameMaxLength {
		return fmt.Errorf(
			"instance name is too long - cannot exceed %d characters",
			threeport.InstanceNameMaxLength,
		)
	}

	// validate infra provider is supported
	allowedInfraProviders := v0.SupportedInfraProviders()
	matched := false
	if slices.Contains(allowedInfraProviders, v0.KubernetesRuntimeInfraProvider(infraProvider)) {
		matched = true
	}
	if !matched {
		return fmt.Errorf(
			"invalid provider value '%s' - must be one of %s",
			infraProvider, allowedInfraProviders,
		)
	}

	// return an error if kind port mappings are provided for a non-kind provider
	if infraProvider != v0.KubernetesRuntimeInfraProviderKind && len(kindPortMappings) > 0 {
		return errors.New("kind port mappings are only supported for infrastructure provider 'kind'")
	}

	// --cluster-name doesn't apply outside --control-plane-only mode;
	// the tptctl up path derives the cluster name from --name
	if !controlPlaneOnly && clusterName != "" {
		return errors.New("--cluster-name is only valid with --control-plane-only")
	}

	return nil
}

// cleanOnCreateError cleans up created infra for a control plane when a
// provisioning error of any kind occurs.
func (u *Uninstaller) cleanOnCreateError(
	createErrMsg string,
	createErr error,
) error {

	if createErrMsg != "" {
		Error(createErrMsg, createErr)
		createErr = fmt.Errorf("%s: %w", createErrMsg, createErr)
	}

	// if teardownOnFailure is not set, return error without tearing down infras
	if !*u.teardownOnFailure {
		return createErr
	}

	// if needed, delete control plane workloads to remove related infra, e.g. load
	// balancers, that will prevent runtime infra deletion
	if u.dynamicKubeClient != nil && u.mapper != nil {
		if workloadErr := u.cpi.UnInstallThreeportControlPlaneComponents(*u.dynamicKubeClient, u.mapper); workloadErr != nil {
			return fmt.Errorf("failed to create control plane infra for threeport: %w\nfailed to delete threeport API components: %w", createErr, workloadErr)
		}
	}

	// if control plane only, return error without tearing down infra
	if u.cpi.Opts.ControlPlaneOnly {
		return createErr
	}

	// if eks provider, load inventory for deletion
	switch u.controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:

		// allow 2 seconds for pending inventory writes to complete
		time.Sleep(time.Second * 2)
		var inventory eks.EksInventory
		if invErr := inventory.Load(
			provider.EKSInventoryFilepath(u.cpi.Opts.ProviderConfigDir, u.cpi.Opts.ControlPlaneName),
		); invErr != nil {
			return fmt.Errorf("failed to create control plane infra for threeport: %w\nfailed to read eks kubernetes runtime inventory for resource deletion: %w", createErr, invErr)
		}
		u.kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraEKS).ResourceInventory = &inventory
	}

	// delete infra
	if deleteErr := u.kubernetesRuntimeInfra.Delete(); deleteErr != nil {
		return fmt.Errorf("failed to create control plane infra for threeport: %w\nfailed to delete control plane infra, you may have dangling kubernetes runtime infra resources still running: %w", createErr, deleteErr)
	}
	Info("Created Threeport infra deleted due to error")

	switch u.controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		Info("Deleting Threeport AWS IAM")
		err := provider.DeleteResourceManagerRole(u.cpi.Opts.ControlPlaneName, *u.awsConfig)
		if err != nil {
			return fmt.Errorf("failed to delete threeport AWS IAM resources: %w", err)
		}
		Info("Threeport AWS IAM resources deleted")

		// remove inventory file
		invFile := provider.EKSInventoryFilepath(u.cpi.Opts.ProviderConfigDir, u.cpi.Opts.ControlPlaneName)
		if err := os.Remove(invFile); err != nil {
			Warning(fmt.Sprintf("failed to remove inventory file %s", invFile))
		}
	case v0.KubernetesRuntimeInfraProviderOKE:
		Info("Deleting Threeport OCI IAM resources")
		kubernetesRuntimeInfraOKE := u.kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraOKE)
		if err := kubernetesRuntimeInfraOKE.DeleteOCIResources(); err != nil {
			Warning(fmt.Sprintf("failed to delete OCI IAM resources: %v", err))
		}
		if err := kubernetesRuntimeInfraOKE.DeleteStackState(); err != nil {
			Warning(fmt.Sprintf("failed to delete Pulumi stack state: %v", err))
		}
		Info("Threeport OCI IAM resources deleted")
	}

	// remove control plane from Threeport config
	if *u.cleanConfig {
		threeportConfig, _, configErr := GetThreeportConfig("")
		if configErr != nil {
			Warning("Threeport config may contain invalid instance for deleted control plane")
			return fmt.Errorf("failed to create control plane infra for threeport: %w\nfailed to get threeport config: %w", createErr, configErr)
		}
		DeleteThreeportConfigControlPlane(threeportConfig, u.cpi.Opts.ControlPlaneName)
	}

	return createErr
}

// runtimeInstanceName returns the effective kubernetes runtime instance
// name for the deployment.
func runtimeInstanceName(opts threeport.Options) string {
	if opts.ControlPlaneOnly {
		// existing cluster. opts.ClusterName comes from:
		//   - --cluster-name when the user supplies it
		//   - default applied in cmd/tptctl: the threeport- prefixed
		//     name, matching clusters tptctl provisions itself
		return opts.ClusterName
	}
	// new cluster, named with the threeport- prefix
	return provider.ThreeportRuntimeName(opts.ControlPlaneName)
}
