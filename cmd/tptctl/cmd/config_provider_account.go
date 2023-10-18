/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var configCurrentInstanceName string

// ConfigCurrentInstanceCmd represents the current-instance command
var ConfigCurrentInstanceCmd = &cobra.Command{
	Use:     "current-instance",
	Example: "tptctl config current-instance --instance-name my-threeport-instance",
	Short:   "Set a threeport instance as the current in-use instance",
	Long: `Set a threeport instance as the current in-use instance.  Once set as
the current instance all subsequent tptctl commands will apply to that Threeport
instance.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config
		threeportConfig, _, err := config.GetThreeportConfig(cliArgs.InstanceName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		// get all instances in the threeport config and make sure the requested
		// current instance name is there.
		allInstances := threeportConfig.GetAllInstanceNames()
		instanceFound := false
		for _, inst := range allInstances {
			if inst == configCurrentInstanceName {
				instanceFound = true
				break
			}
		}
		if !instanceFound {
			err := errors.New(fmt.Sprintf(
				"the requested current instance name %s was not found in your threeport config",
				configCurrentInstanceName,
			))
			cli.Error("cannot set instance as current instance", err)
			os.Exit(1)
		}

		// set the current instance
		threeportConfig.SetCurrentInstance(configCurrentInstanceName)

		cli.Complete(fmt.Sprintf("Threeport instance %s set as the current instance", configCurrentInstanceName))
	},
}

func init() {
	configCmd.AddCommand(ConfigCurrentInstanceCmd)

	ConfigCurrentInstanceCmd.Flags().StringVarP(
		&configCurrentInstanceName,
		"instance-name", "i", "", "The name of the Threeport instance to set as current.",
	)
	ConfigCurrentInstanceCmd.MarkFlagRequired("instance-name")
}
