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
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/provider"
	api "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

var upgradeInfraYes bool

// The control plane tier to size the infrastructure for
var upgradeInfraTier string

// UpgradeInfrastructureCmd previews the GKE infrastructure change and applies it if the operator accepts.
var UpgradeInfrastructureCmd = &cobra.Command{
	Use:          "infrastructure",
	Example:      "tptctl upgrade infrastructure",
	Short:        "Preview and apply an infrastructure change",
	Long:         "Compare the current infrastructure program to the deployed stack, print the diff, and apply it if the operator accepts.\n\nThe --yes flag skips the prompt. The command still prints the diff.",
	SilenceUsage: true,
	PreRun:       CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		// load the control plane config
		apiClient, threeportConfig, apiEndpoint, requestedControlPlane := GetClientContext(cmd)
		if threeportConfig == nil {
			cli.Error("failed to get threeport config", fmt.Errorf("threeport config is empty"))
			os.Exit(1)
		}
		controlPlane, err := threeportConfig.GetControlPlaneConfig(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get control plane config", err)
			os.Exit(1)
		}

		// reject a child connection before preparing the change
		controlPlaneInstance, err := client.GetSelfControlPlaneInstance(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve self control plane instance from threeport API", err)
			os.Exit(1)
		}
		if controlPlaneInstance.ParentControlPlaneInstanceID != nil {
			cli.Error("failed to prepare infrastructure upgrade", api.ErrRequestMustBeMadeToParent)
			os.Exit(1)
		}

		// reject a tier outside the known set before touching the stack
		if upgradeInfraTier != "" && !slices.Contains(api.ControlPlaneTiers, upgradeInfraTier) {
			cli.Error("invalid --tier", fmt.Errorf("must be one of %s", strings.Join(api.ControlPlaneTiers, ", ")))
			os.Exit(1)
		}

		// use the tier given, or else the recorded tier
		tier := upgradeInfraTier
		if tier == "" {
			if controlPlaneInstance.ControlPlaneDefinitionID == nil {
				cli.Error("failed to load the recorded tier", errors.New("control plane instance has no definition"))
				os.Exit(1)
			}
			controlPlaneDefinition, err := client.GetControlPlaneDefinitionByID(
				apiClient,
				apiEndpoint,
				*controlPlaneInstance.ControlPlaneDefinitionID,
			)
			if err != nil {
				cli.Error("failed to load the recorded tier", err)
				os.Exit(1)
			}
			tier = util.DerefString(controlPlaneDefinition.Tier)
		}

		// build the infrastructure a fresh install of the tier creates
		infra, err := cli.GkeInfraForUpgrade(controlPlane, tier)
		if err != nil {
			cli.Error("failed to prepare infrastructure upgrade", err)
			os.Exit(1)
		}

		// keep the egress address the cluster has
		if err := cli.KeepRecordedNatAddress(infra, apiClient, apiEndpoint, infra.RuntimeInstanceName); err != nil {
			cli.Error("failed to load the recorded NAT address setting", err)
			os.Exit(1)
		}

		// preview and apply the change
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
	UpgradeCmd.AddCommand(UpgradeInfrastructureCmd)
	UpgradeInfrastructureCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	UpgradeInfrastructureCmd.Flags().StringVar(
		&upgradeInfraTier,
		"tier", "", fmt.Sprintf(
			"Optional. Control plane tier to size the infrastructure for. One of %s. Defaults to the tier recorded for the control plane.",
			strings.Join(api.ControlPlaneTiers, ", "),
		),
	)
	UpgradeInfrastructureCmd.Flags().BoolVar(
		&upgradeInfraYes,
		"yes",
		false,
		"apply the change without a prompt",
	)
}
