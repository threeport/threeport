package mapping

import (
	"fmt"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// RegionMap contains a threeport location with the corresponding regions for
// cloud providers.
type RegionMap struct {
	Location  string
	AwsRegion string
	AksRegion string
	//GcpRegion string  // future use
}

// ProviderError is an error returned when an unsupported provider is used.
type ProviderError struct {
	Message string
}

// Error returns a customized message for the ProviderError.
func (e *ProviderError) Error() string {
	return e.Message
}

// LocationError is an error returned when an unsupported location is used.
type LocationError struct {
	Message string
}

// Error returns a customized message for the LocationError.
func (e *LocationError) Error() string {
	return e.Message
}

// RegionError is an error returned when an unsupported cloud provider region is
// used.
type RegionError struct {
	Message string
}

// Error returns a customized message for the RegionError.
func (e *RegionError) Error() string {
	return e.Message
}

// getRegionMap returns the mappings of threeport locations to cloud provider
// regions.
func getRegionMap() *[]RegionMap {
	return &[]RegionMap{
		{
			Location:  "Local",
			AwsRegion: "us-east-1",
		},
		{
			Location:  "NorthAmerica:NewYork",
			AwsRegion: "us-east-1",
		},
		{
			Location:  "NorthAmerica:Chicago",
			AwsRegion: "us-east-2",
		},
		{
			Location:  "NorthAmerica:LosAngeles",
			AwsRegion: "us-west-1",
			AksRegion: "West US",
		},
		{
			Location:  "NorthAmerica:Seattle",
			AwsRegion: "us-west-2",
		},
		{
			Location:  "NorthAmerica:Toronto",
			AwsRegion: "ca-central-1",
		},
		{
			Location:  "Asia:HongKong",
			AwsRegion: "ap-east-1",
		},
		{
			Location:  "Asia:Hyderabad",
			AwsRegion: "ap-south-2",
		},
		{
			Location:  "Asia:Jakarta",
			AwsRegion: "ap-southeast-3",
		},
		{
			Location:  "Asia:Mumbai",
			AwsRegion: "ap-south-1",
		},
		{
			Location:  "Asia:Osaka",
			AwsRegion: "ap-northeast-3",
		},
		{
			Location:  "Asia:Seoul",
			AwsRegion: "ap-northeast-2",
		},
		{
			Location:  "Asia:Singapore",
			AwsRegion: "ap-southeast-1",
		},
		{
			Location:  "Asia:Tokyo",
			AwsRegion: "ap-northeast-1",
		},
		{
			Location:  "Asia:Manama",
			AwsRegion: "me-south-1",
		},
		{
			Location:  "Asia:Dubai",
			AwsRegion: "me-central-1",
		},
		{
			Location:  "Oceana:Sydney",
			AwsRegion: "ap-southeast-2",
		},
		{
			Location:  "Oceana:Melbourne",
			AwsRegion: "ap-southeast-4",
		},
		{
			Location:  "Europe:Frankfurt",
			AwsRegion: "eu-central-1",
		},
		{
			Location:  "Europe:Dublin",
			AwsRegion: "eu-west-1",
		},
		{
			Location:  "Europe:London",
			AwsRegion: "eu-west-2",
		},
		{
			Location:  "Europe:Milan",
			AwsRegion: "eu-south-1",
		},
		{
			Location:  "Europe:Paris",
			AwsRegion: "eu-west-3",
		},
		{
			Location:  "Europe:Madrid",
			AwsRegion: "eu-south-2",
		},
		{
			Location:  "Europe:Stockholm",
			AwsRegion: "eu-north-1",
		},
		{
			Location:  "Europe:Zurich",
			AwsRegion: "eu-central-2",
		},
		{
			Location:  "SouthAmerica:SaoPaulo",
			AwsRegion: "sa-east-1",
		},
		{
			Location:  "Africa:CapeTown",
			AwsRegion: "af-south-1",
		},
	}
}

// ValidLocation returns true if the location provided is a supported location.
func ValidLocation(location string) bool {
	// validate location
	locationFound := false
	for _, mapping := range *getRegionMap() {
		if location == mapping.Location {
			locationFound = true
			break
		}
	}

	return locationFound
}

// GetProviderRegionForLocation returns a cloud provider region for a given
// threeport location and provider.
func GetProviderRegionForLocation(provider, location string) (string, error) {
	for _, r := range *getRegionMap() {
		if r.Location == location {
			switch provider {
			case util.AwsProvider:
				return r.AwsRegion, nil
			case util.AksProvider:
				return r.AksRegion, nil
			default:
				msg := fmt.Sprintf("provider %s not supported", provider)
				return "", &ProviderError{Message: msg}
			}
		}
	}

	msg := fmt.Sprintf("location %s not supported", location)
	return "", &LocationError{Message: msg}
}

// GetLocationForAwsRegion returns the threeport location for a given AWS
// region.
func GetLocationForAwsRegion(awsRegion string) (string, error) {
	for _, r := range *getRegionMap() {
		if r.AwsRegion == awsRegion {
			return r.Location, nil
		}
	}

	msg := fmt.Sprintf("AWS region %s not supported", awsRegion)
	return "", &RegionError{Message: msg}
}

// GetLocationFor
func GetLocationForAksRegion(aksRegion string) (string, error) {
	for _, r := range *getRegionMap() {
		if r.AksRegion == aksRegion {
			return r.Location, nil
		}
	}

	msg := fmt.Sprintf("AKS region %s not supported", aksRegion)
	return "", &RegionError{Message: msg}
}
