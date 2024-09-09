// originally generated by 'threeport-sdk codegen api-model' but will not be regenerated - intended for modification

package cmd

import (
	"fmt"
	"net/http"
	"os"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

// outputDescribev0AwsAccountCmd produces the plain description
// output for the 'tptctl describe aws-account' command
func outputDescribev0AwsAccountCmd(
	awsAccount *v0.AwsAccount,
	awsAccountConfig *config.AwsAccountConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe AWS account
	awsAccountStatus, err := awsAccountConfig.AwsAccount.Describe(apiClient, apiEndpoint)
	if err != nil {
		cli.Error("failed to describe AWS account", err)
		os.Exit(1)
	}

	// output describe details
	fmt.Printf(
		"* AwsAccount Name: %s\n",
		awsAccountConfig.AwsAccount.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*awsAccount.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*awsAccount.UpdatedAt,
	)
	if *awsAccount.DefaultAccount {
		fmt.Println("* This AWS account is the default.  It will be used for AWS resources if not otherwise specified.")
	}
	fmt.Printf(
		"* Default AWS Region: %s.  This region will be used for this account unless otherwise specified.\n",
		*awsAccount.DefaultRegion,
	)
	fmt.Printf(
		"* AWS Account ID: %s\n",
		*awsAccount.AccountID,
	)
	if len(*awsAccountStatus.AwsEksKubernetesRuntimeDefinitions) == 0 {
		fmt.Println("* No AWS EKS Kubernetes runtime definitions currently reference this AWS account.")
	} else {
		fmt.Println("* AWS EKS Runtime Instances references this Account:")
		for _, eksRuntimeDef := range *awsAccountStatus.AwsEksKubernetesRuntimeDefinitions {
			fmt.Printf("  * %s\n", *eksRuntimeDef.Name)
		}
	}

	return nil
}

// outputDescribev0AwsEksKubernetesRuntimeDefinitionCmd produces the plain description
// output for the 'tptctl describe aws-eks-kubernetes-runtime-definition' command
func outputDescribev0AwsEksKubernetesRuntimeDefinitionCmd(
	awsEksKubernetesRuntimeDefinition *v0.AwsEksKubernetesRuntimeDefinition,
	awsEksKubernetesRuntimeDefinitionConfig *config.AwsEksKubernetesRuntimeDefinitionConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe AWS EKS kubernetes runtime definition
	awsEksKubernetesRuntimeStatus, err := awsEksKubernetesRuntimeDefinitionConfig.AwsEksKubernetesRuntimeDefinition.Describe(
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return fmt.Errorf("failed to describe AWS EKS kubernetes runtime definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* AwsEksKubernetesRuntimeDefinition Name: %s\n",
		awsEksKubernetesRuntimeDefinitionConfig.AwsEksKubernetesRuntimeDefinition.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*awsEksKubernetesRuntimeDefinition.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*awsEksKubernetesRuntimeDefinition.UpdatedAt,
	)
	fmt.Printf(
		"* Associated KubernetesRuntimeDefinition: %s\n",
		*awsEksKubernetesRuntimeStatus.KubernetesRuntimeDefinition.Name,
	)
	if len(*awsEksKubernetesRuntimeStatus.AwsEksKubernetesRuntimeInstances) == 0 {
		fmt.Println("* No AWS EKS kubernetes runtime instances currently derived from this definition.")
	} else {
		fmt.Println("* Derived AwsEksKubernetesRuntime Instances:")
		for _, awsEksKubernetesRuntimeInst := range *awsEksKubernetesRuntimeStatus.AwsEksKubernetesRuntimeInstances {
			fmt.Printf("  * %s\n", *awsEksKubernetesRuntimeInst.Name)
		}
	}

	return nil
}

// outputDescribev0AwsEksKubernetesRuntimeInstanceCmd produces the plain description
// output for the 'tptctl describe aws-eks-kubernetes-runtime-instance' command
func outputDescribev0AwsEksKubernetesRuntimeInstanceCmd(
	awsEksKubernetesRuntimeInstance *v0.AwsEksKubernetesRuntimeInstance,
	awsEksKubernetesRuntimeInstanceConfig *config.AwsEksKubernetesRuntimeInstanceConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe AWS EKS kubernetes runtime instance
	awsEksKubernetesRuntimeStatus, err := awsEksKubernetesRuntimeInstanceConfig.AwsEksKubernetesRuntimeInstance.Describe(
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return fmt.Errorf("failed to describe AWS EKS kubernetes runtime definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* AwsEksKubernetesRuntimeInstance Name: %s\n",
		awsEksKubernetesRuntimeInstanceConfig.AwsEksKubernetesRuntimeInstance.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*awsEksKubernetesRuntimeInstance.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*awsEksKubernetesRuntimeInstance.UpdatedAt,
	)
	fmt.Printf(
		"* Associated AwsEksKubernetesRuntimeDefinition: %s\n",
		*awsEksKubernetesRuntimeStatus.AwsEksKubernetesRuntimeDefinition.Name,
	)
	fmt.Printf(
		"* Associated KubernetesRuntimeInstance: %s\n",
		*awsEksKubernetesRuntimeStatus.KubernetesRuntimeInstance.Name,
	)

	return nil
}

// outputDescribev0AwsRelationalDatabaseDefinitionCmd produces the plain description
// output for the 'tptctl describe aws-relational-database-definition' command
func outputDescribev0AwsRelationalDatabaseDefinitionCmd(
	awsRelationalDatabaseDefinition *v0.AwsRelationalDatabaseDefinition,
	awsRelationalDatabaseDefinitionConfig *config.AwsRelationalDatabaseDefinitionConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// output describe details
	fmt.Printf(
		"* AwsRelationalDatabaseDefinition Name: %s\n",
		awsRelationalDatabaseDefinitionConfig.AwsRelationalDatabaseDefinition.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*awsRelationalDatabaseDefinition.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*awsRelationalDatabaseDefinition.UpdatedAt,
	)

	return nil
}

// outputDescribev0AwsRelationalDatabaseInstanceCmd produces the plain description
// output for the 'tptctl describe aws-relational-database-instance' command
func outputDescribev0AwsRelationalDatabaseInstanceCmd(
	awsRelationalDatabaseInstance *v0.AwsRelationalDatabaseInstance,
	awsRelationalDatabaseInstanceConfig *config.AwsRelationalDatabaseInstanceConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// output describe details
	fmt.Printf(
		"* AwsRelationalDatabaseInstance Name: %s\n",
		awsRelationalDatabaseInstanceConfig.AwsRelationalDatabaseInstance.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*awsRelationalDatabaseInstance.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*awsRelationalDatabaseInstance.UpdatedAt,
	)

	return nil
}

// outputDescribev0AwsObjectStorageBucketDefinitionCmd produces the plain description
// output for the 'tptctl describe aws-object-storage-bucket-definition' command
func outputDescribev0AwsObjectStorageBucketDefinitionCmd(
	awsObjectStorageBucketDefinition *v0.AwsObjectStorageBucketDefinition,
	awsObjectStorageBucketDefinitionConfig *config.AwsObjectStorageBucketDefinitionConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe AWS EKS kubernetes runtime definition
	awsObjectStorageBucketStatus, err := awsObjectStorageBucketDefinitionConfig.AwsObjectStorageBucketDefinition.Describe(
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return fmt.Errorf("failed to describe AWS EKS kubernetes runtime definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* AwsObjectStorageBucketDefinition Name: %s\n",
		awsObjectStorageBucketDefinitionConfig.AwsObjectStorageBucketDefinition.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*awsObjectStorageBucketDefinition.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*awsObjectStorageBucketDefinition.UpdatedAt,
	)
	if len(*awsObjectStorageBucketStatus.AwsObjectStorageBucketInstances) == 0 {
		fmt.Println("* No AWS object storage bucket instances currently derived from this definition.")
	} else {
		fmt.Println("* Derived AwsObjectStorageBucket Instances:")
		for _, awsObjectStorageBucketInst := range *awsObjectStorageBucketStatus.AwsObjectStorageBucketInstances {
			fmt.Printf("  * %s\n", *awsObjectStorageBucketInst.Name)
		}
	}

	return nil
}

// outputDescribev0AwsObjectStorageBucketInstanceCmd produces the plain description
// output for the 'tptctl describe aws-object-storage-bucket-instance' command
func outputDescribev0AwsObjectStorageBucketInstanceCmd(
	awsObjectStorageBucketInstance *v0.AwsObjectStorageBucketInstance,
	awsObjectStorageBucketInstanceConfig *config.AwsObjectStorageBucketInstanceConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// output describe details
	fmt.Printf(
		"* AwsObjectStorageBucketInstance Name: %s\n",
		awsObjectStorageBucketInstanceConfig.AwsObjectStorageBucketInstance.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*awsObjectStorageBucketInstance.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*awsObjectStorageBucketInstance.UpdatedAt,
	)

	return nil
}
