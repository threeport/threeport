/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/threeport"
	client "github.com/threeport/threeport/pkg/client/v0"
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
		threeportConfig, err := config.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
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
		// check for existing workload instances that may prevent deletion and
		// remove the AWS load balancer before deleting the rest of the infra
		if instanceConfig.Provider == threeport.ControlPlaneInfraProviderEKS {
			ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificates()
			if err != nil {
				cli.Error("failed to get threeport certificates from config", err)
				os.Exit(1)
			}
			apiClient, err := client.GetHTTPClient(instanceConfig.AuthEnabled, ca, clientCertificate, clientPrivateKey)
			if err != nil {
				cli.Error("failed to create http client", err)
				os.Exit(1)
			}

			// check for workload instances on non-kind clusters - halt delete if
			// any are present
			workloadInstances, err := client.GetWorkloadInstances(
				apiClient,
				instanceConfig.APIServer,
			)
			if err != nil {
				cli.Error("failed to retrieve workload instances from threeport API", err)
				os.Exit(1)
			}
			if len(*workloadInstances) > 0 {
				cli.Error(
					"found workload instances that could prevent control plane deletion - delete all workload instances before deleting control plane",
					errors.New("one or more workload instances found"),
				)
				os.Exit(1)
			}

			// get the cluster instance object
			clusterInstance, err := client.GetThreeportControlPlaneClusterInstance(
				apiClient,
				instanceConfig.APIServer,
			)
			if err != nil {
				cli.Error("failed to retrieve cluster instance from threeport API", err)
				os.Exit(1)
			}

			// create a client and resource mapper to connect to kubernetes cluster
			// API for deleting resources
			var dynamicKubeClient dynamic.Interface
			var mapper *meta.RESTMapper
			dynamicKubeClient, mapper, err = kube.GetClient(clusterInstance, false)
			if err != nil {
				if kubeerrors.IsUnauthorized(err) {
					// refresh token, save to cluster instance and get kube client
					kubeConn, err := controlPlaneInfra.(*provider.ControlPlaneInfraEKS).RefreshConnection()
					if err != nil {
						cli.Error("failed to refresh token to connect to EKS cluster", err)
						os.Exit(1)
					}
					clusterInstance.EKSToken = &kubeConn.EKSToken
					updatedClusterInst, err := client.UpdateClusterInstance(
						apiClient,
						instanceConfig.APIServer,
						clusterInstance,
					)
					if err != nil {
						cli.Error("failed to update EKS token on cluster instance", err)
						os.Exit(1)
					}
					dynamicKubeClient, mapper, err = kube.GetClient(updatedClusterInst, false)
					if err != nil {
						cli.Error("failed to get a Kubernetes client and mapper with refreshed token", err)
						os.Exit(1)
					}
				} else {
					cli.Error("failed to get a Kubernetes client and mapper", err)
					os.Exit(1)
				}
			}

			// delete threeport API service to remove load balancer
			if err := threeport.UnInstallThreeportControlPlaneComponents(dynamicKubeClient, mapper); err != nil {
				cli.Error("failed to delete threeport API service", err)
				os.Exit(1)
			}
		}

		// delete control plane infra
		if err := controlPlaneInfra.Delete(providerConfigDir); err != nil {
			cli.Error("failed to delete control plane infra", err)
			os.Exit(1)
		}

		// update threeport config to remove deleted threeport instance
		config.DeleteThreeportConfigInstance(threeportConfig, deleteThreeportInstanceName)
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
