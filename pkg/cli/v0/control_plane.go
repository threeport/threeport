package v0

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"gorm.io/datatypes"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	util "github.com/threeport/threeport/pkg/util/v0"
)

var ThreeportConfigAlreadyExistsErr = errors.New("threeport control plane with provided name already exists in threeport config")

// ControlPlaneCLIArgs is the set of control plane arguments passed to one of
// the CLI tools.
type ControlPlaneCLIArgs struct {
	AuthEnabled             bool
	AwsConfigProfile        string
	AwsConfigEnv            bool
	AwsRegion               string
	CfgFile                 string
	ControlPlaneImageRepo   string
	ControlPlaneImageTag    string
	CreateRootDomain        string
	CreateProviderAccountID string
	CreateAdminEmail        string
	DevEnvironment          bool
	ForceOverwriteConfig    bool
	ControlPlaneName        string
	InfraProvider           string
	KubeconfigPath          string
	NumWorkerNodes          int
	ProviderConfigDir       string
	ThreeportPath           string
}

const tier = threeport.ControlPlaneTierDev

// InitArgs sets the default provider config directory, kubeconfig path and path
// to threeport repo as needed in the CLI arguments.
func InitArgs(args *ControlPlaneCLIArgs) {
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

func (a *ControlPlaneCLIArgs) CreateInstaller() (*threeport.ControlPlaneInstaller, error) {
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
	cpi.Opts.CreateProviderAccountID = a.CreateProviderAccountID
	cpi.Opts.CreateAdminEmail = a.CreateAdminEmail
	cpi.Opts.DevEnvironment = a.DevEnvironment
	cpi.Opts.ForceOverwriteConfig = a.ForceOverwriteConfig
	cpi.Opts.InstanceName = a.ControlPlaneName
	cpi.Opts.InfraProvider = a.InfraProvider
	cpi.Opts.KubeconfigPath = a.KubeconfigPath
	cpi.Opts.NumWorkerNodes = a.NumWorkerNodes
	cpi.Opts.ProviderConfigDir = a.ProviderConfigDir
	cpi.Opts.ThreeportPath = a.ThreeportPath
	cpi.Opts.CreateOrUpdateKubeResources = false

	return cpi, nil
}

// CreateControlPlane uses the CLI arguments to create a new threeport control
// plane.
func CreateControlPlane(customInstaller *threeport.ControlPlaneInstaller) error {
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

	// check threeport config for existing instance config
	threeportInstanceConfigExists := threeportConfig.CheckThreeportConfigExists(cpi.Opts.InstanceName)
	if threeportInstanceConfigExists && !cpi.Opts.ForceOverwriteConfig {
		return ThreeportConfigAlreadyExistsErr
	}

	genesis := true

	// create threeport config for new instance
	threeportInstanceConfig := &config.ControlPlane{
		Name:     cpi.Opts.InstanceName,
		Provider: cpi.Opts.InfraProvider,
		Genesis:  genesis,
	}

	// configure the control plane
	controlPlane := threeport.ControlPlane{
		InfraProvider: v0.KubernetesRuntimeInfraProvider(cpi.Opts.InfraProvider),
		Tier:          tier,
	}

	// configure the infra provider
	var kubernetesRuntimeInfra provider.KubernetesRuntimeInfra
	var threeportAPIEndpoint string
	awsConfig := &aws.Config{}
	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderKind:
		threeportAPIEndpoint = threeport.GetLocalThreeportAPIEndpoint(cpi.Opts.AuthEnabled)

		// construct kind infra provider object
		kubernetesRuntimeInfraKind := provider.KubernetesRuntimeInfraKind{
			RuntimeInstanceName: provider.ThreeportRuntimeName(cpi.Opts.InstanceName),
			KubeconfigPath:      cpi.Opts.KubeconfigPath,
			DevEnvironment:      cpi.Opts.DevEnvironment,
			ThreeportPath:       cpi.Opts.ThreeportPath,
			NumWorkerNodes:      cpi.Opts.NumWorkerNodes,
			AuthEnabled:         cpi.Opts.AuthEnabled,
		}

		// update threerport config
		threeportInstanceConfig.APIServer = threeportAPIEndpoint

		// delete kind kubernetes runtime if interrupted
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			Warning("received Ctrl+C, removing kind kubernetes runtime...")
			// first update the threeport config so the Delete method has
			// something to reference
			config.UpdateThreeportConfig(threeportConfig, threeportInstanceConfig)
			if err := DeleteControlPlane(cpi); err != nil {
				Error("failed to delete kind kubernetes runtime", err)
			}
			os.Exit(1)
		}()

		kubernetesRuntimeInfra = &kubernetesRuntimeInfraKind
	case v0.KubernetesRuntimeInfraProviderEKS:
		// create AWS config
		awsConf, err := resource.LoadAWSConfig(
			cpi.Opts.AwsConfigEnv,
			cpi.Opts.AwsConfigProfile,
			cpi.Opts.AwsRegion,
		)
		if err != nil {
			return fmt.Errorf("failed to load AWS configuration with local config: %w", err)
		}
		awsConfig = awsConf

		// create a resource client to create EKS resources
		resourceClient := resource.CreateResourceClient(awsConfig)

		// capture messages as resources are created and return to user
		go func() {
			for msg := range *resourceClient.MessageChan {
				Info(msg)
			}
		}()

		// capture inventory and write to file as it is created
		go func() {
			for inventory := range *resourceClient.InventoryChan {
				if err := resource.WriteInventory(
					provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.InstanceName),
					&inventory,
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
			time.Sleep(time.Second * 2)
			inventory, err := resource.ReadInventory(
				provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.InstanceName),
			)
			if err != nil {
				Error("failed to read eks kubernetes runtime inventory for resource deletion", err)
			}
			if err = resourceClient.DeleteResourceStack(inventory); err != nil {
				Error("failed to delete eks kubernetes runtime resources", err)
			}
			os.Exit(1)
		}()

		// TODO: add flags to tptctl command for high availability, etc to
		// deterimine these values
		// construct eks kubernetes runtime infra object
		kubernetesRuntimeInfraEKS := provider.KubernetesRuntimeInfraEKS{
			RuntimeInstanceName:          provider.ThreeportRuntimeName(cpi.Opts.InstanceName),
			AwsAccountID:                 cpi.Opts.CreateProviderAccountID,
			AwsConfig:                    awsConfig,
			ResourceClient:               resourceClient,
			ZoneCount:                    int32(2),
			DefaultNodeGroupInstanceType: "t3.medium",
			DefaultNodeGroupInitialNodes: int32(3),
			DefaultNodeGroupMinNodes:     int32(3),
			DefaultNodeGroupMaxNodes:     int32(250),
		}

		// update threeport config
		threeportInstanceConfig.EKSProviderConfig = config.EKSProviderConfig{
			AwsConfigProfile: cpi.Opts.AwsConfigProfile,
			AwsRegion:        cpi.Opts.AwsRegion,
			AwsAccountID:     cpi.Opts.CreateProviderAccountID,
		}

		kubernetesRuntimeInfra = &kubernetesRuntimeInfraEKS
	}

	// create control plane infra
	kubeConnectionInfo, err := kubernetesRuntimeInfra.Create()
	if err != nil {
		msg := "failed to create control plane infra for threeport"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// since we failed to complete kubernetes runtime creation, delete it to
		// prevent dangling runtime resources
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, nil, nil, false, cpi); err != nil {
			return err
		}
		return err
	}
	threeportInstanceConfig.KubeAPI = config.KubeAPI{
		APIEndpoint:   kubeConnectionInfo.APIEndpoint,
		CACertificate: util.Base64Encode(kubeConnectionInfo.CACertificate),
		Certificate:   util.Base64Encode(kubeConnectionInfo.Certificate),
		Key:           util.Base64Encode(kubeConnectionInfo.Key),
		EKSToken:      util.Base64Encode(kubeConnectionInfo.EKSToken),
	}

	// generate encryption key
	encryptionKey, err := encryption.GenerateKey()
	if err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// set encryption key in threeport config
	threeportInstanceConfig.EncryptionKey = encryptionKey
	cpi.Opts.EncryptionKey = encryptionKey

	// the kubernetes runtime instance is the default compute space kubernetes runtime to be added
	// to the API
	kubernetesRuntimeInstName := provider.ThreeportRuntimeName(cpi.Opts.InstanceName)
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
		location, err := mapping.GetLocationForAwsRegion(awsConfig.Region)
		if err != nil {
			msg := fmt.Sprintf("failed to get threeport location for AWS region %s", awsConfig.Region)
			// print the error when it happens and then again post-deletion
			Error(msg, err)
			err = fmt.Errorf("%s: %w", msg, err)
			// delete control plane kubernetes runtime
			if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, nil, nil, false, cpi); err != nil {
				return err
			}
			return err
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
		msg := "failed to get a Kubernetes client and mapper"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, nil, nil, false, cpi); err != nil {
			return err
		}
		return err
	}

	// install the threeport control plane dependencies
	if err := cpi.InstallThreeportControlPlaneDependencies(
		dynamicKubeClient,
		mapper,
		cpi.Opts.InfraProvider,
	); err != nil {
		msg := "failed to install threeport control plane dependencies"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, nil, nil, false, cpi); err != nil {
			return err
		}
		return err
	}

	// if auth is enabled, generate client certificate and add to local config
	var authConfig *auth.AuthConfig
	var clientCredentials *config.Credential
	if cpi.Opts.AuthEnabled {
		// get auth config
		authConfig, err = auth.GetAuthConfig()
		if err != nil {
			msg := "failed to get auth config"
			// print the error when it happens and then again post-deletion
			Error(msg, err)
			err = fmt.Errorf("%s: %w", msg, err)
			// delete control plane kubernetes runtime
			if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, nil, nil, false, cpi); err != nil {
				return err
			}
			return err
		}

		// generate client certificate
		clientCertificate, clientPrivateKey, err := auth.GenerateCertificate(
			authConfig.CAConfig,
			&authConfig.CAPrivateKey,
		)
		if err != nil {
			msg := "failed to generate client certificate and private key"
			// print the error when it happens and then again post-deletion
			Error(msg, err)
			err = fmt.Errorf("%s: %w", msg, err)
			// delete control plane kubernetes runtime
			if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, nil, nil, false, cpi); err != nil {
				return err
			}
			return err
		}

		clientCredentials = &config.Credential{
			Name:       cpi.Opts.InstanceName,
			ClientCert: util.Base64Encode(clientCertificate),
			ClientKey:  util.Base64Encode(clientPrivateKey),
		}

		threeportInstanceConfig.AuthEnabled = true
		threeportInstanceConfig.Credentials = append(threeportInstanceConfig.Credentials, *clientCredentials)
		threeportInstanceConfig.CACert = authConfig.CABase64Encoded

	} else {
		threeportInstanceConfig.AuthEnabled = false
	}

	// update threeport config and refresh threeport config to updated version
	config.UpdateThreeportConfig(threeportConfig, threeportInstanceConfig)
	threeportConfig, _, err = config.GetThreeportConfig("")
	if err != nil {
		msg := "failed to refresh threeport config"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, nil, nil, true, cpi); err != nil {
			return err
		}
		return err
	}

	// get threeport API client
	ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForControlPlane(cpi.Opts.InstanceName)
	if err != nil {
		msg := "failed to get threeport certificates from config"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, nil, nil, true, cpi); err != nil {
			return err
		}
		return err
	}
	apiClient, err := client.GetHTTPClient(cpi.Opts.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
	if err != nil {
		msg := "failed to create http client"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, nil, nil, true, cpi); err != nil {
			return err
		}
		return err
	}

	// for dev environment, build and load dev images for API and controllers
	if cpi.Opts.DevEnvironment {
		if err := tptdev.PrepareDevImages(cpi.Opts.ThreeportPath, provider.ThreeportRuntimeName(cpi.Opts.InstanceName), cpi); err != nil {
			msg := "failed to build and load dev control plane images"
			// print the error when it happens and then again post-deletion
			Error(msg, err)
			err = fmt.Errorf("%s: %w", msg, err)
			// delete control plane kubernetes runtime
			if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, nil, nil, true, cpi); err != nil {
				return err
			}
			return err
		}
	}

	// install the API
	if err := cpi.InstallThreeportAPI(
		dynamicKubeClient,
		mapper,
		cpi.Opts.DevEnvironment,
		authConfig,
		cpi.Opts.InfraProvider,
		encryptionKey,
	); err != nil {
		msg := "failed to install threeport API server"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
			return err
		}
		return err
	}

	// for a cloud provider installed control plane, determine the threeport
	// API's remote endpoint to add to the threeport config and to add to the
	// server certificate's alt names when TLS assets are installed
	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		tpapiEndpoint, err := cpi.GetThreeportAPIEndpoint(dynamicKubeClient, *mapper)
		if err != nil {
			msg := "failed to get threeport API's public endpoint"
			// print the error when it happens and then again post-deletion
			Error(msg, err)
			err = fmt.Errorf("%s: %w", msg, err)
			// delete control plane kubernetes runtime
			if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
				return err
			}
			return err
		}
		threeportAPIEndpoint = tpapiEndpoint
		threeportInstanceConfig.APIServer = fmt.Sprintf("%s:443", threeportAPIEndpoint)
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
			msg := "failed to install threeport API TLS assets"
			// print the error when it happens and then again post-deletion
			Error(msg, err)
			err = fmt.Errorf("%s: %w", msg, err)
			// delete control plane kubernetes runtime
			if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
				return err
			}
			return err
		}
	}

	// wait for API server to start running - it is not strictly necessary to
	// wait for the API before installing the rest of the control plane, however
	// it is helpful for dev environments and harmless otherwise since the
	// controllers need the API to be running in order to start
	Info("Waiting for threeport API to start running...")
	if err := threeport.WaitForThreeportAPI(
		apiClient,
		threeportAPIEndpoint,
	); err != nil {
		msg := "threeport API did not come up"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
			return err
		}
		return err
	}
	Info("Threeport API is running")

	// get a new kubernetes API client to ensure the connection token does not
	// expire
	dynamicKubeClient, mapper, err = kube.GetClient(
		&kubernetesRuntimeInstance,
		false,
		apiClient,
		threeportAPIEndpoint,
		"",
	)
	if err != nil {
		msg := "failed to get a new Kubernetes client and mapper"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, nil, nil, false, cpi); err != nil {
			return err
		}
		return err
	}

	err = cpi.Opts.PreInstallFunction(dynamicKubeClient, mapper, cpi)

	if err != nil {
		msg := "failed to run custom preInstall function"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
			return err
		}
		return err
	}

	// install the controllers
	if err := cpi.InstallThreeportControllers(
		dynamicKubeClient,
		mapper,
		cpi.Opts.DevEnvironment,
		authConfig,
	); err != nil {
		msg := "failed to install threeport controllers"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
			return err
		}
		return err
	}

	err = cpi.Opts.PostInstallFunction(dynamicKubeClient, mapper, cpi)

	if err != nil {
		msg := "failed to run custom postInstall function"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
			return err
		}
		return err
	}

	// install the agent
	if err := cpi.InstallThreeportAgent(
		dynamicKubeClient,
		mapper,
		cpi.Opts.InstanceName,
		cpi.Opts.DevEnvironment,
		authConfig,
	); err != nil {
		msg := "failed to install threeport agent"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
			return err
		}
		return err
	}

	// install support services CRDs
	err = threeport.InstallThreeportCRDs(dynamicKubeClient, mapper)
	if err != nil {
		msg := "failed to install threeport support services CRDs"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
			return err
		}
		return err
	}

	// install the support services operator
	err = threeport.InstallThreeportSupportServicesOperator(dynamicKubeClient, mapper)
	if err != nil {
		msg := "failed to install threeport support services operator"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
			return err
		}
		return err
	}

	// update threeport config and refresh threeport config to updated version
	config.UpdateThreeportConfig(threeportConfig, threeportInstanceConfig)
	threeportConfig, _, err = config.GetThreeportConfig("")
	if err != nil {
		msg := "failed to refresh threeport config"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
			return err
		}
		return err
	}

	// create the default compute space kubernetes runtime definition in threeport API
	kubernetesRuntimeDefName := provider.ThreeportRuntimeName(cpi.Opts.InstanceName)
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
		msg := "failed to create new kubernetes runtime definition for default compute space"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
			return err
		}
		return err
	}

	// create default compute space kubernetes runtime instance in threeport API
	kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID = kubernetesRuntimeDefResult.ID
	kubernetesRuntimeInstResult, err := client.CreateKubernetesRuntimeInstance(
		apiClient,
		threeportAPIEndpoint,
		&kubernetesRuntimeInstance,
	)
	if err != nil {
		msg := "failed to create new kubernetes runtime instance for default compute space"
		// print the error when it happens and then again post-deletion
		Error(msg, err)
		err = fmt.Errorf("%s: %w", msg, err)
		// delete control plane kubernetes runtime
		if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
			return err
		}
		return err
	}

	// for eks clusters:
	// * create aws account
	// * set region in threeport config
	// * create aws eks k8s runtime definition
	// * create aws eks k8s runtime instance
	// * copy aws eks resource inventory to cluster
	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		// create aws account
		accessKeyID, secretAccessKey, err := provider.GetKeysFromLocalConfig(cpi.Opts.AwsConfigProfile)
		if err != nil {
			msg := "failed to get AWS credentials to create default AWS Account"
			// print the error when it happens and then again post-deletion
			Error(msg, err)
			err = fmt.Errorf("%s: %w", msg, err)
			// delete control plane kubernetes runtime
			if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
				return err
			}
			return err
		}
		awsAccountName := "default-account"
		defaultAccount := true

		awsAccount := v0.AwsAccount{
			Name:            &awsAccountName,
			AccountID:       &cpi.Opts.CreateProviderAccountID,
			DefaultAccount:  &defaultAccount,
			DefaultRegion:   &awsConfig.Region,
			AccessKeyID:     &accessKeyID,
			SecretAccessKey: &secretAccessKey,
		}
		createdAwsAccount, err := client.CreateAwsAccount(
			apiClient,
			threeportAPIEndpoint,
			&awsAccount,
		)
		if err != nil {
			msg := "failed to create new default AWS account"
			// print the error when it happens and then again post-deletion
			Error(msg, err)
			err = fmt.Errorf("%s: %w", msg, err)
			// delete control plane kubernetes runtime
			if err := cleanOnCreateError(err, &controlPlane, kubernetesRuntimeInfra, dynamicKubeClient, mapper, true, cpi); err != nil {
				return err
			}
			return err
		}

		// set region in threeport config
		threeportInstanceConfig.EKSProviderConfig.AwsRegion = awsConfig.Region
		config.UpdateThreeportConfig(threeportConfig, threeportInstanceConfig)

		// create aws eks k8s runtime definition
		eksRuntimeDefName := provider.ThreeportRuntimeName(cpi.Opts.InstanceName)
		zoneCount := int(kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraEKS).ZoneCount)
		defaultNodeGroupInitialSize := int(kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraEKS).DefaultNodeGroupInitialNodes)
		defaultNodeGroupMinSize := int(kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraEKS).DefaultNodeGroupMinNodes)
		defaultNodeGroupMaxSize := int(kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraEKS).DefaultNodeGroupMaxNodes)
		awsEksKubernetesRuntimeDef := v0.AwsEksKubernetesRuntimeDefinition{
			Definition: v0.Definition{
				Name: &eksRuntimeDefName,
			},
			AwsAccountID:                  createdAwsAccount.ID,
			ZoneCount:                     &zoneCount,
			DefaultNodeGroupInstanceType:  &kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraEKS).DefaultNodeGroupInstanceType,
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
			return fmt.Errorf("failed to create new AWS EKS kubernetes runtime definition for control plane cluster: %w", err)
		}

		// create aws eks k8s runtime instance
		inventory, err := resource.ReadInventory(
			provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.InstanceName),
		)
		if err != nil {
			return fmt.Errorf("failed to read eks kubernetes runtime inventory for inventory update: %w", err)
		}
		inventoryJSON, err := resource.MarshalInventory(inventory)
		if err != nil {
			return fmt.Errorf("failed to marshal eks kubernetes runtime inventory for inventory update: %w", err)
		}
		dbInventory := datatypes.JSON(inventoryJSON)
		eksRuntimeInstName := provider.ThreeportRuntimeName(cpi.Opts.InstanceName)
		reconciled := true
		awsEksKubernetesRuntimeInstance := v0.AwsEksKubernetesRuntimeInstance{
			Instance: v0.Instance{
				Name: &eksRuntimeInstName,
			},
			Reconciliation: v0.Reconciliation{
				Reconciled: &reconciled,
			},
			Region:                              &awsConfig.Region,
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
			return fmt.Errorf("failed to create new AWS EKS kubernetes runtime instance for control plane cluster: %w", err)
		}
	}

	reconciled := true
	// create control plane definitons and instance for the newly create control plane
	controlPlaneDefinition := v0.ControlPlaneDefinition{
		Definition: v0.Definition{
			Name: &cpi.Opts.InstanceName,
		},
		Reconciliation: v0.Reconciliation{
			Reconciled: &reconciled,
		},
		AuthEnabled: &cpi.Opts.AuthEnabled,
	}
	_, err = client.CreateControlPlaneDefinition(apiClient, threeportAPIEndpoint, &controlPlaneDefinition)
	if err != nil {
		return fmt.Errorf("failed to create control plane definition in threeport API: %w", err)
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
			Name: &cpi.Opts.InstanceName,
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
		CustomInstallInfo:           componentList,
		ControlPlaneDefinitionID:    controlPlaneDefinition.ID,
	}

	// create control plane instance
	_, err = client.CreateControlPlaneInstance(apiClient, threeportAPIEndpoint, &controlPlaneInstance)
	if err != nil {
		return fmt.Errorf("failed to create control plane instance in threeport API: %w", err)
	}

	Info("Threeport control plane installed")
	Info("Threeport config updated")

	Complete(fmt.Sprintf("Threeport instance %s created", cpi.Opts.InstanceName))

	return nil
}

// DeleteControlPlane deletes a threeport control plane.
func DeleteControlPlane(customInstaller *threeport.ControlPlaneInstaller) error {
	// get threeport config
	threeportConfig, _, err := config.GetThreeportConfig("")
	if err != nil {
		return fmt.Errorf("failed to get threeport config: %w", err)
	}

	// configure installer
	cpi := customInstaller
	if customInstaller == nil {
		cpi = threeport.NewInstaller()
	}

	// check threeport config for existing instance
	// find the threeport instance by name
	threeportInstanceConfigExists := false
	var threeportInstanceConfig config.ControlPlane
	for _, instance := range threeportConfig.ControlPlanes {
		if instance.Name == cpi.Opts.InstanceName {
			threeportInstanceConfig = instance
			threeportInstanceConfigExists = true
		}
	}
	if !threeportInstanceConfigExists {
		return errors.New(fmt.Sprintf(
			"config for threeport instance with name %s not found", cpi.Opts.InstanceName,
		))
	}

	var kubernetesRuntimeInfra provider.KubernetesRuntimeInfra
	switch threeportInstanceConfig.Provider {
	case v0.KubernetesRuntimeInfraProviderKind:
		kubernetesRuntimeInfraKind := provider.KubernetesRuntimeInfraKind{
			RuntimeInstanceName: provider.ThreeportRuntimeName(threeportInstanceConfig.Name),
			KubeconfigPath:      cpi.Opts.KubeconfigPath,
		}
		kubernetesRuntimeInfra = &kubernetesRuntimeInfraKind
	case v0.KubernetesRuntimeInfraProviderEKS:
		// create AWS config
		// * AwsConfigEnv is always passed in from CLI args as it is not
		//   persisted in threeport config
		// * AwsConfigProfile and AwsRegion cannot be passed in through CLI for
		// deletion opertion as these are stored in threeport config
		awsConfig, err := resource.LoadAWSConfig(
			cpi.Opts.AwsConfigEnv,
			threeportInstanceConfig.EKSProviderConfig.AwsConfigProfile,
			threeportInstanceConfig.EKSProviderConfig.AwsRegion,
		)
		if err != nil {
			return fmt.Errorf("failed to load AWS configuration with local config: %w", err)
		}

		// create a resource client to create EKS resources
		resourceClient := resource.CreateResourceClient(awsConfig)

		// capture messages as resources are created and return to user
		go func() {
			for msg := range *resourceClient.MessageChan {
				Info(msg)
			}
		}()

		// capture inventory and write to file as it is updated
		go func() {
			for inventory := range *resourceClient.InventoryChan {
				if err := resource.WriteInventory(
					provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.InstanceName),
					&inventory,
				); err != nil {
					Error("failed to write inventory file", err)
				}
			}
		}()

		// read inventory to delete
		inventory, err := resource.ReadInventory(provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.InstanceName))
		if err != nil {
			return fmt.Errorf("failed to read inventory file for deleting eks kubernetes runtime resources: %w", err)
		}

		// construct eks kubernetes runtime infra object
		kubernetesRuntimeInfraEKS := provider.KubernetesRuntimeInfraEKS{
			RuntimeInstanceName: provider.ThreeportRuntimeName(threeportInstanceConfig.Name),
			AwsAccountID:        cpi.Opts.CreateProviderAccountID,
			AwsConfig:           awsConfig,
			ResourceClient:      resourceClient,
			ResourceInventory:   inventory,
		}
		kubernetesRuntimeInfra = &kubernetesRuntimeInfraEKS
	}

	// if provider is EKS we need to delete the threeport API service to
	// remove the AWS load balancer before deleting the rest of the infra and
	// check for existing workload instances that may prevent deletion
	switch threeportInstanceConfig.Provider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForControlPlane(cpi.Opts.InstanceName)
		if err != nil {
			return fmt.Errorf("failed to get threeport certificates from config: %w", err)
		}
		apiClient, err := client.GetHTTPClient(threeportInstanceConfig.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
		if err != nil {
			return fmt.Errorf("failed to create http client: %w", err)
		}

		// check for workload instances on non-kind kubernetes runtimes - halt delete if
		// any are present
		workloadInstances, err := client.GetWorkloadInstances(
			apiClient,
			threeportInstanceConfig.APIServer,
		)
		if err != nil {
			return fmt.Errorf("failed to retrieve workload instances from threeport API: %w", err)
		}
		if len(*workloadInstances) > 0 {
			return errors.New("found workload instances that could prevent control plane deletion - delete all workload instances before deleting control plane")
		}

		// get the kubernetes runtime instance object
		kubernetesRuntimeInstance, err := client.GetThreeportControlPlaneKubernetesRuntimeInstance(
			apiClient,
			threeportInstanceConfig.APIServer,
		)
		if err != nil {
			return fmt.Errorf("failed to retrieve kubernetes runtime instance from threeport API: %w", err)
		}

		// create a client and resource mapper to connect to kubernetes cluster
		// API for deleting resources
		var dynamicKubeClient dynamic.Interface
		var mapper *meta.RESTMapper
		dynamicKubeClient, mapper, err = kube.GetClient(
			kubernetesRuntimeInstance,
			false,
			apiClient,
			threeportInstanceConfig.APIServer,
			threeportInstanceConfig.EncryptionKey,
		)
		if err != nil {
			if kubeerrors.IsUnauthorized(err) {
				// refresh token, save to kubernetes runtime instance and get kube client
				kubeConn, err := kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraEKS).RefreshConnection()
				if err != nil {
					return fmt.Errorf("failed to refresh token to connect to EKS kubernetes runtime: %w", err)
				}

				kubernetesRuntimeInstance.ConnectionToken = &kubeConn.EKSToken
				updatedKubernetesRuntimeInst, err := client.UpdateKubernetesRuntimeInstance(
					apiClient,
					threeportInstanceConfig.APIServer,
					kubernetesRuntimeInstance,
				)
				if err != nil {
					return fmt.Errorf("failed to update EKS token on kubernetes runtime instance: %w", err)
				}
				dynamicKubeClient, mapper, err = kube.GetClient(
					updatedKubernetesRuntimeInst,
					false,
					apiClient,
					threeportInstanceConfig.APIServer,
					threeportInstanceConfig.EncryptionKey,
				)
				if err != nil {
					return fmt.Errorf("failed to get a Kubernetes client and mapper with refreshed token: %w", err)
				}
			} else {
				return fmt.Errorf("failed to get a Kubernetes client and mapper: %w", err)
			}
		}

		// delete threeport API service to remove load balancer
		if err := cpi.UnInstallThreeportControlPlaneComponents(dynamicKubeClient, mapper); err != nil {
			return fmt.Errorf("failed to delete threeport API service: %w", err)
		}
	}

	// delete control plane infra
	if err := kubernetesRuntimeInfra.Delete(); err != nil {
		return fmt.Errorf("failed to delete control plane infra: %w", err)
	}

	switch threeportInstanceConfig.Provider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		// remove inventory file
		invFile := provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.InstanceName)
		if err := os.Remove(invFile); err != nil {
			Warning(fmt.Sprintf("failed to remove inventory file %s", invFile))
		}
	}

	// update threeport config to remove deleted threeport instance
	config.DeleteThreeportConfigControlPlane(threeportConfig, cpi.Opts.InstanceName)
	Info("Threeport config updated")

	Complete(fmt.Sprintf("Threeport instance %s deleted", cpi.Opts.InstanceName))

	return nil
}

// validateCreateControlPlaneFlags validates flag inputs as needed
func ValidateCreateControlPlaneFlags(
	instanceName string,
	infraProvider string,
	createRootDomain string,
	createProviderAccountID string,
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

	// ensure client cert auth is used on remote installations
	if infraProvider != v0.KubernetesRuntimeInfraProviderKind && !authEnabled {
		return errors.New(
			"cannot turn off client certificate authentication unless using the kind provider",
		)
	}

	// ensure that AWS account ID is provided if using EKS provider
	if infraProvider == v0.KubernetesRuntimeInfraProviderEKS && createProviderAccountID == "" {
		return errors.New(
			"your AWS account ID must be provided if deploying using the eks provider",
		)
	}

	return nil
}

// cleanOnCreateError cleans up created infra for a control plane when a
// provisioning error of any kind occurs.
func cleanOnCreateError(
	createErr error,
	controlPlane *threeport.ControlPlane,
	kubernetesRuntimeInfra provider.KubernetesRuntimeInfra,
	dynamicKubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	cleanConfig bool,
	cpi *threeport.ControlPlaneInstaller,
) error {
	// if needed, delete control plane workloads to remove related infra, e.g. load
	// balancers, that will prevent runtime infra deletion
	if dynamicKubeClient != nil && mapper != nil {
		if workloadErr := cpi.UnInstallThreeportControlPlaneComponents(dynamicKubeClient, mapper); workloadErr != nil {
			return fmt.Errorf("failed to create control plane infra for threeport: %w\nfailed to delete threeport API service: %w", createErr, workloadErr)
		}
	}

	// if eks provider, load inventory for deletion
	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		// allow 2 seconds for pending inventory writes to complete
		time.Sleep(time.Second * 2)
		inventory, invErr := resource.ReadInventory(
			provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.InstanceName),
		)
		if invErr != nil {
			return fmt.Errorf("failed to create control plane infra for threeport: %w\nfailed to read eks kubernetes runtime inventory for resource deletion: %w", createErr, invErr)
		}
		kubernetesRuntimeInfra.(*provider.KubernetesRuntimeInfraEKS).ResourceInventory = inventory
	}

	// delete infra
	if deleteErr := kubernetesRuntimeInfra.Delete(); deleteErr != nil {
		return fmt.Errorf("failed to create control plane infra for threeport: %w\nfailed to delete control plane infra, you may have dangling kubernetes runtime infra resources still running: %w", createErr, deleteErr)
	}
	Info("Created Threeport infra deleted due to error")

	switch controlPlane.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		// remove inventory file
		invFile := provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.InstanceName)
		if err := os.Remove(invFile); err != nil {
			Warning(fmt.Sprintf("failed to remove inventory file %s", invFile))
		}
	}

	if cleanConfig {
		threeportConfig, _, configErr := config.GetThreeportConfig("")
		if configErr != nil {
			Warning("Threeport config may contain invalid instance for deleted control plane")
			return fmt.Errorf("failed to create control plane infra for threeport: %w\nfailed to get threeport config: %w", createErr, configErr)
		}
		config.DeleteThreeportConfigControlPlane(threeportConfig, cpi.Opts.InstanceName)
	}

	return nil
}
