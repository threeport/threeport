/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/threeport"
	"github.com/threeport/threeport/internal/tptdev"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
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
	Run: func(cmd *cobra.Command, args []string) {
		// get default kubeconfig if not provided
		if createKubeconfig == "" {
			ck, err := kube.DefaultKubeconfig()
			if err != nil {
				cli.Error("failed to get path to default kubeconfig", err)
				os.Exit(1)
			}
			createKubeconfig = ck
		}

		// set default threeport repo path if not provided
		// this is needed to map the container path to the host path for live
		// reloads of the code
		if threeportPath == "" {
			tp, err := os.Getwd()
			if err != nil {
				cli.Error("failed to get current working directory", err)
				os.Exit(1)
			}
			threeportPath = tp
		}

		// create kind cluster
		controlPlaneInfra := provider.ControlPlaneInfraKind{
			ThreeportInstanceName: createThreeportDevName,
			KubeconfigPath:        createKubeconfig,
			ThreeportPath:         threeportPath,
		}
		devEnvironment := true
		kindConfig := controlPlaneInfra.GetKindConfig(devEnvironment)
		controlPlaneInfra.KindConfig = kindConfig
		kubeConnectionInfo, err := controlPlaneInfra.Create()
		if err != nil {
			cli.Error("failed to create kind cluster", err)
			os.Exit(1)
		}

		// the cluster instance is the default compute space cluster to be added
		// to the API - it is used to kube client for creating control plane
		// resources
		clusterInstName := fmt.Sprintf("%s-compute-space-0", createThreeportDevName)
		controlPlaneCluster := true
		clusterInstance := v0.ClusterInstance{
			Instance: v0.Instance{
				Name: &clusterInstName,
			},
			ThreeportControlPlaneCluster: &controlPlaneCluster,
			APIEndpoint:                  &kubeConnectionInfo.APIEndpoint,
			CACertificate:                &kubeConnectionInfo.CACertificate,
			Certificate:                  &kubeConnectionInfo.Certificate,
			Key:                          &kubeConnectionInfo.Key,
		}

		// create a client to connect to kind cluster kube API
		dynamicKubeClient, mapper, err := kube.GetClient(&clusterInstance, false)
		if err != nil {
			cli.Error("failed to get a Kubernetes client and mapper", err)
			os.Exit(1)
		}

		// generate certificate authority for the threeport API
		caConfig, ca, caPrivateKey, err := threeport.GenerateCACertificate()
		if err != nil {
			cli.Error("failed to generate certificate authority and private key", err)
			os.Exit(1)
		}

		// generate server certificate
		serverCertificate, serverPrivateKey, err := threeport.GenerateCertificate(caConfig, caPrivateKey)
		if err != nil {
			cli.Error("failed to generate server certificate and private key", err)
			os.Exit(1)
		}

		// generate client certificate
		clientCertificate, clientPrivateKey, err := threeport.GenerateCertificate(caConfig, caPrivateKey)
		if err != nil {
			cli.Error("failed to generate client certificate and private key", err)
			os.Exit(1)
		}

		// write client certificate and private key to config directory
		clientCertificateEncoded := threeport.GetPEMEncoding(clientCertificate, "CERTIFICATE")
		clientPrivateKeyEncoded := threeport.GetPEMEncoding(x509.MarshalPKCS1PrivateKey(clientPrivateKey), "RSA PRIVATE KEY")
		caEncoded := threeport.GetPEMEncoding(ca, "CERTIFICATE")

		// Set the path to the directory and files
		dirPath := filepath.Join(os.Getenv("HOME"), ".threeport")
		certPath := filepath.Join(dirPath, "tls.crt")
		keyPath := filepath.Join(dirPath, "tls.key")
		caCertPath := filepath.Join(dirPath, "ca.crt")

		// Ensure that the directory exists
		if err := os.MkdirAll(dirPath, 0700); err != nil {
			fmt.Printf("Error creating directory: %v\n", err)
			return
		}

		// Create or overwrite the certificate file and write the client certificate to it
		certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			fmt.Printf("Error opening certificate file: %v\n", err)
			return
		}
		defer certFile.Close()
		if _, err := certFile.WriteString(clientCertificateEncoded); err != nil {
			fmt.Printf("Error writing to certificate file: %v\n", err)
			return
		}

		// Create or overwrite the key file and write the client private key to it
		keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			fmt.Printf("Error opening key file: %v\n", err)
			return
		}
		defer keyFile.Close()
		if _, err := keyFile.WriteString(clientPrivateKeyEncoded); err != nil {
			fmt.Printf("Error writing to key file: %v\n", err)
			return
		}

		// Create or overwrite the ca certificate file and write the ca certificate to it
		caCertFile, err := os.OpenFile(caCertPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			fmt.Printf("Error opening ca cert file: %v\n", err)
			return
		}
		defer caCertFile.Close()
		if _, err := caCertFile.WriteString(caEncoded); err != nil {
			fmt.Printf("Error writing to key file: %v\n", err)
			return
		}

		// install the threeport control plane dependencies
		if err := threeport.InstallThreeportControlPlaneDependencies(dynamicKubeClient, mapper); err != nil {
			cli.Error("failed to install threeport control plane dependencies", err)
			os.Exit(1)
		}

		// build and load dev images for API and controllers
		if err := tptdev.PrepareDevImages(threeportPath, provider.ThreeportClusterName(createThreeportDevName)); err != nil {
			cli.Error("failed to build and load dev control plane images", err)
			os.Exit(1)
		}

		// get PEM-encoded keypairs as strings to pass into deployment manifests
		caPrivateKeyEncoded := threeport.GetPEMEncoding(x509.MarshalPKCS1PrivateKey(caPrivateKey), "RSA PRIVATE KEY")
		serverCertificateEncoded := threeport.GetPEMEncoding(serverCertificate, "CERTIFICATE")
		serverPrivateKeyEncoded := threeport.GetPEMEncoding(x509.MarshalPKCS1PrivateKey(serverPrivateKey), "RSA PRIVATE KEY")

		// install the threeport control plane API and controllers
		//if err := threeport.InstallThreeportControlPlaneComponents(
		if err := threeport.InstallThreeportAPI(
			dynamicKubeClient,
			mapper,
			true,
			threeport.ThreeportLocalAPIEndpoint,
			"",
			caEncoded,
			caPrivateKeyEncoded,
			serverCertificateEncoded,
			serverPrivateKeyEncoded,
		); err != nil {
			cli.Error("failed to install threeport control plane components", err)
			os.Exit(1)
		}

		// wait for API server to start running
		cli.Info("waiting for threeport API to start running")
		if err := threeport.WaitForThreeportAPI(
			fmt.Sprintf("https://%s:1323", threeport.ThreeportLocalAPIEndpoint),
		); err != nil {
			cli.Error("threeport API did not come up", err)
			os.Exit(1)
		}

		// install the threeport controllers - these need to be installed once
		// API server is running in dev environment because the air entrypoint
		// prevents the controllers from crashlooping if they come up before
		// the API server
		if err := threeport.InstallThreeportControllers(
			dynamicKubeClient,
			mapper,
			true,
			"",
			caEncoded,
			clientCertificateEncoded,
			clientPrivateKeyEncoded,
		); err != nil {
			cli.Error("failed to install threeport control plane components", err)
			os.Exit(1)
		}

		// create the default compute space cluster definition in threeport API
		clusterDefName := fmt.Sprintf("compute-space-%s", createThreeportDevName)
		clusterDefinition := v0.ClusterDefinition{
			Definition: v0.Definition{
				Name: &clusterDefName,
			},
		}
		clusterDefResult, err := client.CreateClusterDefinition(
			&clusterDefinition,
			fmt.Sprintf("%s://%s:%s", threeport.ThreeportLocalAPIProtocol, threeport.ThreeportLocalAPIEndpoint, threeport.ThreeportLocalAPIPort),
		)
		if err != nil {
			cli.Error("failed to create new cluster definition for default compute space", err)
			os.Exit(1)
		}

		// create default compute space cluster instance in threeport API
		clusterInstance.ClusterDefinitionID = clusterDefResult.ID
		_, err = client.CreateClusterInstance(
			&clusterInstance,
			fmt.Sprintf("%s://%s:%s", threeport.ThreeportLocalAPIProtocol, threeport.ThreeportLocalAPIEndpoint, threeport.ThreeportLocalAPIPort),
		)
		if err != nil {
			cli.Error("failed to create new cluster instance for default compute space", err)
			os.Exit(1)
		}

		// wait for 20 seconds to allow time for controllers to build and start
		// so that when this command returns, the control plane is ready
		time.Sleep(time.Second * 20)

		cli.Complete(fmt.Sprintf("threeport dev instance %s created", createThreeportDevName))
	},
}

func init() {
	rootCmd.AddCommand(upCmd)

	upCmd.Flags().StringVarP(&createThreeportDevName,
		"name", "n", tptdev.DefaultInstanceName, "name of dev control plane instance")
	upCmd.Flags().StringVarP(&createKubeconfig,
		"kubeconfig", "k", "", "path to kubeconfig - default is ~/.kube/config")
	upCmd.Flags().StringVarP(&threeportPath,
		"threeport-path", "t", "", "path to threeport repository root - default is ./")
}
