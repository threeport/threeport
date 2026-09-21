// Package v0 translates threeport's provider-neutral vocabulary into the
// identifiers each cloud provider uses. It maps a threeport location to an
// AWS, OCI or GCP region, and a node profile with a node size to a machine
// type on each of those providers. It also reports whether a location or
// machine size is one threeport supports, so callers can reject bad input
// before it reaches a controller.
package v0

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
	GcpRegion string
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
			GcpRegion: "us-east1",
		},
		{
			Location:  "NorthAmerica:NewYork",
			AwsRegion: "us-east-1",
			OciRegion: "us-ashburn-1",
			GcpRegion: "us-east4",
		},
		{
			Location:  "NorthAmerica:Atlanta",
			AwsRegion: "us-east-1",
			OciRegion: "us-ashburn-1",
			GcpRegion: "us-east1",
		},
		{
			Location:  "NorthAmerica:Chicago",
			AwsRegion: "us-east-2",
			OciRegion: "us-chicago-1",
			GcpRegion: "us-east5",
		},
		{
			Location:  "NorthAmerica:Dallas",
			AwsRegion: "us-east-2",
			OciRegion: "us-chicago-1",
			GcpRegion: "us-south1",
		},
		{
			Location:  "NorthAmerica:Denver",
			AwsRegion: "us-west-1",
			OciRegion: "us-phoenix-1",
			GcpRegion: "us-central1",
		},
		{
			Location:  "NorthAmerica:Phoenix",
			AwsRegion: "us-west-1",
			OciRegion: "us-phoenix-1",
			GcpRegion: "us-west4",
		},
		{
			Location:  "NorthAmerica:SaltLakeCity",
			AwsRegion: "us-west-1",
			OciRegion: "us-phoenix-1",
			GcpRegion: "us-west3",
		},
		{
			Location:  "NorthAmerica:LosAngeles",
			AwsRegion: "us-west-1",
			OciRegion: "us-sanjose-1",
			GcpRegion: "us-west2",
		},
		{
			Location:  "NorthAmerica:Seattle",
			AwsRegion: "us-west-2",
			OciRegion: "us-sanjose-1",
			GcpRegion: "us-west1",
		},
		{
			Location:  "NorthAmerica:Calgary",
			AwsRegion: "ca-west-1",
			OciRegion: "ca-toronto-1",
			GcpRegion: "northamerica-northeast2",
		},
		{
			Location:  "NorthAmerica:Toronto",
			AwsRegion: "ca-central-1",
			OciRegion: "ca-toronto-1",
			GcpRegion: "northamerica-northeast2",
		},
		{
			Location:  "NorthAmerica:Montreal",
			AwsRegion: "ca-central-1",
			OciRegion: "ca-montreal-1",
			GcpRegion: "northamerica-northeast1",
		},
		{
			Location:  "NorthAmerica:Monterrey",
			AwsRegion: "mx-central-1",
			OciRegion: "mx-monterrey-1",
			GcpRegion: "northamerica-south1",
		},
		{
			Location:  "NorthAmerica:MexicoCity",
			AwsRegion: "mx-central-1",
			OciRegion: "mx-queretaro-1",
			GcpRegion: "northamerica-south1",
		},
		// Asia
		{
			Location:  "Asia:Jerusalem",
			AwsRegion: "il-central-1",
			OciRegion: "il-jerusalem-1",
			GcpRegion: "me-west1",
		},
		{
			Location:  "Asia:Jeddah",
			AwsRegion: "me-south-1",
			OciRegion: "me-jeddah-1",
			GcpRegion: "me-central1",
		},
		{
			Location:  "Asia:Riyadh",
			AwsRegion: "me-south-1",
			OciRegion: "me-riyadh-1",
			GcpRegion: "me-central1",
		},
		{
			Location:  "Asia:Dubai",
			AwsRegion: "me-central-1",
			OciRegion: "me-dubai-1",
			GcpRegion: "me-central2",
		},
		{
			Location:  "Asia:AbuDhabi",
			AwsRegion: "me-central-1",
			OciRegion: "me-abudhabi-1",
			GcpRegion: "me-central2",
		},
		{
			Location:  "Asia:Delhi",
			AwsRegion: "ap-south-1",
			OciRegion: "ap-mumbai-1",
			GcpRegion: "asia-south2",
		},
		{
			Location:  "Asia:Mumbai",
			AwsRegion: "ap-south-1",
			OciRegion: "ap-mumbai-1",
			GcpRegion: "asia-south1",
		},
		{
			Location:  "Asia:Bangalore",
			AwsRegion: "ap-south-2",
			OciRegion: "ap-hyderabad-1",
			GcpRegion: "asia-south1",
		},
		{
			Location:  "Asia:Bangkok",
			AwsRegion: "ap-southeast-7",
			OciRegion: "ap-singapore-1",
			GcpRegion: "asia-southeast1",
		},
		{
			Location:  "Asia:KualaLumpur",
			AwsRegion: "ap-southeast-5",
			OciRegion: "ap-singapore-1",
			GcpRegion: "asia-southeast1",
		},
		{
			Location:  "Asia:Singapore",
			AwsRegion: "ap-southeast-1",
			OciRegion: "ap-singapore-1",
			GcpRegion: "asia-southeast1",
		},
		{
			Location:  "Asia:Jakarta",
			AwsRegion: "ap-southeast-3",
			OciRegion: "ap-batam-1",
			GcpRegion: "asia-southeast2",
		},
		{
			Location:  "Asia:HongKong",
			AwsRegion: "ap-east-1",
			OciRegion: "ap-singapore-2",
			GcpRegion: "asia-east2",
		},
		{
			Location:  "Asia:Seoul",
			AwsRegion: "ap-northeast-2",
			OciRegion: "ap-seoul-1",
			GcpRegion: "asia-northeast3",
		},
		{
			Location:  "Asia:Tokyo",
			AwsRegion: "ap-northeast-1",
			OciRegion: "ap-tokyo-1",
			GcpRegion: "asia-northeast1",
		},
		{
			Location:  "Asia:Osaka",
			AwsRegion: "ap-northeast-3",
			OciRegion: "ap-osaka-1",
			GcpRegion: "asia-northeast2",
		},
		{
			Location:  "Asia:Taipei",
			AwsRegion: "ap-east-2",
			OciRegion: "ap-osaka-1",
			GcpRegion: "asia-east1",
		},
		// Oceana
		{
			Location:  "Oceana:Sydney",
			AwsRegion: "ap-southeast-2",
			OciRegion: "ap-sydney-1",
			GcpRegion: "australia-southeast1",
		},
		{
			Location:  "Oceana:Melbourne",
			AwsRegion: "ap-southeast-4",
			OciRegion: "ap-melbourne-1",
			GcpRegion: "australia-southeast2",
		},
		{
			Location:  "Oceana:Auckland",
			AwsRegion: "ap-southeast-6",
			OciRegion: "ap-melbourne-1",
			GcpRegion: "australia-southeast2",
		},
		// Europe
		{
			Location:  "Europe:Madrid",
			AwsRegion: "eu-south-2",
			OciRegion: "eu-madrid-1",
			GcpRegion: "europe-southwest1",
		},
		{
			Location:  "Europe:Dublin",
			AwsRegion: "eu-west-1",
			OciRegion: "uk-cardiff-1",
			GcpRegion: "europe-west2",
		},
		{
			Location:  "Europe:London",
			AwsRegion: "eu-west-2",
			OciRegion: "uk-london-1",
			GcpRegion: "europe-west2",
		},
		{
			Location:  "Europe:Paris",
			AwsRegion: "eu-west-3",
			OciRegion: "eu-paris-1",
			GcpRegion: "europe-west9",
		},
		{
			Location:  "Europe:Marseille",
			AwsRegion: "eu-south-1",
			OciRegion: "eu-marseille-1",
			GcpRegion: "europe-west12",
		},
		{
			Location:  "Europe:Milan",
			AwsRegion: "eu-south-1",
			OciRegion: "eu-milan-1",
			GcpRegion: "europe-west8",
		},
		{
			Location:  "Europe:Amsterdam",
			AwsRegion: "eu-central-1",
			OciRegion: "eu-amsterdam-1",
			GcpRegion: "europe-west4",
		},
		{
			Location:  "Europe:Brussels",
			AwsRegion: "eu-central-1",
			OciRegion: "eu-amsterdam-1",
			GcpRegion: "europe-west1",
		},
		{
			Location:  "Europe:Zurich",
			AwsRegion: "eu-central-2",
			OciRegion: "eu-zurich-1",
			GcpRegion: "europe-west6",
		},
		{
			Location:  "Europe:Frankfurt",
			AwsRegion: "eu-central-1",
			OciRegion: "eu-frankfurt-1",
			GcpRegion: "europe-west3",
		},
		{
			Location:  "Europe:Berlin",
			AwsRegion: "eu-central-1",
			OciRegion: "eu-frankfurt-1",
			GcpRegion: "europe-west10",
		},
		{
			Location:  "Europe:Warsaw",
			AwsRegion: "eu-central-1",
			OciRegion: "eu-frankfurt-1",
			GcpRegion: "europe-central2",
		},
		{
			Location:  "Europe:Stockholm",
			AwsRegion: "eu-north-1",
			OciRegion: "eu-stockholm-1",
			GcpRegion: "europe-north2",
		},
		{
			Location:  "Europe:Helsinki",
			AwsRegion: "eu-north-1",
			OciRegion: "eu-stockholm-1",
			GcpRegion: "europe-north1",
		},
		// South America
		{
			Location:  "SouthAmerica:SaoPaulo",
			AwsRegion: "sa-east-1",
			OciRegion: "sa-saopaulo-1",
			GcpRegion: "southamerica-east1",
		},
		{
			Location:  "SouthAmerica:Campinas",
			AwsRegion: "sa-east-1",
			OciRegion: "sa-vinhedo-1",
			GcpRegion: "southamerica-east1",
		},
		{
			Location:  "SouthAmerica:Bogota",
			AwsRegion: "sa-east-1",
			OciRegion: "sa-bogota-1",
			GcpRegion: "southamerica-east1",
		},
		{
			Location:  "SouthAmerica:Santiago",
			AwsRegion: "sa-east-1",
			OciRegion: "sa-santiago-1",
			GcpRegion: "southamerica-west1",
		},
		{
			Location:  "SouthAmerica:Valparaiso",
			AwsRegion: "sa-east-1",
			OciRegion: "sa-valparaiso-1",
			GcpRegion: "southamerica-west1",
		},
		// Africa
		{
			Location:  "Africa:Johannesburg",
			AwsRegion: "af-south-1",
			OciRegion: "af-johannesburg-1",
			GcpRegion: "africa-south1",
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
			case util.GcpProvider:
				return r.GcpRegion, nil
			default:
				msg := fmt.Sprintf("provider %s not supported", provider)
				return "", &ProviderError{Message: msg}
			}
		}
	}

	msg := fmt.Sprintf("location %s not supported", location)
	return "", &LocationError{Message: msg}
}

// GetLocationForAwsRegion returns the threeport location for a given AWS region.
func GetLocationForAwsRegion(awsRegion string) (string, error) {
	for _, r := range *GetRegionMap() {
		if r.AwsRegion == awsRegion {
			return r.Location, nil
		}
	}

	msg := fmt.Sprintf("AWS region %s not supported", awsRegion)
	return "", &RegionError{Message: msg}
}

// GetLocationForOciRegion returns the threeport location for a given OCI region.
func GetLocationForOciRegion(ociRegion string) (string, error) {
	for _, r := range *GetRegionMap() {
		if r.OciRegion == ociRegion {
			return r.Location, nil
		}
	}

	msg := fmt.Sprintf("OCI region %s not supported", ociRegion)
	return "", &RegionError{Message: msg}
}

// GetLocationForGcpRegion returns the threeport location for a given GCP region.
func GetLocationForGcpRegion(gcpRegion string) (string, error) {
	for _, r := range *GetRegionMap() {
		if r.GcpRegion == gcpRegion {
			return r.Location, nil
		}
	}

	msg := fmt.Sprintf("GCP region %s not supported", gcpRegion)
	return "", &RegionError{Message: msg}
}
