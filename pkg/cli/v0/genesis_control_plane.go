package v0

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	builder_client "github.com/nukleros/aws-builder/pkg/client"
	builder_config "github.com/nukleros/aws-builder/pkg/config"
	"github.com/nukleros/aws-builder/pkg/eks"
	"github.com/nukleros/aws-builder/pkg/eks/connection"
	builder_iam "github.com/nukleros/aws-builder/pkg/iam"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
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
	AwsRoleArn            string
	AwsSerialNumber       string
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
	SkipTeardown          bool
	ControlPlaneOnly      bool
	KindInfraPortForward  []string
}

// Uninstaller contains the necessary information to uninstall a control plane
// via its cleanOnCreate method.
type Uninstaller struct {
	createErrMsg           string
	createErr              error
	controlPlane           *threeport.ControlPlane
	kubernetesRuntimeInfra *provider.KubernetesRuntimeInfra
	dynamicKubeClient      *dynamic.Interface
	mapper                 *meta.RESTMapper
	cleanConfig            *bool
	cpi                    *threeport.ControlPlaneInstaller
	awsConfig              *aws.Config
	skipTeardown           *bool
}

const tier = threeport.ControlPlaneTierDev

// InitArgs sets the default provider config directory, kubeconfig path and path
// to threeport repo as needed in the CLI arguments.
func InitArgs(args *GenesisControlPlaneCLIArgs) {
	// provider config dir
	if args.ProviderConfigDir == "" {
		providerConf, err := config.DefaultProviderConfigDir()
		if err != nil {
			Error("failed to set infra provider config directory", err)
			os.Exit(1)
		}
		args.ProviderConfigDir = providerConf
	}

	// kubeconfig
	defaultKubeconfig, err := kube.DefaultKubeconfig()
	if err != nil {
		Error("failed to get path to default kubeconfig", err)
		os.Exit(1)
	}
	args.KubeconfigPath = defaultKubeconfig

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
	cpi.Opts.LiveReload = false
	cpi.Opts.CreateOrUpdateKubeResources = false
	cpi.Opts.ControlPlaneOnly = a.ControlPlaneOnly
	cpi.Opts.RestApiEksLoadBalancer = true
	cpi.Opts.SkipTeardown = a.SkipTeardown

	return cpi, nil
}

// CreateGenesisControlPlane uses the CLI arguments to create a new threeport control
// plane.
func CreateGenesisControlPlane(customInstaller *threeport.ControlPlaneInstaller) error {
	// get the threeport config
	threeportConfig, _, err := config.GetThreeportConfig("")
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
		cpi:          cpi,
		skipTeardown: &cpi.Opts.SkipTeardown,
	}

	// check threeport config to see if it is empty
	threeportInstanceConfigEmpty := threeportConfig.CheckThreeportConfigEmpty()
	if !threeportInstanceConfigEmpty && !cpi.Opts.ForceOverwriteConfig {
		return ErrThreeportConfigAlreadyExists
	}

	genesis := true

	threeportConfig.ControlPlanes = []config.ControlPlane{}
	threeportControlPlaneConfig := &config.ControlPlane{}

	// create threeport config for new instance
	if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {
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

	// configure the infra provider
	var kubernetesRuntimeInfra provider.KubernetesRuntimeInfra
	var threeportAPIEndpoint string
	var callerIdentity *sts.GetCallerIdentityOutput
	var kubeConnectionInfo *kube.KubeConnectionInfo
	awsConfigUser := aws.Config{}
	uninstaller.awsConfig = &awsConfigUser
	awsConfigResourceManager := &aws.Config{}
	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderKind:

		portForwards := make(map[int32]int32)
		for _, mapping := range cpi.Opts.KindInfraPortForward {
			split := strings.Split(mapping, ":")
			if len(split) != 2 {
				return fmt.Errorf("failed to parse kind port forward %s", mapping)
			}

			containerPort, err := strconv.ParseInt(split[0], 10, 32)
			if err != nil {
				return fmt.Errorf("failed to parse container port: %s as int32", split[0])
			}

			hostPort, err := strconv.ParseInt(split[1], 10, 32)
			if err != nil {
				return fmt.Errorf("failed to parse host port: %s as int32", split[0])
			}

			portForwards[int32(containerPort)] = int32(hostPort)
		}

		// construct kind infra provider object
		kubernetesRuntimeInfraKind := provider.KubernetesRuntimeInfraKind{
			RuntimeInstanceName: provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName),
			KubeconfigPath:      cpi.Opts.KubeconfigPath,
			DevEnvironment:      cpi.Opts.DevEnvironment,
			ThreeportPath:       cpi.Opts.ThreeportPath,
			NumWorkerNodes:      cpi.Opts.NumWorkerNodes,
			AuthEnabled:         cpi.Opts.AuthEnabled,
			PortForwards:        portForwards,
		}

		// update threeport config with api endpoint
		threeportAPIEndpoint = threeport.GetLocalThreeportAPIEndpoint(cpi.Opts.AuthEnabled)
		if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {
			c.APIServer = threeportAPIEndpoint
		}); err != nil {
			return fmt.Errorf("failed to update threeport config: %w", err)
		}

		// delete kind kubernetes runtime if interrupted
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			Warning("received Ctrl+C, removing kind kubernetes runtime...")
			// first update the threeport config so the Delete method has
			// something to reference
			threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {})
			if err := DeleteGenesisControlPlane(cpi); err != nil {
				Error("failed to delete kind kubernetes runtime", err)
			}
			os.Exit(1)
		}()

		kubernetesRuntimeInfra = &kubernetesRuntimeInfraKind
		uninstaller.kubernetesRuntimeInfra = &kubernetesRuntimeInfra
		if cpi.Opts.ControlPlaneOnly {
			kubeConnectionInfo, err = kube.GetConnectionInfoFromKubeconfig(kubernetesRuntimeInfraKind.KubeconfigPath)
			if err != nil {
				return fmt.Errorf("failed to get connection info for kind kubernetes runtime: %w", err)
			}
		} else {
			kubeConnectionInfo, err = kubernetesRuntimeInfra.Create()
			if err != nil {
				return uninstaller.cleanOnCreateError("failed to create control plane infra for threeport", err)
			}
		}
	case v0.KubernetesRuntimeInfraProviderEKS:
		// create AWS config
		awsConf, err := builder_config.LoadAWSConfig(
			cpi.Opts.AwsConfigEnv,
			cpi.Opts.AwsConfigProfile,
			cpi.Opts.AwsRegion,
			"",
			"",
			"",
		)
		if err != nil {
			return fmt.Errorf("failed to load AWS configuration with local config: %w", err)
		}
		awsConfigUser = *awsConf

		// get account ID
		if callerIdentity, err = provider.GetCallerIdentity(awsConf); err != nil {
			return fmt.Errorf("failed to get caller identity: %w", err)
		}
		Info(fmt.Sprintf("Successfully authenticated to account %s as %s", *callerIdentity.Account, *callerIdentity.Arn))

		// update threeport config with eks provider info
		if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {
			c.EKSProviderConfig = config.EKSProviderConfig{
				AwsConfigProfile: cpi.Opts.AwsConfigProfile,
				AwsRegion:        cpi.Opts.AwsRegion,
				AwsAccountID:     *callerIdentity.Account,
			}
		}); err != nil {
			return fmt.Errorf("failed to update threeport config: %w", err)
		}

		if !cpi.Opts.ControlPlaneOnly {
			Info("Creating Threeport IAM role")

			// create IAM role for resource management
			resourceManagerRoleName := provider.GetResourceManagerRoleName(cpi.Opts.ControlPlaneName)
			_, err = provider.CreateResourceManagerRole(
				builder_iam.CreateIamTags(
					cpi.Opts.Name,
					map[string]string{},
				),
				resourceManagerRoleName,
				*callerIdentity.Account,
				"",
				"",
				"",
				true,
				true,
				awsConfigUser,
			)
			if err != nil {
				deleteErr := provider.DeleteResourceManagerRole(cpi.Opts.ControlPlaneName, awsConfigUser)
				if deleteErr != nil {
					return fmt.Errorf("failed to create runtime manager role: %w, failed to delete IAM resources: %w", err, deleteErr)
				}
				return fmt.Errorf("failed to create runtime manager role: %w", err)
			}
		}

		// assume IAM role for resource management
		awsConfigResourceManager, err = builder_config.AssumeRole(
			provider.GetResourceManagerRoleArn(
				cpi.Opts.ControlPlaneName,
				*callerIdentity.Account,
			),
			"",
			"",
			3600,
			awsConfigUser,
			[]func(*aws_config.LoadOptions) error{
				aws_config.WithRegion(cpi.Opts.AwsRegion),
			},
		)
		if err != nil {
			deleteErr := provider.DeleteResourceManagerRole(cpi.Opts.ControlPlaneName, awsConfigUser)
			if deleteErr != nil {
				return fmt.Errorf("failed to assume role for AWS resource manager: %w, failed to delete IAM resources: %w", err, deleteErr)
			}
			return fmt.Errorf("failed to assume role for AWS resource manager: %w", err)
		}

		if !cpi.Opts.ControlPlaneOnly {
			// wait for IAM role to be available
			Info("Waiting for IAM role to become available...")
			if err = util.Retry(30, 1, func() error {
				if callerIdentity, err = provider.GetCallerIdentity(awsConfigResourceManager); err != nil {
					return fmt.Errorf("failed to get caller identity: %w", err)
				}
				Info(fmt.Sprintf("Successfully authenticated to account %s as %s", *callerIdentity.Account, *callerIdentity.Arn))

				// wait 5 seconds to allow IAM resources to become available
				time.Sleep(time.Second * 5)

				Info("IAM resources created")
				return nil
			}); err != nil {
				deleteErr := provider.DeleteResourceManagerRole(cpi.Opts.ControlPlaneName, awsConfigUser)
				if deleteErr != nil {
					return fmt.Errorf("failed to wait for IAM resources to be available: %w, failed to delete IAM resources: %w", err, deleteErr)
				}
				return fmt.Errorf("failed to wait for IAM resources to be available: %w", err)
			}
		}

		// create a resource client to create EKS resources
		eksInventoryChan := make(chan eks.EksInventory)
		eksClient := eks.EksClient{
			*builder_client.CreateResourceClient(awsConfigResourceManager),
			&eksInventoryChan,
		}

		// capture messages as resources are created and return to user
		go func() {
			for msg := range *eksClient.MessageChan {
				Info(msg)
			}
		}()

		// capture inventory and write to file as it is created
		go func() {
			for inventory := range *eksClient.InventoryChan {
				if err := inventory.Write(
					provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.ControlPlaneName),
				); err != nil {
					Error("failed to write inventory file", err)
				}
			}
		}()

		// delete eks kubernetes runtime resources if interrupted
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			Warning("received Ctrl+C, cleaning up resources...")
			// allow 2 seconds for pending inventory writes to complete
			time.Sleep(time.Duration(2) * time.Second)
			if err := uninstaller.cleanOnCreateError("", nil); err != nil {
				Error("failed to clean up resources: ", err)
			}
			os.Exit(1)
		}()

		// TODO: add flags to tptctl command for high availability, etc to
		// deterimine these values
		// construct eks kubernetes runtime infra object
		kubernetesRuntimeInfraEKS := provider.KubernetesRuntimeInfraEKS{
			RuntimeInstanceName:          provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName),
			AwsAccountID:                 *callerIdentity.Account,
			AwsConfig:                    awsConfigResourceManager,
			ResourceClient:               &eksClient,
			ZoneCount:                    int32(2),
			DefaultNodeGroupInstanceType: "t3.medium",
			DefaultNodeGroupInitialNodes: int32(3),
			DefaultNodeGroupMinNodes:     int32(3),
			DefaultNodeGroupMaxNodes:     int32(250),
		}

		kubernetesRuntimeInfra = &kubernetesRuntimeInfraEKS

		if cpi.Opts.ControlPlaneOnly {
			kubeConnectionInfo, err = kubernetesRuntimeInfraEKS.GetConnection()
			if err != nil {
				return fmt.Errorf("failed to get connection info for eks kubernetes runtime: %w", err)
			}
		} else {
			kubeConnectionInfo, err = kubernetesRuntimeInfra.Create()
			if err != nil {
				return uninstaller.cleanOnCreateError("failed to create control plane infra for threeport", err)
			}
		}
	}

	// update threeport config with kube API info
	if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {
		c.KubeAPI = config.KubeAPI{
			APIEndpoint:   kubeConnectionInfo.APIEndpoint,
			CACertificate: util.Base64Encode(kubeConnectionInfo.CACertificate),
			Certificate:   util.Base64Encode(kubeConnectionInfo.Certificate),
			Key:           util.Base64Encode(kubeConnectionInfo.Key),
			EKSToken:      util.Base64Encode(kubeConnectionInfo.EKSToken),
		}
	}); err != nil {
		return uninstaller.cleanOnCreateError("failed to update threeport config", err)
	}

	// generate encryption key
	encryptionKey, err := encryption.GenerateKey()
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to generate encryption key", err)
	}

	// update threeport config with encryption key
	if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {
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
	var kubernetesRuntimeInstance v0.KubernetesRuntimeInstance
	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderKind:
		location := "Local"
		kubernetesRuntimeInstance = v0.KubernetesRuntimeInstance{
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

		// update resource manager role to allow pods to assume it
		var inventory eks.EksInventory
		if err := inventory.Load(
			provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.ControlPlaneName),
		); err != nil {
			return uninstaller.cleanOnCreateError("failed to read eks kubernetes runtime inventory for inventory update", err)
		}
		if err = provider.UpdateResourceManagerRoleTrustPolicy(
			cpi.Opts.ControlPlaneName,
			*callerIdentity.Account,
			"",
			inventory.Cluster.OidcProviderUrl,
			awsConfigUser,
		); err != nil {
			return uninstaller.cleanOnCreateError("failed to update resource manager role", err)
		}

		location, err := mapping.GetLocationForAwsRegion(awsConfigResourceManager.Region)
		if err != nil {
			return uninstaller.cleanOnCreateError(fmt.Sprintf("failed to get threeport location for AWS region %s", awsConfigResourceManager.Region), err)
		}

		kubernetesRuntimeInstance = v0.KubernetesRuntimeInstance{
			Instance: v0.Instance{
				Name: &kubernetesRuntimeInstName,
			},
			Reconciliation: v0.Reconciliation{
				Reconciled: &instReconciled,
			},
			Location:                  &location,
			ThreeportControlPlaneHost: &controlPlaneHost,
			APIEndpoint:               &kubeConnectionInfo.APIEndpoint,
			CACertificate:             &kubeConnectionInfo.CACertificate,
			ConnectionToken:           &kubeConnectionInfo.EKSToken,
			ConnectionTokenExpiration: &kubeConnectionInfo.EKSTokenExpiration,
			DefaultRuntime:            &defaultRuntime,
		}
	}

	// get kubernetes client and mapper for use with kube API
	// we don't have a client or endpoint for threeport API yet - but those are
	// only used when a token refresh is needed and that should not be necessary
	dynamicKubeClient, mapper, err := kube.GetClient(
		&kubernetesRuntimeInstance,
		false,
		nil,
		"",
		"",
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to get a Kubernetes client and mapper", err)
	}
	uninstaller.dynamicKubeClient = &dynamicKubeClient
	uninstaller.mapper = mapper

	// install the threeport control plane dependencies
	if err := cpi.InstallThreeportControlPlaneDependencies(
		dynamicKubeClient,
		mapper,
		cpi.Opts.InfraProvider,
		encryptionKey,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport control plane dependencies", err)
	}

	// if auth is enabled, generate client certificate and add to local config
	var authConfig *auth.AuthConfig
	var clientCredentials *config.Credential
	if cpi.Opts.AuthEnabled {
		// get auth config
		authConfig, err = auth.GetAuthConfig()
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to get auth config", err)
		}

		// generate client certificate
		clientCertificate, clientPrivateKey, err := auth.GenerateCertificate(
			authConfig.CAConfig,
			&authConfig.CAPrivateKey,
		)
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to generate client certificate and private key", err)
		}

		clientCredentials = &config.Credential{
			Name:       cpi.Opts.ControlPlaneName,
			ClientCert: util.Base64Encode(clientCertificate),
			ClientKey:  util.Base64Encode(clientPrivateKey),
		}

		// update threeport config with auth info
		if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {
			c.AuthEnabled = true
			c.Credentials = append(c.Credentials, *clientCredentials)
			c.CACert = authConfig.CABase64Encoded
		}); err != nil {
			return uninstaller.cleanOnCreateError("failed to update threeport config", err)
		}
	} else {
		// update threeport config with auth info
		if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {
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

	err = cpi.Opts.PreInstallFunction(&kubernetesRuntimeInstance, cpi)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to run custom preInstall function", err)
	}

	// install the API
	if err := cpi.UpdateThreeportAPIDeployment(
		dynamicKubeClient,
		mapper,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport API server", err)
	}

	// for a cloud provider installed control plane:
	// * determine the threeport API's remote endpoint to add to the threeport
	//   config and to add to the server certificate's alt names when TLS
	//   assets are installed
	// * install provider-specific kubernetes resources
	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		threeportAPIEndpoint, err = cpi.GetThreeportAPIEndpoint(dynamicKubeClient, *mapper)
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to get threeport API's public endpoint", err)
		}
		if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {
			c.APIServer = fmt.Sprintf("%s:%d", threeportAPIEndpoint, threeport.GetThreeportAPIPort(cpi.Opts.AuthEnabled))
		}); err != nil {
			return uninstaller.cleanOnCreateError("failed to update threeport config", err)
		}

		// create and configure service accounts for workload and aws controllers,
		// which will be used to authenticate to AWS via IRSA

		// configure IRSA controllers to use appropriate service account names
		provider.UpdateIrsaControllerList(cpi.Opts.ControllerList)

		// create IRSA service accounts
		for _, serviceAccount := range provider.GetIrsaServiceAccounts(
			cpi.Opts.Namespace,
			*callerIdentity.Account,
			provider.GetResourceManagerRoleName(cpi.Opts.ControlPlaneName),
		) {
			if err := cpi.CreateOrUpdateKubeResource(serviceAccount, dynamicKubeClient, mapper); err != nil {
				return uninstaller.cleanOnCreateError("failed to get threeport API's public endpoint", err)
			}
		}
	}

	// if auth enabled install the threeport API TLS assets that include the alt
	// name for the remote load balancer if applicable
	if cpi.Opts.AuthEnabled {
		// install the threeport API TLS assets
		if err := cpi.InstallThreeportAPITLS(
			dynamicKubeClient,
			mapper,
			authConfig,
			threeportAPIEndpoint,
		); err != nil {
			return uninstaller.cleanOnCreateError("failed to install threeport API TLS assets", err)
		}
	}

	// update uninstaller to clean the threeport config if an error occurs
	uninstaller.cleanConfig = util.Ptr(true)

	// wait for API server to start running - it is not strictly necessary to
	// wait for the API before installing the rest of the control plane, however
	// it is helpful for dev environments and harmless otherwise since the
	// controllers need the API to be running in order to start
	Info("Waiting for threeport API to start running...")
	attemptsMax := 30
	waitDurationSeconds := 10
	if err = util.Retry(attemptsMax, waitDurationSeconds, func() error {
		_, err := client.GetResponse(
			apiClient,
			fmt.Sprintf("%s/version", threeportAPIEndpoint),
			http.MethodGet,
			new(bytes.Buffer),
			map[string]string{},
			http.StatusOK,
		)
		if err != nil {
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

	// install the controllers
	if err := cpi.InstallThreeportControllers(
		dynamicKubeClient,
		mapper,
		authConfig,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport controllers", err)
	}

	err = cpi.Opts.PostInstallFunction(&kubernetesRuntimeInstance, cpi)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to run custom postInstall function", err)
	}

	// install the agent
	if err := cpi.InstallThreeportAgent(
		dynamicKubeClient,
		mapper,
		cpi.Opts.ControlPlaneName,
		authConfig,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport agent", err)
	}

	// install support services CRDs
	err = threeport.InstallThreeportCRDs(dynamicKubeClient, mapper)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport support services CRDs", err)
	}

	// wait for kube API to persist the change and refresh the client and mapper
	// this is necessary to have the updated REST mapping for the CRDs as the
	// support services operator install includes one of those custom resources
	time.Sleep(time.Second * 10)
	dynamicKubeClient, mapper, err = kube.GetClient(
		&kubernetesRuntimeInstance,
		false,
		nil,
		"",
		"",
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to refresh the Kubernetes client and mapper", err)
	}

	// install the support services operator
	err = threeport.InstallThreeportSupportServicesOperator(dynamicKubeClient, mapper)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to install threeport support services operator", err)
	}

	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		// install system services incl cluster autoscaler
		if err := threeport.InstallThreeportSystemServices(
			dynamicKubeClient,
			mapper,
			cpi.Opts.InfraProvider,
			cpi.Opts.Name+"-"+cpi.Opts.ControlPlaneName,
			*callerIdentity.Account,
		); err != nil {
			return uninstaller.cleanOnCreateError("failed to install system services", err)
		}
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
		&kubernetesRuntimeInstance,
	)
	if err != nil {
		return uninstaller.cleanOnCreateError("failed to create new kubernetes runtime instance for default compute space", err)
	}

	// for eks clusters:
	// * create aws account
	// * set region in threeport config
	// * create aws eks k8s runtime definition
	// * create aws eks k8s runtime instance
	// * copy aws eks resource inventory to cluster
	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:

		awsAccountName := "default-account"
		defaultAccount := true

		roleArn := provider.GetResourceManagerRoleArn(
			cpi.Opts.ControlPlaneName,
			*callerIdentity.Account,
		)
		awsAccount := v0.AwsAccount{
			Name:           &awsAccountName,
			AccountID:      callerIdentity.Account,
			DefaultAccount: &defaultAccount,
			DefaultRegion:  &awsConfigResourceManager.Region,
			RoleArn:        &roleArn,
		}
		createdAwsAccount, err := client.CreateAwsAccount(
			apiClient,
			threeportAPIEndpoint,
			&awsAccount,
		)
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to create new default AWS account", err)
		}

		// create aws eks k8s runtime definition
		eksRuntimeDefName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
		kubernetesRuntimeInfraEKS := kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraEKS)
		zoneCount := int(kubernetesRuntimeInfraEKS.ZoneCount)
		defaultNodeGroupInitialSize := int(kubernetesRuntimeInfraEKS.DefaultNodeGroupInitialNodes)
		defaultNodeGroupMinSize := int(kubernetesRuntimeInfraEKS.DefaultNodeGroupMinNodes)
		defaultNodeGroupMaxSize := int(kubernetesRuntimeInfraEKS.DefaultNodeGroupMaxNodes)
		awsEksKubernetesRuntimeDef := v0.AwsEksKubernetesRuntimeDefinition{
			Definition: v0.Definition{
				Name: &eksRuntimeDefName,
			},
			AwsAccountID:                  createdAwsAccount.ID,
			ZoneCount:                     &zoneCount,
			DefaultNodeGroupInstanceType:  &kubernetesRuntimeInfraEKS.DefaultNodeGroupInstanceType,
			DefaultNodeGroupInitialSize:   &defaultNodeGroupInitialSize,
			DefaultNodeGroupMinimumSize:   &defaultNodeGroupMinSize,
			DefaultNodeGroupMaximumSize:   &defaultNodeGroupMaxSize,
			KubernetesRuntimeDefinitionID: kubernetesRuntimeDefResult.ID,
		}
		createdAwsEksKubernetesRuntimeDef, err := client.CreateAwsEksKubernetesRuntimeDefinition(
			apiClient,
			threeportAPIEndpoint,
			&awsEksKubernetesRuntimeDef,
		)
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to create new AWS EKS kubernetes runtime definition for control plane cluster", err)
		}

		// create aws eks k8s runtime instance
		var inventory eks.EksInventory
		if err := inventory.Load(
			provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.ControlPlaneName),
		); err != nil {
			return uninstaller.cleanOnCreateError("failed to read eks kubernetes runtime inventory for inventory update", err)
		}
		inventoryJson, err := inventory.Marshal()
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to marshal eks kubernetes runtime inventory for inventory update", err)
		}
		dbInventory := datatypes.JSON(inventoryJson)
		eksRuntimeInstName := provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName)
		reconciled := true
		awsEksKubernetesRuntimeInstance := v0.AwsEksKubernetesRuntimeInstance{
			Instance: v0.Instance{
				Name: &eksRuntimeInstName,
			},
			Reconciliation: v0.Reconciliation{
				Reconciled: &reconciled,
			},
			Region:                              &awsConfigResourceManager.Region,
			AwsEksKubernetesRuntimeDefinitionID: createdAwsEksKubernetesRuntimeDef.ID,
			KubernetesRuntimeInstanceID:         kubernetesRuntimeInstResult.ID,
			ResourceInventory:                   &dbInventory,
		}
		_, err = client.CreateAwsEksKubernetesRuntimeInstance(
			apiClient,
			threeportAPIEndpoint,
			&awsEksKubernetesRuntimeInstance,
		)
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to create new AWS EKS kubernetes runtime instance for control plane cluster", err)
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
	threeportConfig, requestedControlPlane, err := config.GetThreeportConfig("")
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

	var kubernetesRuntimeInfra provider.KubernetesRuntimeInfra
	var awsConfigUser *aws.Config
	var awsConfigResourceManager *aws.Config
	switch threeportControlPlaneConfig.Provider {
	case v0.KubernetesRuntimeInfraProviderKind:
		kubernetesRuntimeInfraKind := provider.KubernetesRuntimeInfraKind{
			RuntimeInstanceName: provider.ThreeportRuntimeName(threeportControlPlaneConfig.Name),
			KubeconfigPath:      cpi.Opts.KubeconfigPath,
		}
		kubernetesRuntimeInfra = &kubernetesRuntimeInfraKind
	case v0.KubernetesRuntimeInfraProviderEKS:
		// create AWS config
		// * AwsConfigEnv is always passed in from CLI args as it is not
		//   persisted in threeport config
		// * AwsConfigProfile and AwsRegion cannot be passed in through CLI for
		// deletion opertion as these are stored in threeport config
		// create a resource client to delete EKS resources

		var accountId string
		awsConfigUser, awsConfigResourceManager, accountId, err = threeportConfig.GetAwsConfigs(requestedControlPlane)
		if err != nil {
			return fmt.Errorf("failed to get AWS configs from threeport config: %w", err)
		}

		eksInventoryChan := make(chan eks.EksInventory)
		eksClient := eks.EksClient{
			*builder_client.CreateResourceClient(awsConfigResourceManager),
			&eksInventoryChan,
		}

		// capture messages as resources are created and return to user
		go func() {
			for msg := range *eksClient.MessageChan {
				Info(msg)
			}
		}()

		// capture inventory and write to file as it is updated
		go func() {
			for inventory := range *eksClient.InventoryChan {
				if err := inventory.Write(
					provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.ControlPlaneName),
				); err != nil {
					Error("failed to write inventory file", err)
				}
			}
		}()

		// read inventory to delete
		var inventory eks.EksInventory
		if err := inventory.Load(
			provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.ControlPlaneName),
		); err != nil {
			return fmt.Errorf("failed to read inventory file for deleting eks kubernetes runtime resources: %w", err)
		}

		// construct eks kubernetes runtime infra object
		kubernetesRuntimeInfraEKS := provider.KubernetesRuntimeInfraEKS{
			RuntimeInstanceName: provider.ThreeportRuntimeName(threeportControlPlaneConfig.Name),
			AwsAccountID:        accountId,
			AwsConfig:           awsConfigResourceManager,
			ResourceClient:      &eksClient,
			ResourceInventory:   &inventory,
		}
		kubernetesRuntimeInfra = &kubernetesRuntimeInfraEKS
	}

	ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForControlPlane(cpi.Opts.ControlPlaneName)
	if err != nil {
		return fmt.Errorf("failed to get threeport certificates from config: %w", err)
	}
	apiClient, err := client.GetHTTPClient(threeportControlPlaneConfig.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}

	// get the kubernetes runtime instance object
	kubernetesRuntimeInstance, err := client.GetThreeportControlPlaneKubernetesRuntimeInstance(
		apiClient,
		threeportControlPlaneConfig.APIServer,
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve kubernetes runtime instance from threeport API: %w", err)
	}

	// if provider is EKS we need to delete the threeport API service to
	// remove the AWS load balancer before deleting the rest of the infra and
	// check for existing workload instances that may prevent deletion
	switch threeportControlPlaneConfig.Provider {
	case v0.KubernetesRuntimeInfraProviderKind:
		if cpi.Opts.ControlPlaneOnly {

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
			// delete control plane infra
			if err := kubernetesRuntimeInfra.Delete(); err != nil {
				return fmt.Errorf("failed to delete control plane infra: %w", err)
			}
		}
	case v0.KubernetesRuntimeInfraProviderEKS:
		// check for workload instances on non-kind kubernetes runtimes - halt delete if
		// any are present
		workloadInstances, err := client.GetWorkloadInstances(
			apiClient,
			threeportControlPlaneConfig.APIServer,
		)
		if err != nil {
			return fmt.Errorf("failed to retrieve workload instances from threeport API: %w", err)
		}
		if len(*workloadInstances) > 0 {
			return errors.New("found workload instances that could prevent control plane deletion - delete all workload instances before deleting control plane")
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

		updatedKubernetesRuntimeInstance, err := RefreshEKSConnectionWithLocalConfig(awsConfigResourceManager, kubernetesRuntimeInstance, apiClient, threeportControlPlaneConfig.APIServer)
		if err != nil {
			return fmt.Errorf("failed to refresh EKS connection with local config: %w", err)
		}

		// create a client and resource mapper to connect to kubernetes cluster
		// API for deleting resources
		var dynamicKubeClient dynamic.Interface
		var mapper *meta.RESTMapper
		dynamicKubeClient, mapper, err = kube.GetClient(
			updatedKubernetesRuntimeInstance,
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

		if !cpi.Opts.ControlPlaneOnly {
			// delete control plane infra
			if err := kubernetesRuntimeInfra.Delete(); err != nil {
				return fmt.Errorf("failed to delete control plane infra: %w", err)
			}

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
		}
	}

	// update threeport config to remove deleted threeport instance
	config.DeleteThreeportConfigControlPlane(threeportConfig, cpi.Opts.ControlPlaneName)
	Info("Threeport config updated")

	Complete(fmt.Sprintf("Threeport control plane %s deleted", cpi.Opts.ControlPlaneName))

	return nil
}

// RefreshEKSConnectionWithLocalConfig uses the local AWS config to refresh
// EKS connection info on the kubernetes runtime instance object
func RefreshEKSConnectionWithLocalConfig(
	awsConfig *aws.Config,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	apiClient *http.Client,
	threeportAPIEndpoint string,
) (*v0.KubernetesRuntimeInstance, error) {
	// use local AWS config to get EKS cluster connection info
	eksClusterConn := connection.EksClusterConnectionInfo{ClusterName: *kubernetesRuntimeInstance.Name}
	if err := eksClusterConn.Get(awsConfig); err != nil {
		return nil, fmt.Errorf("failed to get EKS cluster connection info: %w", err)
	}

	kubernetesRuntimeInstance.ConnectionToken = &eksClusterConn.Token
	kubernetesRuntimeInstance.ConnectionTokenExpiration = &eksClusterConn.TokenExpiration
	updatedKubernetesRuntimeInst, err := client.UpdateKubernetesRuntimeInstance(
		apiClient,
		threeportAPIEndpoint,
		kubernetesRuntimeInstance,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update EKS token on kubernetes runtime instance: %w", err)
	}
	return updatedKubernetesRuntimeInst, nil
}

// validateCreateControlPlaneFlags validates flag inputs as needed
func ValidateCreateGenesisControlPlaneFlags(
	instanceName string,
	infraProvider string,
	createRootDomain string,
	authEnabled bool,
) error {
	// ensure name length doesn't exceed maximum
	if utf8.RuneCountInString(instanceName) > threeport.InstanceNameMaxLength {
		return errors.New(
			fmt.Sprintf(
				"instance name is too long - cannot exceed %d characters",
				threeport.InstanceNameMaxLength,
			),
		)
	}

	// validate infra provider is supported
	allowedInfraProviders := v0.SupportedInfraProviders()
	matched := false
	for _, prov := range allowedInfraProviders {
		if v0.KubernetesRuntimeInfraProvider(infraProvider) == prov {
			matched = true
			break
		}
	}
	if !matched {
		return errors.New(
			fmt.Sprintf(
				"invalid provider value '%s' - must be one of %s",
				infraProvider, allowedInfraProviders,
			),
		)
	}

	// TODO: We are currently deploying on EKS without internal auth enabled.
	// When we switch over to auth enabled internally we can re-enable this

	// ensure client cert auth is used on remote installations
	// if infraProvider != v0.KubernetesRuntimeInfraProviderKind && !authEnabled {
	// 	return errors.New(
	// 		"cannot turn off client certificate authentication unless using the kind provider",
	// 	)
	// }

	return nil
}

// cleanOnCreateError cleans up created infra for a control plane when a
// provisioning error of any kind occurs.
func (u *Uninstaller) cleanOnCreateError(
	createErrMsg string,
	createErr error,
) error {

	if createErrMsg != "" {
		// print the error when it happens and then again post-deletion
		Error(createErrMsg, createErr)
		createErr = fmt.Errorf("%s: %w", createErrMsg, createErr)
	}

	// if skipTeardown is set, return error without tearing down infras
	if *u.skipTeardown {
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
		(*u.kubernetesRuntimeInfra).(*provider.KubernetesRuntimeInfraEKS).ResourceInventory = &inventory
	}

	// delete infra
	if deleteErr := (*u.kubernetesRuntimeInfra).Delete(); deleteErr != nil {
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
	}

	if *u.cleanConfig {
		threeportConfig, _, configErr := config.GetThreeportConfig("")
		if configErr != nil {
			Warning("Threeport config may contain invalid instance for deleted control plane")
			return fmt.Errorf("failed to create control plane infra for threeport: %w\nfailed to get threeport config: %w", createErr, configErr)
		}
		config.DeleteThreeportConfigControlPlane(threeportConfig, u.cpi.Opts.ControlPlaneName)
	}

	return createErr
}
