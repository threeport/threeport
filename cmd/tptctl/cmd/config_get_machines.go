/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	mapping "github.com/threeport/threeport/pkg/mapping/v0"
)

var (
	nodeProfile    string
	nodeSize       string
	awsMachineType string
	ociMachineType string
	gcpMachineType string
)

// ConfigGetControlPlanesCmd represents the get-instances command
var ConfigGetMachinesCmd = &cobra.Command{
	Use:          "get-machines",
	Example:      "tptctl config get-machines",
	Short:        "Get a list of available Threeport node profiles and node sizes with their corresponding cloud provider machine types",
	Long:         `Get a list of available Threeport node profiles and node sizes with their corresponding cloud provider machine types.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get the machine type map and print to table
		machineTypeMap := mapping.GetMachineTypeMap()
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NODE PROFILE\t NODE SIZE\t AWS MACHINE TYPE\t OCI MACHINE TYPE\t GCP MACHINE TYPE")
		filterFound := false
		switch {
		case nodeProfile != "":
			for _, machineType := range *machineTypeMap {
				if machineType.NodeProfile != nodeProfile {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, machineType.NodeProfile, "\t", machineType.NodeSize, "\t", machineType.AwsMachineType, "\t", machineType.OciMachineType, "\t", machineType.GcpMachineType)
			}
		case nodeSize != "":
			for _, machineType := range *machineTypeMap {
				if machineType.NodeSize != nodeSize {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, machineType.NodeProfile, "\t", machineType.NodeSize, "\t", machineType.AwsMachineType, "\t", machineType.OciMachineType, "\t", machineType.GcpMachineType)
			}
		case awsMachineType != "":
			for _, machineType := range *machineTypeMap {
				if machineType.AwsMachineType != awsMachineType {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, machineType.NodeProfile, "\t", machineType.NodeSize, "\t", machineType.AwsMachineType, "\t", machineType.OciMachineType, "\t", machineType.GcpMachineType)
			}
		case ociMachineType != "":
			for _, machineType := range *machineTypeMap {
				if machineType.OciMachineType != ociMachineType {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, machineType.NodeProfile, "\t", machineType.NodeSize, "\t", machineType.AwsMachineType, "\t", machineType.OciMachineType, "\t", machineType.GcpMachineType)
			}
		case gcpMachineType != "":
			for _, machineType := range *machineTypeMap {
				if machineType.GcpMachineType != gcpMachineType {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, machineType.NodeProfile, "\t", machineType.NodeSize, "\t", machineType.AwsMachineType, "\t", machineType.OciMachineType, "\t", machineType.GcpMachineType)
			}
		default:
			filterFound = true
			for _, machineType := range *machineTypeMap {
				fmt.Fprintln(writer, machineType.NodeProfile, "\t", machineType.NodeSize, "\t", machineType.AwsMachineType, "\t", machineType.OciMachineType, "\t", machineType.GcpMachineType)
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
	ConfigGetMachinesCmd.Flags().StringVarP(
		&gcpMachineType,
		"gcp-machine-type", "g", "", "GCP machine type to get machines for",
	)

	// One filter at a time. cobra enforces this before Run and panics at
	// startup on a name no flag declares, so a name here that no longer
	// matches a declaration above fails loudly instead of silently.
	ConfigGetMachinesCmd.MarkFlagsMutuallyExclusive(
		"node-profile",
		"node-size",
		"aws-machine-type",
		"oci-machine-type",
		"gcp-machine-type",
	)
}
