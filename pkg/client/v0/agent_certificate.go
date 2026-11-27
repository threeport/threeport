package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// CreateAgentCertificate asks the Threeport API to issue a client certificate
// to the agent on the named kubernetes runtime instance, for the public key in
// the PEM-encoded certificate signing request.  Only the kubernetes runtime
// controller is allowed to make this call.  The API chooses the certificate's
// identity; the request only supplies the key.
func CreateAgentCertificate(
	apiClient *http.Client,
	apiAddr string,
	runtimeInstanceName string,
	certificateSigningRequest string,
) (*v0.AgentCertificate, error) {
	jsonRequest, err := json.Marshal(v0.AgentCertificateRequest{
		RuntimeInstanceName:       runtimeInstanceName,
		CertificateSigningRequest: certificateSigningRequest,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal agent certificate request to JSON: %w", err)
	}

	response, err := client_lib.GetResponse(
		apiClient,
		fmt.Sprintf("%s%s", apiAddr, v0.PathAgentCertificates),
		http.MethodPost,
		bytes.NewBuffer(jsonRequest),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return nil, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	if len(response.Data) < 1 {
		return nil, fmt.Errorf("threeport API returned no agent certificate")
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	var certificate v0.AgentCertificate
	if err := json.Unmarshal(jsonData, &certificate); err != nil {
		return nil, fmt.Errorf("failed to decode agent certificate in response data from threeport API: %w", err)
	}
	if certificate.Certificate == "" {
		return nil, fmt.Errorf("threeport API returned an empty agent certificate")
	}

	return &certificate, nil
}
