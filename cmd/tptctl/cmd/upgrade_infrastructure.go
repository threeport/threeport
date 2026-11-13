/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/provider"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

var upgradeInfraYes bool

// upgradeInfrastructureCmd previews the GKE infrastructure change and applies it if the operator accepts.
var upgradeInfrastructureCmd = &cobra.Command{
	Use:          "infrastructure",
	Example:      "tptctl upgrade infrastructure --name my-threeport",
	Short:        "Preview and apply an infrastructure change",
	Long:         "Compare the current infrastructure program to the deployed stack, print the diff, and apply it if the operator accepts.\n\nThe --yes flag skips the prompt. The command still prints the diff.",
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		// load the control plane config
		_, threeportConfig, _, requestedControlPlane := GetClientContext(cmd)
		if threeportConfig == nil {
			cli.Error("failed to get threeport config", fmt.Errorf("threeport config is empty"))
			os.Exit(1)
		}
		controlPlane, err := threeportConfig.GetControlPlaneConfig(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get control plane config", err)
			os.Exit(1)
		}

		// preview the diff and apply it if the operator accepts
		infra, err := cli.GkeInfraForUpgrade(controlPlane)
		if err != nil {
			cli.Error("failed to prepare infrastructure upgrade", err)
			os.Exit(1)
		}
		err = infra.Upgrade(cmd.Context(), acceptInfrastructureUpgrade)
		if errors.Is(err, provider.ErrUpgradeDeclined) {
			fmt.Println("declined.")
			return
		}
		if err != nil {
			cli.Error("failed to upgrade infrastructure", err)
			os.Exit(1)
		}
	},
}

// acceptInfrastructureUpgrade prints the diff and reads the operator's answer.
// The --yes flag skips the prompt.
func acceptInfrastructureUpgrade(diff string) bool {
	// print the diff
	if diff != "" && !strings.HasSuffix(diff, "\n") {
		diff += "\n"
	}
	fmt.Print(diff)

	// accept without a prompt when --yes is set
	if upgradeInfraYes {
		return true
	}

	// read the operator's answer
	fmt.Print("Apply this change? [y/N]: ")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	answer = strings.TrimSpace(answer)
	return answer == "y" || answer == "Y"
}

// init registers the infrastructure upgrade command.
func init() {
	// add the yes flag and the command
	upgradeInfrastructureCmd.Flags().BoolVar(
		&upgradeInfraYes,
		"yes",
		false,
		"apply the change without a prompt",
	)
	UpgradeCmd.AddCommand(upgradeInfrastructureCmd)
}
