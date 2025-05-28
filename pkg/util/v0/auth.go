package v0

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	ocicontainerengine "github.com/oracle/oci-go-sdk/v65/containerengine"
)

// GenerateOkeToken generates a token for an OKE cluster.
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

	// set token time parameter
	tokenTime := time.Now().In(time.FixedZone("GMT", 0))
	tokenTimeString := tokenTime.Format("Mon, 02 Jan 2006 15:04:05 GMT")
	req.Header.Set("date", tokenTimeString)

	// calculate token expiration time
	// value is inferred by output of:
	// oci ce cluster generate-token --cluster-id <cluster-id> --region <region>
	// and is not configurable via API
	tokenExpirationTime := tokenTime.Add(time.Minute * 4)

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
