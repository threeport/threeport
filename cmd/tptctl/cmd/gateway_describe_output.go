// originally generated by 'threeport-sdk codegen api-model' but will not be regenerated - intended for modification

package cmd

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

// outputDescribeGatewayDefinitionCmd produces the plain description
// output for the 'tptctl describe gateway-definition' command
func outputDescribeGatewayDefinitionCmd(
	gatewayDefinition *v0.GatewayDefinition,
	gatewayDefinitionConfig *config.GatewayDefinitionConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe gateway definition
	gatewayStatus, err := gatewayDefinitionConfig.GatewayDefinition.Describe(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to describe gateway definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* GatewayDefinition Name: %s\n",
		gatewayDefinitionConfig.GatewayDefinition.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*gatewayDefinition.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*gatewayDefinition.UpdatedAt,
	)
	if len(*gatewayStatus.GatewayInstances) == 0 {
		fmt.Println("* No gateway instances currently derived from this definition.")
	} else {
		fmt.Println("* Derived Gateway Instances:")
		for _, gatewayInst := range *gatewayStatus.GatewayInstances {
			fmt.Printf("  * %s\n", *gatewayInst.Name)
		}
	}

	return nil
}

// outputDescribeGatewayInstanceCmd produces the plain description
// output for the 'tptctl describe gateway-instance' command
func outputDescribeGatewayInstanceCmd(
	gatewayInstance *v0.GatewayInstance,
	gatewayInstanceConfig *config.GatewayInstanceConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe gateway instance
	gatewayStatus, err := gatewayInstanceConfig.GatewayInstance.Describe(
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return fmt.Errorf("failed to describe gateway definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* GatewayInstance Name: %s\n",
		gatewayInstanceConfig.GatewayInstance.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*gatewayInstance.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*gatewayInstance.UpdatedAt,
	)
	fmt.Printf(
		"* Associated GatewayDefinition: %s\n",
		*gatewayStatus.GatewayDefinition.Name,
	)

	return nil
}

// outputDescribeDomainNameDefinitionCmd produces the plain description
// output for the 'tptctl describe domain-name-definition' command
func outputDescribeDomainNameDefinitionCmd(
	domainNameDefinition *v0.DomainNameDefinition,
	domainNameDefinitionConfig *config.DomainNameDefinitionConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe domain name definition
	domainNameStatus, err := domainNameDefinitionConfig.DomainNameDefinition.Describe(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to describe domain name definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* DomainNameDefinition Name: %s\n",
		domainNameDefinitionConfig.DomainNameDefinition.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*domainNameDefinition.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*domainNameDefinition.UpdatedAt,
	)
	if len(*domainNameStatus.DomainNameInstances) == 0 {
		fmt.Println("* No domain name instances currently derived from this definition.")
	} else {
		fmt.Println("* Derived DomainName Instances:")
		for _, domainNameInst := range *domainNameStatus.DomainNameInstances {
			fmt.Printf("  * %s\n", *domainNameInst.Name)
		}
	}

	return nil
}

// outputDescribeDomainNameInstanceCmd produces the plain description
// output for the 'tptctl describe domain-name-instance' command
func outputDescribeDomainNameInstanceCmd(
	domainNameInstance *v0.DomainNameInstance,
	domainNameInstanceConfig *config.DomainNameInstanceConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe domain name instance
	domainNameStatus, err := domainNameInstanceConfig.DomainNameInstance.Describe(
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return fmt.Errorf("failed to describe domain name definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* DomainNameInstance Name: %s\n",
		domainNameInstanceConfig.DomainNameInstance.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*domainNameInstance.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*domainNameInstance.UpdatedAt,
	)
	fmt.Printf(
		"* Associated DomainNameDefinition: %s\n",
		*domainNameStatus.DomainNameDefinition.Name,
	)

	return nil
}
