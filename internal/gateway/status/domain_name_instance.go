package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// DomainNameInstanceStatusDetail contains all the data for
// domain name instance status info.
type DomainNameInstanceStatusDetail struct {
	DomainNameDefinition *v0.DomainNameDefinition
}

// GetDomainNameInstanceStatus inspects a domain name instance
// and returns the status detials for it.
func GetDomainNameInstanceStatus(
	apiClient *http.Client,
	apiEndpoint string,
	domainNameInstance *v0.DomainNameInstance,
) (*DomainNameInstanceStatusDetail, error) {
	var domainNameInstStatus DomainNameInstanceStatusDetail

	// retrieve domain name definition for the instance
	domainNameDef, err := client.GetDomainNameDefinitionByID(
		apiClient,
		apiEndpoint,
		*domainNameInstance.DomainNameDefinitionID,
	)
	if err != nil {
		return &domainNameInstStatus, fmt.Errorf("failed to retrieve domain name definition related to domain name instance: %w", err)
	}
	domainNameInstStatus.DomainNameDefinition = domainNameDef

	return &domainNameInstStatus, nil
}
