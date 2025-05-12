package v0

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	builder_client "github.com/nukleros/aws-builder/pkg/client"
	builder_config "github.com/nukleros/aws-builder/pkg/config"
	"github.com/nukleros/aws-builder/pkg/eks"
	builder_iam "github.com/nukleros/aws-builder/pkg/iam"
	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
)

// DeployEksInfra deploys the EKS infrastructure for the control plane.
func DeployEksInfra(
	cpi *threeport.ControlPlaneInstaller,
	threeportControlPlaneConfig *config.ControlPlane,
	threeportConfig *config.ThreeportConfig,
	kubernetesRuntimeInfra *provider.KubernetesRuntimeInfra,
	kubeConnectionInfo *kube.KubeConnectionInfo,
	uninstaller *Uninstaller,
	awsConfigUser *aws.Config,
	callerIdentity *sts.GetCallerIdentityOutput,
	awsConfigResourceManager *aws.Config,
) error {
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
	awsConfigUser = awsConf

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
			cpi.Opts.Namespace,
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
			*awsConfigUser,
			cpi.Opts.AdditionalAwsIrsaConditions,
		)
		if err != nil {
			deleteErr := provider.DeleteResourceManagerRole(cpi.Opts.ControlPlaneName, *awsConfigUser)
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
		*awsConfigUser,
		[]func(*aws_config.LoadOptions) error{
			aws_config.WithRegion(cpi.Opts.AwsRegion),
		},
	)
	if err != nil {
		deleteErr := provider.DeleteResourceManagerRole(cpi.Opts.ControlPlaneName, *awsConfigUser)
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
			deleteErr := provider.DeleteResourceManagerRole(cpi.Opts.ControlPlaneName, *awsConfigUser)
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
	*kubernetesRuntimeInfra = &kubernetesRuntimeInfraEKS
	uninstaller.kubernetesRuntimeInfra = *kubernetesRuntimeInfra

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

	if cpi.Opts.ControlPlaneOnly {
		kubeConnectionInfo, err = kubernetesRuntimeInfraEKS.GetConnection()
		if err != nil {
			return fmt.Errorf("failed to get connection info for eks kubernetes runtime: %w", err)
		}
	} else {
		kubeConnectionInfo, err = (*kubernetesRuntimeInfra).Create()
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to create control plane infra for threeport", err)
		}
	}
	return nil
}

// ConfigureEksKubernetesRuntimeInstance configures the kubernetes runtime instance for the eks provider.
func ConfigureEksKubernetesRuntimeInstance(
	cpi *threeport.ControlPlaneInstaller,
	kubeConnectionInfo *kube.KubeConnectionInfo,
	uninstaller *Uninstaller,
	awsConfigUser *aws.Config,
	callerIdentity *sts.GetCallerIdentityOutput,
	awsConfigResourceManager *aws.Config,
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	kubernetesRuntimeInstName string,
	instReconciled bool,
	controlPlaneHost bool,
	defaultRuntime bool,
) error {
	var err error

	// update resource manager role to allow pods to assume it
	var inventory eks.EksInventory
	if err := inventory.Load(
		provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.ControlPlaneName),
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to read eks kubernetes runtime inventory for inventory update", err)
	}
	if err = provider.UpdateResourceManagerRoleTrustPolicy(
		cpi.Opts.Namespace,
		cpi.Opts.ControlPlaneName,
		*callerIdentity.Account,
		"",
		inventory.Cluster.OidcProviderUrl,
		*awsConfigUser,
		cpi.Opts.AdditionalAwsIrsaConditions,
	); err != nil {
		return uninstaller.cleanOnCreateError("failed to update resource manager role", err)
	}

	location, err := mapping.GetLocationForAwsRegion(awsConfigResourceManager.Region)
	if err != nil {
		return uninstaller.cleanOnCreateError(fmt.Sprintf("failed to get threeport location for AWS region %s", awsConfigResourceManager.Region), err)
	}

	kubernetesRuntimeInstance = &v0.KubernetesRuntimeInstance{
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
		ConnectionToken:           &kubeConnectionInfo.Token,
		ConnectionTokenExpiration: &kubeConnectionInfo.TokenExpiration,
		DefaultRuntime:            &defaultRuntime,
	}

	return nil
}

// InstallEksKubernetesResources installs the kubernetes resources for the eks provider.
func InstallEksKubernetesResources(
	cpi *threeport.ControlPlaneInstaller,
	uninstaller *Uninstaller,
	callerIdentity *sts.GetCallerIdentityOutput,
	dynamicKubeClient *dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
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
		if err := cpi.CreateOrUpdateKubeResource(serviceAccount, *dynamicKubeClient, mapper); err != nil {
			return uninstaller.cleanOnCreateError("failed to get threeport API's public endpoint", err)
		}
	}
	return nil
}
