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
	LocationName      string
	LocationContinent string
	LocationAwsRegion string
	LocationOciRegion string
)

// ConfigGetControlPlanesCmd represents the get-instances command
var ConfigGetLocationsCmd = &cobra.Command{
	Use:          "get-locations",
	Example:      "tptctl config get-locations",
	Short:        "Get a list of available Threeport locations and what cloud provider regions they map to",
	Long:         `Get a list of available Threeport locations and what cloud provider regions they map to.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// validate flags
		providedFlags := []string{}
		if LocationName != "" {
			providedFlags = append(providedFlags, "--location")
		}
		if LocationContinent != "" {
			providedFlags = append(providedFlags, "--continent")
		}
		if LocationAwsRegion != "" {
			providedFlags = append(providedFlags, "--aws-region")
		}
		if LocationOciRegion != "" {
			providedFlags = append(providedFlags, "--oci-region")
		}

		if len(providedFlags) > 1 {
			err := fmt.Sprintf("only one filter flag can be provided at a time. Provided flags: %s\n", strings.Join(providedFlags, ", "))
			cli.Error(err, nil)
			os.Exit(1)
		}

		// get the region map and print to table
		regionMap := mapping.GetRegionMap()
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "LOCATION\t AWS REGION\t OCI REGION")
		switch {
		case LocationName != "":
			for _, region := range *regionMap {
				if region.Location != LocationName {
					continue
				}
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion)
			}
		case LocationContinent != "":
			for _, region := range *regionMap {
				// get the continent from the location
				continent := strings.Split(region.Location, ":")[0]
				if continent != LocationContinent {
					continue
				}
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion)
			}
		case LocationAwsRegion != "":
			for _, region := range *regionMap {
				if region.AwsRegion != LocationAwsRegion {
					continue
				}
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion)
			}
		case LocationOciRegion != "":
			for _, region := range *regionMap {
				if region.OciRegion != LocationOciRegion {
					continue
				}
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion)
			}
		default:
			for _, region := range *regionMap {
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion)
			}
		}
		writer.Flush()
	},
}

func init() {
	ConfigCmd.AddCommand(ConfigGetLocationsCmd)

	ConfigGetLocationsCmd.Flags().StringVarP(
		&LocationName,
		"location", "l", "", "Location to get regions for",
	)
	ConfigGetLocationsCmd.Flags().StringVarP(
		&LocationContinent,
		"continent", "c", "", "Continent to get regions for",
	)
	ConfigGetLocationsCmd.Flags().StringVarP(
		&LocationAwsRegion,
		"aws-region", "a", "", "AWS region to get locations for",
	)
	ConfigGetLocationsCmd.Flags().StringVarP(
		&LocationOciRegion,
		"oci-region", "o", "", "OCI region to get locations for",
	)
}
