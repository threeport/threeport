/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	util "github.com/threeport/threeport/pkg/util/v0"
)

var (
	getCredsThreeportName string
	getCredsOutputDir     string
)

// getCredsCmd represents the get-creds command
var getCredsCmd = &cobra.Command{
	Use:   "get-creds",
	Short: "Get user client client cert, key and server CA for threeport instance API",
	Long:  `Get user client client cert, key and server CA for threeport instance API.`,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config
		threeportConfig, _, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		var threeportInstanceConfig config.ControlPlane
		instanceConfigFound := false
		for i, instance := range threeportConfig.ControlPlanes {
			if instance.Name == getCredsThreeportName {
				threeportInstanceConfig = threeportConfig.ControlPlanes[i]
				instanceConfigFound = true
			}
		}
		if !instanceConfigFound {
			msg := fmt.Sprintf(
				"failed to find threeport instance with name %s in threeport config",
				getCredsThreeportName,
			)
			cli.Error(msg, errors.New("threeport instance not found"))
			os.Exit(1)
		}

		// extract values from threeport config
		caCertEncoded := threeportInstanceConfig.CACert
		var certEncoded string
		var keyEncoded string
		credsFound := false
		for _, creds := range threeportInstanceConfig.Credentials {
			if creds.Name == getCredsThreeportName {
				certEncoded = creds.ClientCert
				keyEncoded = creds.ClientKey
				credsFound = true
			}
		}
		if !credsFound {
			msg := fmt.Sprintf(
				"failed to find credentials for threeport instance with name %s in threeport config",
				getCredsThreeportName,
			)
			cli.Error(msg, errors.New("threeport instance credentials not found"))
			os.Exit(1)
		}

		// decode values
		caCert, err := util.Base64Decode(caCertEncoded)
		if err != nil {
			cli.Error("failed to decode threeport API server CA", err)
			os.Exit(1)
		}
		cert, err := util.Base64Decode(certEncoded)
		if err != nil {
			cli.Error("failed to decode client cert", err)
			os.Exit(1)
		}
		key, err := util.Base64Decode(keyEncoded)
		if err != nil {
			cli.Error("failed to decode client key", err)
			os.Exit(1)
		}

		// determine output dir
		var outputDir string
		if getCredsOutputDir == "" {
			currentDir, err := os.Getwd()
			if err != nil {
				cli.Error("failed to get working directory for output", err)
				os.Exit(1)
			}
			outputDir = currentDir
		} else {
			outputDir = getCredsOutputDir
		}

		// write files
		caCertFile := filepath.Join(outputDir, fmt.Sprintf("%s-ca.crt", getCredsThreeportName))
		if err := os.WriteFile(caCertFile, []byte(caCert), 0644); err != nil {
			cli.Error("failed to write threeport API CA to file", err)
			os.Exit(1)
		}
		cli.Info(fmt.Sprintf("CA cert written to %s", caCertFile))

		certFile := filepath.Join(outputDir, fmt.Sprintf("%s-client.crt", getCredsThreeportName))
		if err := os.WriteFile(certFile, []byte(cert), 0644); err != nil {
			cli.Error("failed to write client certificate to file", err)
			os.Exit(1)
		}
		cli.Info(fmt.Sprintf("client cert written to %s", certFile))

		keyFile := filepath.Join(outputDir, fmt.Sprintf("%s-client.key", getCredsThreeportName))
		if err := os.WriteFile(keyFile, []byte(key), 0600); err != nil {
			cli.Error("failed to write client key to file", err)
			os.Exit(1)
		}
		cli.Info(fmt.Sprintf("client key written to %s", keyFile))

		cli.Complete(fmt.Sprintf("credentials for threeport instance %s written", getCredsThreeportName))
	},
}

func init() {
	rootCmd.AddCommand(getCredsCmd)

	getCredsCmd.Flags().StringVarP(
		&getCredsThreeportName,
		"name", "n", tptdev.DefaultInstanceName, "name of threeport instance",
	)
	getCredsCmd.Flags().StringVarP(
		&getCredsOutputDir,
		"output-dir", "o", "", "directory to write client cert, key and CA cert files to",
	)
}
