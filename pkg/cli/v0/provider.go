package v0

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	aws_config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	builder_client "github.com/nukleros/aws-builder/pkg/client"
	builder_config "github.com/nukleros/aws-builder/pkg/config"
	"github.com/nukleros/aws-builder/pkg/eks"
	builder_iam "github.com/nukleros/aws-builder/pkg/iam"
	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GetKubernetesRuntimeInfra returns the appropriate Kubernetes runtime infrastructure
// based on the provider type and configuration.
func GetKubernetesRuntimeInfra(
	cpi *threeport.ControlPlaneInstaller,
	threeportConfig *config.ThreeportConfig,
	threeportControlPlaneConfig *config.ControlPlane,
	uninstaller *Uninstaller,
) (provider.KubernetesRuntimeInfra, *kube.KubeConnectionInfo, *aws.Config, *aws.Config, *sts.GetCallerIdentityOutput, error) {
	var kubernetesRuntimeInfra provider.KubernetesRuntimeInfra
	var kubeConnectionInfo *kube.KubeConnectionInfo
	var awsConfigUser aws.Config
	var awsConfigResourceManager *aws.Config
	var callerIdentity *sts.GetCallerIdentityOutput
	var err error

	switch threeportControlPlaneConfig.Provider {
	case v0.KubernetesRuntimeInfraProviderKind:
		// Handle Kind provider setup
		portForwards := make(map[int32]int32)
		for _, mapping := range cpi.Opts.KindInfraPortForward {
			split := strings.Split(mapping, ":")
			if len(split) != 2 {
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to parse kind port forward %s", mapping)
			}

			containerPort, err := strconv.ParseInt(split[0], 10, 32)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to parse container port: %s as int32", split[0])
			}

			hostPort, err := strconv.ParseInt(split[1], 10, 32)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to parse host port: %s as int32", split[0])
			}

			portForwards[int32(containerPort)] = int32(hostPort)
		}

		kubernetesRuntimeInfraKind := provider.KubernetesRuntimeInfraKind{
			RuntimeInstanceName: provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName),
			KubeconfigPath:      cpi.Opts.KubeconfigPath,
			DevEnvironment:      cpi.Opts.DevEnvironment,
			ThreeportPath:       cpi.Opts.ThreeportPath,
			NumWorkerNodes:      cpi.Opts.NumWorkerNodes,
			AuthEnabled:         cpi.Opts.AuthEnabled,
			PortForwards:        portForwards,
		}

		kubernetesRuntimeInfra = &kubernetesRuntimeInfraKind
		uninstaller.kubernetesRuntimeInfra = kubernetesRuntimeInfra

		if cpi.Opts.ControlPlaneOnly {
			kubeConnectionInfo, err = kube.GetConnectionInfoFromKubeconfig(kubernetesRuntimeInfraKind.KubeconfigPath)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to get connection info for kind kubernetes runtime: %w", err)
			}
		} else {
			kubeConnectionInfo, err = kubernetesRuntimeInfra.Create()
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to create control plane infra for threeport: %w", err)
			}
		}

	case v0.KubernetesRuntimeInfraProviderEKS:
		// Handle EKS provider setup
		awsConf, err := builder_config.LoadAWSConfig(
			cpi.Opts.AwsConfigEnv,
			cpi.Opts.AwsConfigProfile,
			cpi.Opts.AwsRegion,
			"",
			"",
			"",
		)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("failed to load AWS configuration with local config: %w", err)
		}
		awsConfigUser = *awsConf

		if callerIdentity, err = provider.GetCallerIdentity(awsConf); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("failed to get caller identity: %w", err)
		}
		Info(fmt.Sprintf("Successfully authenticated to account %s as %s", *callerIdentity.Account, *callerIdentity.Arn))

		if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {
			c.EKSProviderConfig = config.EKSProviderConfig{
				AwsConfigProfile: cpi.Opts.AwsConfigProfile,
				AwsRegion:        cpi.Opts.AwsRegion,
				AwsAccountID:     *callerIdentity.Account,
			}
		}); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("failed to update threeport config: %w", err)
		}

		if !cpi.Opts.ControlPlaneOnly {
			Info("Creating Threeport IAM role")

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
				awsConfigUser,
				cpi.Opts.AdditionalAwsIrsaConditions,
			)
			if err != nil {
				deleteErr := provider.DeleteResourceManagerRole(cpi.Opts.ControlPlaneName, awsConfigUser)
				if deleteErr != nil {
					return nil, nil, nil, nil, nil, fmt.Errorf("failed to create runtime manager role: %w, failed to delete IAM resources: %w", err, deleteErr)
				}
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to create runtime manager role: %w", err)
			}
		}

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
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to assume role for AWS resource manager: %w, failed to delete IAM resources: %w", err, deleteErr)
			}
			return nil, nil, nil, nil, nil, fmt.Errorf("failed to assume role for AWS resource manager: %w", err)
		}

		if !cpi.Opts.ControlPlaneOnly {
			Info("Waiting for IAM role to become available...")
			if err = util.Retry(30, 1, func() error {
				if callerIdentity, err = provider.GetCallerIdentity(awsConfigResourceManager); err != nil {
					return fmt.Errorf("failed to get caller identity: %w", err)
				}
				Info(fmt.Sprintf("Successfully authenticated to account %s as %s", *callerIdentity.Account, *callerIdentity.Arn))

				time.Sleep(time.Second * 5)

				Info("IAM resources created")
				return nil
			}); err != nil {
				deleteErr := provider.DeleteResourceManagerRole(cpi.Opts.ControlPlaneName, awsConfigUser)
				if deleteErr != nil {
					return nil, nil, nil, nil, nil, fmt.Errorf("failed to wait for IAM resources to be available: %w, failed to delete IAM resources: %w", err, deleteErr)
				}
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to wait for IAM resources to be available: %w", err)
			}
		}

		eksInventoryChan := make(chan eks.EksInventory)
		eksClient := eks.EksClient{
			*builder_client.CreateResourceClient(awsConfigResourceManager),
			&eksInventoryChan,
		}

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
		uninstaller.kubernetesRuntimeInfra = kubernetesRuntimeInfra

		go func() {
			for msg := range *eksClient.MessageChan {
				Info(msg)
			}
		}()

		go func() {
			for inventory := range *eksClient.InventoryChan {
				if err := inventory.Write(
					provider.EKSInventoryFilepath(cpi.Opts.ProviderConfigDir, cpi.Opts.ControlPlaneName),
				); err != nil {
					Error("failed to write inventory file", err)
				}
			}
		}()

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			Warning("received Ctrl+C, cleaning up resources...")
			time.Sleep(time.Duration(2) * time.Second)
			if err := uninstaller.cleanOnCreateError("", nil); err != nil {
				Error("failed to clean up resources: ", err)
			}
			os.Exit(1)
		}()

		if cpi.Opts.ControlPlaneOnly {
			kubeConnectionInfo, err = kubernetesRuntimeInfraEKS.GetConnection()
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to get connection info for eks kubernetes runtime: %w", err)
			}
		} else {
			kubeConnectionInfo, err = kubernetesRuntimeInfra.Create()
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to create control plane infra for threeport: %w", err)
			}
		}

	case v0.KubernetesRuntimeInfraProviderOKE:
		// Handle OKE provider setup
		kubernetesRuntimeInfraOKE := provider.KubernetesRuntimeInfraOKE{
			RuntimeInstanceName:     provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName),
			TenancyID:               cpi.Opts.OracleTenancyID,
			CompartmentID:           cpi.Opts.OracleCompartmentID,
			Region:                  cpi.Opts.OracleRegion,
			AvailabilityDomainCount: cpi.Opts.OracleAvailabilityDomainCount,
			WorkerNodeShape:         cpi.Opts.OracleWorkerNodeShape,
			WorkerNodeInitialCount:  cpi.Opts.OracleWorkerNodeInitialCount,
			WorkerNodeMinCount:      cpi.Opts.OracleWorkerNodeMinCount,
			WorkerNodeMaxCount:      cpi.Opts.OracleWorkerNodeMaxCount,
		}
		kubernetesRuntimeInfra = &kubernetesRuntimeInfraOKE
		uninstaller.kubernetesRuntimeInfra = kubernetesRuntimeInfra

		if cpi.Opts.ControlPlaneOnly {
			kubeConnectionInfo, err = kubernetesRuntimeInfraOKE.GetConnection()
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to get connection info for OKE kubernetes runtime: %w", err)
			}
		} else {
			kubeConnectionInfo, err = kubernetesRuntimeInfra.Create()
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to create control plane infra for threeport: %w", err)
			}
		}
	}

	return kubernetesRuntimeInfra, kubeConnectionInfo, &awsConfigUser, awsConfigResourceManager, callerIdentity, nil
}
