/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// DownCmd represents the delete threeports
var DownCmd = &cobra.Command{
	Use:          "down",
	Example:      "tptctl down --name my-threeport",
	Short:        "Spin down a deployment of the Threeport control plane",
	Long:         `Spin down a deployment of the Threeport control plane.`,
	PreRun:       CommandPreRunFunc,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// confirm with user before tearing down
		if !downSkipConfirmation {
			if cliArgs.ControlPlaneOnly {
				fmt.Printf("This will tear down the threeport control plane '%s' (infrastructure will be left intact).\n", cliArgs.ControlPlaneName)
			} else {
				fmt.Printf("This will tear down the threeport control plane '%s' and its underlying infrastructure.\n", cliArgs.ControlPlaneName)
			}
			fmt.Print("Are you sure? (y/N): ")
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(response)) != "y" {
				// a caller with nothing on stdin, such as a test harness,
				// lands here too - exit non-zero so that a teardown which did
				// not happen is not mistaken for one that did
				fmt.Println("Aborted.")
				os.Exit(1)
			}
		}

		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}

		err = cli.DeleteGenesisControlPlane(cpi)
		if err != nil {
			cli.Error("failed to delete threeport control plane", err)
			os.Exit(1)
		}
	},
}

// downSkipConfirmation bypasses the interactive confirmation so that the
// command can be driven by a script or a test harness.
var downSkipConfirmation bool

func init() {
	rootCmd.AddCommand(DownCmd)

	DownCmd.Flags().BoolVarP(
		&downSkipConfirmation,
		"yes", "y", false, "Tear down without asking for confirmation.",
	)

	DownCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"name", "n", "", "Required. Name of genesis control plane.",
	)
	DownCmd.Flags().BoolVar(
		&cliArgs.ControlPlaneOnly,
		"control-plane-only", false, "Tear down the control plane and leave runtime intact. Defaults to false.",
	)
	DownCmd.Flags().BoolVar(
		&cliArgs.InfraOnly,
		"infra-only", false, "Tear down only the infrastructure without the control plane. Defaults to false.",
	)
	DownCmd.Flags().BoolVar(
		&cliArgs.AwsConfigEnv,
		"aws-config-env", false, "Retrieve AWS credentials from environment variables when using eks provider.",
	)
	DownCmd.MarkFlagRequired("name")
}
