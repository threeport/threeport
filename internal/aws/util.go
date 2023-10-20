package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/nukleros/eks-cluster/pkg/resource"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
)

// GetAwsConfigFromAwsAccount returns an aws config from an aws account.
func GetAwsConfigFromAwsAccount(encryptionKey, region string, awsAccount *v0.AwsAccount) (*aws.Config, error) {

	// load aws config via default credentials
	awsConfig, err := resource.LoadAWSConfigFromAPIKeys("", "", "", region, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}

	// get caller identity
	svc := sts.NewFromConfig(*awsConfig)
	callerIdentity, err := svc.GetCallerIdentity(
		context.Background(),
		&sts.GetCallerIdentityInput{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %w", err)
	}

	// if caller identity is an assumed role in the current AWS account,
	// return the default aws config. This will always be the case when
	// this function is called within a control plane hosted in EKS, as the
	// pod will be authenticated via IRSA to an IAM role.
	// https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html
	if strings.Contains(*callerIdentity.Arn, "assumed-role") &&
		*callerIdentity.Account == *awsAccount.AccountID {
		return awsConfig, nil
	}

	roleArn := ""
	externalId := ""
	accessKeyID := ""
	secretAccessKey := ""

	// if a role arn is provided, use it
	if awsAccount.RoleArn != nil {
		roleArn = *awsAccount.RoleArn

		// if an external ID is provided with role arn, use it
		if awsAccount.ExternalId != nil {
			externalId = *awsAccount.ExternalId
		}
	}

	// if keys are provided, decrypt and return aws config
	if awsAccount.AccessKeyID != nil && awsAccount.SecretAccessKey != nil {

		// decrypt access key id and secret access key
		accessKeyID, err = encryption.Decrypt(encryptionKey, *awsAccount.AccessKeyID)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt access key id: %w", err)
		}
		secretAccessKey, err = encryption.Decrypt(encryptionKey, *awsAccount.SecretAccessKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt secret access key: %w", err)
		}
	}

	// construct aws config given values
	awsConfig, err = resource.LoadAWSConfigFromAPIKeys(
		accessKeyID,
		secretAccessKey,
		"",
		region,
		roleArn,
		"",
		externalId,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config from API keys: %w", err)
	}
	return awsConfig, nil

}
