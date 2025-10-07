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
	locationName      string
	locationContinent string
	locationAwsRegion string
	locationOciRegion string
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
		if locationName != "" {
			providedFlags = append(providedFlags, "--location")
		}
		if locationContinent != "" {
			providedFlags = append(providedFlags, "--continent")
		}
		if locationAwsRegion != "" {
			providedFlags = append(providedFlags, "--aws-region")
		}
		if locationOciRegion != "" {
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
		filterFound := false
		switch {
		case locationName != "":
			for _, region := range *regionMap {
				if region.Location != locationName {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion)
			}
		case locationContinent != "":
			for _, region := range *regionMap {
				// get the continent from the location
				continent := strings.Split(region.Location, ":")[0]
				if continent != locationContinent {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion)
			}
		case locationAwsRegion != "":
			for _, region := range *regionMap {
				if region.AwsRegion != locationAwsRegion {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion)
			}
		case locationOciRegion != "":
			for _, region := range *regionMap {
				if region.OciRegion != locationOciRegion {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion)
			}
		default:
			filterFound = true
			for _, region := range *regionMap {
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion)
			}
		}
		if !filterFound {
			cli.Error("no locations found for the given filter", nil)
			os.Exit(1)
		}
		writer.Flush()
	},
}

func init() {
	ConfigCmd.AddCommand(ConfigGetLocationsCmd)

	ConfigGetLocationsCmd.Flags().StringVarP(
		&locationName,
		"location", "l", "", "Location to get regions for",
	)
	ConfigGetLocationsCmd.Flags().StringVarP(
		&locationContinent,
		"continent", "c", "", "Continent to get regions for",
	)
	ConfigGetLocationsCmd.Flags().StringVarP(
		&locationAwsRegion,
		"aws-region", "a", "", "AWS region to get locations for",
	)
	ConfigGetLocationsCmd.Flags().StringVarP(
		&locationOciRegion,
		"oci-region", "o", "", "OCI region to get locations for",
	)
}
