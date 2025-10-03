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
	OciRegion string
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

// GetRegionMap returns the mappings of threeport locations to cloud provider
// regions.
func GetRegionMap() *[]RegionMap {
	return &[]RegionMap{
		// North America
		{
			Location:  "Local",
			AwsRegion: "us-east-1",
			OciRegion: "us-ashburn-1",
		},
		{
			Location:  "NorthAmerica:NewYork",
			AwsRegion: "us-east-1",
			OciRegion: "us-ashburn-1",
		},
		{
			Location:  "NorthAmerica:Chicago",
			AwsRegion: "us-east-2",
			OciRegion: "us-chicago-1",
		},
		{
			Location:  "NorthAmerica:LosAngeles",
			AwsRegion: "us-west-1",
			OciRegion: "us-sanjose-1",
		},
		{
			Location:  "NorthAmerica:Seattle",
			AwsRegion: "us-west-2",
			OciRegion: "us-sanjose-1",
		},
		{
			Location:  "NorthAmerica:Denver",
			AwsRegion: "us-west-1",
			OciRegion: "us-phoenix-1",
		},
		{
			Location:  "NorthAmerica:Calgary",
			AwsRegion: "ca-west-1",
			OciRegion: "ca-toronto-1",
		},
		{
			Location:  "NorthAmerica:Toronto",
			AwsRegion: "ca-central-1",
			OciRegion: "ca-toronto-1",
		},
		{
			Location:  "NorthAmerica:Montreal",
			AwsRegion: "ca-central-1",
			OciRegion: "ca-montreal-1",
		},
		{
			Location:  "NorthAmerica:Monterrey",
			AwsRegion: "mx-central-1",
			OciRegion: "mx-monterrey-1",
		},
		{
			Location:  "NorthAmerica:MexicoCity",
			AwsRegion: "mx-central-1",
			OciRegion: "mx-queretaro-1",
		},
		// Asia
		{
			Location:  "Asia:Jerusalem",
			AwsRegion: "il-central-1",
			OciRegion: "il-jerusalem-1",
		},
		{
			Location:  "Asia:Jeddah",
			AwsRegion: "me-south-1",
			OciRegion: "me-jeddah-1",
		},
		{
			Location:  "Asia:Riyadh",
			AwsRegion: "me-south-1",
			OciRegion: "me-riyadh-1",
		},
		{
			Location:  "Asia:Dubai",
			AwsRegion: "me-central-1",
			OciRegion: "me-dubai-1",
		},
		{
			Location:  "Asia:AbuDhabi",
			AwsRegion: "me-central-1",
			OciRegion: "me-abudhabi-1",
		},
		{
			Location:  "Asia:Mumbai",
			AwsRegion: "ap-south-1",
			OciRegion: "ap-mumbai-1",
		},
		{
			Location:  "Asia:Bangalore",
			AwsRegion: "ap-south-2",
			OciRegion: "ap-hyderabad-1",
		},
		{
			Location:  "Asia:Bangkok",
			AwsRegion: "ap-southeast-7",
			OciRegion: "ap-singapore-1",
		},
		{
			Location:  "Asia:KualaLumpur",
			AwsRegion: "ap-southeast-5",
			OciRegion: "ap-singapore-1",
		},
		{
			Location:  "Asia:Singapore",
			AwsRegion: "ap-southeast-1",
			OciRegion: "ap-singapore-1",
		},
		{
			Location:  "Asia:Jakarta",
			AwsRegion: "ap-southeast-3",
			OciRegion: "ap-batam-1",
		},
		{
			Location:  "Asia:HongKong",
			AwsRegion: "ap-east-1",
			OciRegion: "ap-singapore-2",
		},
		{
			Location:  "Asia:Seoul",
			AwsRegion: "ap-northeast-2",
			OciRegion: "ap-seoul-1",
		},
		{
			Location:  "Asia:Tokyo",
			AwsRegion: "ap-northeast-1",
			OciRegion: "ap-tokyo-1",
		},
		{
			Location:  "Asia:Osaka",
			AwsRegion: "ap-northeast-3",
			OciRegion: "ap-osaka-1",
		},
		{
			Location:  "Asia:Taipei",
			AwsRegion: "ap-east-2",
			OciRegion: "ap-osaka-1",
		},
		// Oceana
		{
			Location:  "Oceana:Sydney",
			AwsRegion: "ap-southeast-2",
			OciRegion: "ap-sydney-1",
		},
		{
			Location:  "Oceana:Melbourne",
			AwsRegion: "ap-southeast-4",
			OciRegion: "ap-melbourne-1",
		},
		{
			Location:  "Oceana:Auckland",
			AwsRegion: "ap-southeast-6",
			OciRegion: "ap-melbourne-1",
		},
		// Europe
		{
			Location:  "Europe:Madrid",
			AwsRegion: "eu-south-2",
			OciRegion: "eu-madrid-1",
		},
		{
			Location:  "Europe:Dublin",
			AwsRegion: "eu-west-1",
			OciRegion: "uk-cardiff-1",
		},
		{
			Location:  "Europe:London",
			AwsRegion: "eu-west-2",
			OciRegion: "uk-london-1",
		},
		{
			Location:  "Europe:Paris",
			AwsRegion: "eu-west-3",
			OciRegion: "eu-paris-1",
		},
		{
			Location:  "Europe:Marseille",
			AwsRegion: "eu-south-1",
			OciRegion: "eu-marseille-1",
		},
		{
			Location:  "Europe:Milan",
			AwsRegion: "eu-south-1",
			OciRegion: "eu-milan-1",
		},
		{
			Location:  "Europe:Amsterdam",
			AwsRegion: "eu-central-1",
			OciRegion: "eu-amsterdam-1",
		},
		{
			Location:  "Europe:Frankfurt",
			AwsRegion: "eu-central-1",
			OciRegion: "eu-frankfurt-1",
		},
		{
			Location:  "Europe:Zurich",
			AwsRegion: "eu-central-2",
			OciRegion: "eu-zurich-1",
		},
		{
			Location:  "Europe:Stockholm",
			AwsRegion: "eu-north-1",
			OciRegion: "eu-stockholm-1",
		},
		// South America
		{
			Location:  "SouthAmerica:SaoPaulo",
			AwsRegion: "sa-east-1",
			OciRegion: "sa-saopaulo-1",
		},
		{
			Location:  "SouthAmerica:Bogota",
			AwsRegion: "sa-east-1",
			OciRegion: "sa-bogota-1",
		},
		{
			Location:  "SouthAmerica:Santiago",
			AwsRegion: "sa-east-1",
			OciRegion: "sa-santiago-1",
		},
		{
			Location:  "SouthAmerica:Valparaiso",
			AwsRegion: "sa-east-1",
			OciRegion: "sa-valparaiso-1",
		},
		{
			Location:  "SouthAmerica:Campinas",
			AwsRegion: "sa-east-1",
			OciRegion: "sa-vinhedo-1",
		},
		// Africa
		{
			Location:  "Africa:Johannesburg",
			AwsRegion: "af-south-1",
			OciRegion: "af-johannesburg-1",
		},
	}
}

// ValidLocation returns true if the location provided is a supported location.
func ValidLocation(location string) bool {
	// validate location
	locationFound := false
	for _, mapping := range *GetRegionMap() {
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
	for _, r := range *GetRegionMap() {
		if r.Location == location {
			switch provider {
			case util.AwsProvider:
				return r.AwsRegion, nil
			case util.OciProvider:
				return r.OciRegion, nil
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
	for _, r := range *GetRegionMap() {
		if r.AwsRegion == awsRegion {
			return r.Location, nil
		}
	}

	msg := fmt.Sprintf("AWS region %s not supported", awsRegion)
	return "", &RegionError{Message: msg}
}

// GetLocationForOciRegion returns the threeport location for a given OCI
// region.
func GetLocationForOciRegion(ociRegion string) (string, error) {
	for _, r := range *GetRegionMap() {
		if r.OciRegion == ociRegion {
			return r.Location, nil
		}
	}

	msg := fmt.Sprintf("OCI region %s not supported", ociRegion)
	return "", &RegionError{Message: msg}
}
