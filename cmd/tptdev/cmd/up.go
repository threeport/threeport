/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/provider/kind"
	"github.com/threeport/threeport/internal/threeport"
	"github.com/threeport/threeport/internal/tptctl/output"
	"github.com/threeport/threeport/internal/tptdev"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

var (
	createThreeportDevName string
	createKubeconfig       string
	threeportPath          string
)

// upCmd represents the up command
var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Spin up a new threeport development environment",
	Long:  `Spin up a new threeport development environment.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// get default kubeconfig if not provided
		if createKubeconfig == "" {
			ck, err := defaultKubeconfig()
			if err != nil {
				return fmt.Errorf("failed to get path to default kubeconfig: %w", err)
			}
			createKubeconfig = ck
		}

		// set default threeport repo path if not provided
		// this is needed to map the container path to the host path for live
		// reloads of the code
		if threeportPath == "" {
			tp, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current working directory: %w", err)
			}
			threeportPath = tp
		}

		// create kind cluster
		kubeConnectionInfo, err := kind.CreateKindDevCluster(
			kindClusterName(createThreeportDevName),
			createKubeconfig,
			threeportPath,
		)
		if err != nil {
			return fmt.Errorf("failed to create kind cluster: %w", err)
		}

		// create cluster definition and instance objects
		//clusterDefName := fmt.Sprintf("compute-space-%s", createThreeportDevName)
		//clusterDefinition := v0.ClusterDefinition{
		//	Definition: v0.Definition{
		//		Name: &clusterDefName,
		//	},
		//}
		clusterInstName := fmt.Sprintf("compute-space-%s-0", createThreeportDevName)
		clusterInstance := v0.ClusterInstance{
			Instance: v0.Instance{
				Name: &clusterInstName,
			},
			APIEndpoint:   &kubeConnectionInfo.APIEndpoint,
			CACertificate: &kubeConnectionInfo.CACertificate,
			Certificate:   &kubeConnectionInfo.Certificate,
			Key:           &kubeConnectionInfo.Key,
		}

		// create a client to connect to kind cluster kube API
		dynamicKubeClient, mapper, err := kube.GetClient(&clusterInstance)
		if err != nil {
			return fmt.Errorf("failed to get a Kubernetes client and mapper: %w", err)
		}

		// install the threeport control plane dependencies
		if err := threeport.InstallThreeportControlPlaneDependencies(dynamicKubeClient, mapper); err != nil {
			return fmt.Errorf("failed to install threeport control plane dependencies: %w", err)
		}

		// build and load dev images
		if err := tptdev.PrepareDevImages(threeportPath, kindClusterName(createThreeportDevName)); err != nil {
			return fmt.Errorf("failed to build and load dev control plane images: %w", err)
		}

		// install the threeport control plane API and controllers
		if err := threeport.InstallThreeportControlPlaneComponents(dynamicKubeClient, mapper); err != nil {
			return fmt.Errorf("failed to install threeport control plane components: %w", err)
		}

		output.Complete(fmt.Sprintf("Threeport dev instance %s created", createThreeportDevName))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(upCmd)

	upCmd.Flags().StringVarP(&createThreeportDevName,
		"name", "n", defaultDevName, "name of dev control plane instance")
	upCmd.Flags().StringVarP(&createKubeconfig,
		"kubeconfig", "k", "", "path to kubeconfig - default is ~/.kube/config")
	upCmd.Flags().StringVarP(&threeportPath,
		"threeport-path", "t", "", "path to threeport repository root - default is ./")
}
