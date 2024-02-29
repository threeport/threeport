package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// DomainNameDefinitionStatusDetail contains all the data for domain name instance
// status info.
type DomainNameDefinitionStatusDetail struct {
	DomainNameInstances *[]v0.DomainNameInstance
}

// GetDomainNameDefinitionStatus inspects a domain name definition and returns the status
// detials for it.
func GetDomainNameDefinitionStatus(
	apiClient *http.Client,
	apiEndpoint string,
	domainNameDefinitionId uint,
) (*DomainNameDefinitionStatusDetail, error) {
	var domainNameDefStatus DomainNameDefinitionStatusDetail

	// retrieve domain name instances related to domain name definition
	domainNameInsts, err := client.GetDomainNameInstancesByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("domainnamedefinitionid=%d", domainNameDefinitionId),
	)
	if err != nil {
		return &domainNameDefStatus, fmt.Errorf("failed to retrieve domain name instances related to domain name definition: %w", err)
	}
	domainNameDefStatus.DomainNameInstances = domainNameInsts

	return &domainNameDefStatus, nil
}
