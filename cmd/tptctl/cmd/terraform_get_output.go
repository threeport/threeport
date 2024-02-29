// originally generated by 'threeport-codegen api-model' but will not be regenerated - intended for modification

package cmd

import (
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// outputGetTerraformsCmd produces the tabular output for the
// 'tptctl get terraforms' command.
func outputGetTerraformsCmd(
	terraformInstances *[]v0.TerraformInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t TERRAFORM DEFINITION\t TERRAFORM INSTANCE\t AWS ACCOUNT\t STATUS\t AGE")
	metadataErr := false
	var terraformDefErr error
	var awsAccountErr error
	for _, terraformInst := range *terraformInstances {
		// get terraform definition name for instance
		var terraformDef string
		terraformDefinition, err := client.GetTerraformDefinitionByID(apiClient, apiEndpoint, *terraformInst.TerraformDefinitionID)
		if err != nil {
			metadataErr = true
			terraformDefErr = err
			terraformDef = "<error>"
		} else {
			terraformDef = *terraformDefinition.Name
		}
		// get AWS acocunt name
		var awsAccountName string
		awsAccount, err := client.GetAwsAccountByID(apiClient, apiEndpoint, *terraformInst.AwsAccountID)
		if err != nil {
			metadataErr = true
			awsAccountErr = err
			awsAccountName = "<error>"
		} else {
			awsAccountName = *awsAccount.Name
		}
		// get terraform instance status
		terraformInstStatus := "Reconciling"
		if *terraformInst.Reconciled {
			terraformInstStatus = "Healthy"
		}
		if *terraformInst.CreationFailed {
			terraformInstStatus = "Failed"
		}
		fmt.Fprintln(
			writer,
			terraformDef, "\t",
			terraformDef, "\t",
			*terraformInst.Name, "\t",
			awsAccountName, "\t",
			terraformInstStatus, "\t",
			util.GetAge(terraformInst.CreatedAt),
		)
	}
	writer.Flush()

	if metadataErr {
		multiError := util.MultiError{}
		if terraformDefErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving terraform definition info: %w", terraformDefErr),
			)
		}
		if awsAccountErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving AWS account info: %w", terraformDefErr),
			)
		}
		return multiError.Error()
	}

	return nil
}

// outputGetTerraformDefinitionsCmd produces the tabular output for the
// 'tptctl get terraform-definitions' command.
func outputGetTerraformDefinitionsCmd(
	terraformDefinitions *[]v0.TerraformDefinition,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t AGE")
	for _, terraformDefinition := range *terraformDefinitions {
		fmt.Fprintln(
			writer,
			*terraformDefinition.Name, "\t",
			util.GetAge(terraformDefinition.CreatedAt),
		)
	}
	writer.Flush()

	return nil
}

// outputGetTerraformInstancesCmd produces the tabular output for the
// 'tptctl get terraform-instances' command.
func outputGetTerraformInstancesCmd(
	terraformInstances *[]v0.TerraformInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t TERRAFORM DEFINITION\t AWS ACCOUNT NAME\t STATUS\t AGE")
	metadataErr := false
	var terraformDefErr error
	var awsAccountErr error
	var statusErr error
	for _, terraformInst := range *terraformInstances {
		// get terraform definition name for instance
		var terraformDef string
		terraformDefinition, err := client.GetTerraformDefinitionByID(apiClient, apiEndpoint, *terraformInst.TerraformDefinitionID)
		if err != nil {
			metadataErr = true
			terraformDefErr = err
			terraformDef = "<error>"
		} else {
			terraformDef = *terraformDefinition.Name
		}
		// get AWS acocunt name
		var awsAccountName string
		awsAccount, err := client.GetAwsAccountByID(apiClient, apiEndpoint, *terraformInst.AwsAccountID)
		if err != nil {
			metadataErr = true
			awsAccountErr = err
			awsAccountName = "<error>"
		} else {
			awsAccountName = *awsAccount.Name
		}
		// get terraform instance status
		terraformInstStatus := "Reconciling"
		if *terraformInst.Reconciled {
			terraformInstStatus = "Healthy"
		}
		if *terraformInst.CreationFailed {
			terraformInstStatus = "Failed"
		}
		fmt.Fprintln(
			writer,
			*terraformInst.Name, "\t",
			terraformDef, "\t",
			awsAccountName, "\t",
			terraformInstStatus, "\t",
			util.GetAge(terraformInst.CreatedAt),
		)
	}
	writer.Flush()

	if metadataErr {
		multiError := util.MultiError{}
		if terraformDefErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving terraform definition info: %w", terraformDefErr),
			)
		}
		if awsAccountErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving AWS account info: %w", terraformDefErr),
			)
		}
		if statusErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving terraform instance status: %w", statusErr),
			)
		}
		return multiError.Error()
	}

	return nil
}
