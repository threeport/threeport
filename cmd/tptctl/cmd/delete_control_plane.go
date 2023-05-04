/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	configInternal "github.com/threeport/threeport/internal/config"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/threeport"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var deleteThreeportInstanceName string

// DeleteControlPlaneCmd represents the delete control-plane command
var DeleteControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl delete control-plane --name my-threeport",
	Short:        "Delete an instance of the Threeport control plane",
	Long:         `Delete an instance of the Threeport control plane.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config
		threeportConfig, err := configInternal.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to get threeport config", err)
		}

		// check threeport config for exisiting instance
		// find the threeport instance by name
		threeportInstanceConfigExists := false
		var instanceConfig config.Instance
		for _, instance := range threeportConfig.Instances {
			if instance.Name == deleteThreeportInstanceName {
				instanceConfig = instance
				threeportInstanceConfigExists = true
			}
		}
		if !threeportInstanceConfigExists {
			cli.Error("failed to find threeport instance config",
				errors.New(fmt.Sprintf(
					"config for threeport instance with name %s not found", deleteThreeportInstanceName)))
			os.Exit(1)
		}

		var controlPlaneInfra provider.ControlPlaneInfra
		switch instanceConfig.Provider {
		case threeport.ControlPlaneInfraProviderKind:
			controlPlaneInfraKind := provider.ControlPlaneInfraKind{
				ThreeportInstanceName: instanceConfig.Name,
			}
			controlPlaneInfra = &controlPlaneInfraKind
		case threeport.ControlPlaneInfraProviderEKS:
			controlPlaneInfraEKS := provider.ControlPlaneInfraEKS{
				ThreeportInstanceName: instanceConfig.Name,
				AWSConfigEnv:          instanceConfig.EKSProviderConfig.AWSConfigEnv,
				AWSConfigProfile:      instanceConfig.EKSProviderConfig.AWSConfigProfile,
				AWSRegion:             instanceConfig.EKSProviderConfig.AWSRegion,
			}
			controlPlaneInfra = &controlPlaneInfraEKS
		}

		// if provider is EKS we need to delete the threeport API service to
		// remove the AWS load balancer before deleting the rest of the infra
		if instanceConfig.Provider == threeport.ControlPlaneInfraProviderEKS {
			// get the cluster instance object
			clusterInstName := threeport.BootstrapClusterName(deleteThreeportInstanceName)
			clusterInstance, err := GetClusterInstanceByName(clusterInstName, instanceConfig.APIServer, "")
			if err != nil {
				cli.Error("failed to retrieve cluster instance from threeport API", err)
				os.Exit(1)
			}

			// create a client and resource mapper to connect to kubernetes cluster
			// API for deleting resources
			dynamicKubeClient, mapper, err := kube.GetClient(&clusterInstance, false)
			if err != nil {
				cli.Error("failed to get a Kubernetes client and mapper", err)
				os.Exit(1)
			}

			// delete threeport API service to remove load balancer
			if err := UnInstallThreeportControlPlaneComponents(dynamicKubeClient, mapper); err != nil {
				cli.Error("failed delete threeport API service", err)
				os.Exit(1)
			}
		}

		// delete control plane infra
		if err := controlPlaneInfra.Delete(providerConfigDir); err != nil {
			cli.Error("failed to delete control plane infra", err)
			os.Exit(1)
		}

		configInternal.DeleteThreeportConfigInstance(threeportConfig, deleteThreeportInstanceName)

		cli.Info("threeport config updated")

		cli.Complete(fmt.Sprintf("threeport instance %s deleted", deleteThreeportInstanceName))
	},
}

func init() {
	deleteCmd.AddCommand(DeleteControlPlaneCmd)

	DeleteControlPlaneCmd.Flags().StringVarP(
		&deleteThreeportInstanceName,
		"name", "n", "", "Required. Name of control plane instance.",
	)
	DeleteControlPlaneCmd.MarkFlagRequired("name")
}
