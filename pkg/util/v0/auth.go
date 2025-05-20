package v0

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	ocicontainerengine "github.com/oracle/oci-go-sdk/v65/containerengine"
)

// TODO: deduplicate and move elsewhere to avoid import loop
// generateToken generates a token for an OKE cluster.
func GenerateOkeToken(
	clusterID string,
	configProvider common.ConfigurationProvider,
) (string, time.Time, error) {

	// Get the region from the config
	region, err := configProvider.Region()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get region: %v", err)
	}

	// Create the container engine client
	client, err := ocicontainerengine.NewContainerEngineClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create client: %v", err)
	}

	// Construct the URL
	url := fmt.Sprintf("https://containerengine.%s.oraclecloud.com/cluster_request/%s", region, clusterID)

	// Create the initial request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create request: %v", err)
	}

	// Set the date header in RFC1123 format with GMT
	tokenExpirationTime := time.Now().In(time.FixedZone("GMT", 0))
	tokenExpiration := tokenExpirationTime.Format("Mon, 02 Jan 2006 15:04:05 GMT")
	req.Header.Set("date", tokenExpiration)

	// Sign the request
	err = client.Signer.Sign(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign request: %v", err)
	}

	// Get the authorization and date headers
	headerParams := map[string]string{
		"authorization": req.Header.Get("authorization"),
		"date":          req.Header.Get("date"),
	}

	// Create the token request with the headers as query parameters
	tokenReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create token request: %v", err)
	}

	// Add the headers as query parameters
	q := tokenReq.URL.Query()
	for key, value := range headerParams {
		q.Add(key, value)
	}
	tokenReq.URL.RawQuery = q.Encode()

	// Encode the URL as the token
	token := base64.URLEncoding.EncodeToString([]byte(tokenReq.URL.String()))

	return token, tokenExpirationTime, nil
}
