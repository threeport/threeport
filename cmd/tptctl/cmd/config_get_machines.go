/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/kubernetes-runtime/mapping"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

var (
	nodeProfile    string
	nodeSize       string
	awsMachineType string
	ociMachineType string
)

// ConfigGetControlPlanesCmd represents the get-instances command
var ConfigGetMachinesCmd = &cobra.Command{
	Use:          "get-machines",
	Example:      "tptctl config get-machines",
	Short:        "Get a list of available Threeport node profiles and node sizes with their corresponding cloud provider machine types",
	Long:         `Get a list of available Threeport node profiles and node sizes with their corresponding cloud provider machine types.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// validate flags
		providedFlags := []string{}
		if nodeProfile != "" {
			providedFlags = append(providedFlags, "--location")
		}
		if nodeSize != "" {
			providedFlags = append(providedFlags, "--continent")
		}
		if awsMachineType != "" {
			providedFlags = append(providedFlags, "--aws-region")
		}
		if ociMachineType != "" {
			providedFlags = append(providedFlags, "--oci-region")
		}

		if len(providedFlags) > 1 {
			err := fmt.Sprintf("only one filter flag can be provided at a time. Provided flags: %s\n", strings.Join(providedFlags, ", "))
			cli.Error(err, nil)
			os.Exit(1)
		}

		// get the machine type map and print to table
		machineTypeMap := mapping.GetMachineTypeMap()
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NODE PROFILE\t NODE SIZE\t AWS MACHINE TYPE\t OCI MACHINE TYPE")
		filterFound := false
		switch {
		case nodeProfile != "":
			for _, machineType := range *machineTypeMap {
				if machineType.NodeProfile != nodeProfile {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, machineType.NodeProfile, "\t", machineType.NodeSize, "\t", machineType.AwsMachineType, "\t", machineType.OciMachineType)
			}
		case nodeSize != "":
			for _, machineType := range *machineTypeMap {
				if machineType.NodeSize != nodeSize {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, machineType.NodeProfile, "\t", machineType.NodeSize, "\t", machineType.AwsMachineType, "\t", machineType.OciMachineType)
			}
		case awsMachineType != "":
			for _, machineType := range *machineTypeMap {
				if machineType.AwsMachineType != awsMachineType {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, machineType.NodeProfile, "\t", machineType.NodeSize, "\t", machineType.AwsMachineType, "\t", machineType.OciMachineType)
			}
		case ociMachineType != "":
			for _, machineType := range *machineTypeMap {
				if machineType.OciMachineType != ociMachineType {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, machineType.NodeProfile, "\t", machineType.NodeSize, "\t", machineType.AwsMachineType, "\t", machineType.OciMachineType)
			}
		default:
			filterFound = true
			for _, machineType := range *machineTypeMap {
				fmt.Fprintln(writer, machineType.NodeProfile, "\t", machineType.NodeSize, "\t", machineType.AwsMachineType, "\t", machineType.OciMachineType)
			}
		}
		if !filterFound {
			cli.Error("no machines found for the given filter", nil)
			os.Exit(1)
		}
		writer.Flush()
	},
}

func init() {
	ConfigCmd.AddCommand(ConfigGetMachinesCmd)

	ConfigGetMachinesCmd.Flags().StringVarP(
		&nodeProfile,
		"node-profile", "p", "", "Node profile to get machines for",
	)
	ConfigGetMachinesCmd.Flags().StringVarP(
		&nodeSize,
		"node-size", "s", "", "Node size to get machines for",
	)
	ConfigGetMachinesCmd.Flags().StringVarP(
		&awsMachineType,
		"aws-machine-type", "a", "", "AWS machine type to get machines for",
	)
	ConfigGetMachinesCmd.Flags().StringVarP(
		&ociMachineType,
		"oci-machine-type", "o", "", "OCI machine type to get machines for",
	)
}
