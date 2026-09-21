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

	cli "github.com/threeport/threeport/pkg/cli/v0"
	mapping "github.com/threeport/threeport/pkg/mapping/v0"
)

var (
	locationName      string
	locationContinent string
	locationAwsRegion string
	locationOciRegion string
	locationGcpRegion string
)

// ConfigGetControlPlanesCmd represents the get-instances command
var ConfigGetLocationsCmd = &cobra.Command{
	Use:          "get-locations",
	Example:      "tptctl config get-locations",
	Short:        "Get a list of available Threeport locations and what cloud provider regions they map to",
	Long:         `Get a list of available Threeport locations and what cloud provider regions they map to.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get the region map and print to table
		regionMap := mapping.GetRegionMap()
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "LOCATION\t AWS REGION\t OCI REGION\t GCP REGION")
		filterFound := false
		switch {
		case locationName != "":
			for _, region := range *regionMap {
				if region.Location != locationName {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion, "\t", region.GcpRegion)
			}
		case locationContinent != "":
			for _, region := range *regionMap {
				// get the continent from the location
				continent := strings.Split(region.Location, ":")[0]
				if continent != locationContinent {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion, "\t", region.GcpRegion)
			}
		case locationAwsRegion != "":
			for _, region := range *regionMap {
				if region.AwsRegion != locationAwsRegion {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion, "\t", region.GcpRegion)
			}
		case locationOciRegion != "":
			for _, region := range *regionMap {
				if region.OciRegion != locationOciRegion {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion, "\t", region.GcpRegion)
			}
		case locationGcpRegion != "":
			for _, region := range *regionMap {
				if region.GcpRegion != locationGcpRegion {
					continue
				}
				filterFound = true
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion, "\t", region.GcpRegion)
			}
		default:
			filterFound = true
			for _, region := range *regionMap {
				fmt.Fprintln(writer, region.Location, "\t", region.AwsRegion, "\t", region.OciRegion, "\t", region.GcpRegion)
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
	ConfigGetLocationsCmd.Flags().StringVarP(
		&locationGcpRegion,
		"gcp-region", "g", "", "GCP region to get locations for",
	)

	// One filter at a time. cobra enforces this before Run and panics at
	// startup on a name no flag declares, so a name here that no longer
	// matches a declaration above fails loudly instead of silently.
	ConfigGetLocationsCmd.MarkFlagsMutuallyExclusive(
		"location",
		"continent",
		"aws-region",
		"oci-region",
		"gcp-region",
	)
}
