package secret

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// SecretDefinitionConfig is a configuration object for
// secret definition reconciliation.
type SecretDefinitionConfig struct {
	r                *controller.Reconciler
	secretDefinition *v0.SecretDefinition
	log              *logr.Logger
}

// PushSecret pushes a secret to a secret store.
func (c *SecretDefinitionConfig) PushSecret() error {

	// push secret to secret store based
	// on the secret definition's provider
	switch {
	case c.secretDefinition.AwsAccountID != nil:
		if err := c.PushSecretToAwsSecretsManager(); err != nil {
			return fmt.Errorf("failed to push secret to AWS Secrets Manager: %w", err)
		}
	}

	return nil
}

// DeleteSecret pushes a secret to a secret store.
func (c *SecretDefinitionConfig) DeleteSecret() error {

	// delete secret from secret store based
	// on the secret definition's provider
	switch {
	case c.secretDefinition.AwsAccountID != nil:
		if err := c.DeleteSecretFromAwsSecretsManager(); err != nil {
			return fmt.Errorf("failed to delete secret from AWS Secrets Manager: %w", err)
		}
	}

	return nil
}

// PushSecretToAwsSecretsManager pushes a secret to AWS Secrets Manager.
func (c *SecretDefinitionConfig) PushSecretToAwsSecretsManager() error {

	// configure aws session
	awsAccount, err := client.GetAwsAccountByID(
		c.r.APIClient,
		c.r.APIServer,
		*c.secretDefinition.AwsAccountID,
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve AWS account by ID: %w", err)
	}

	// get aws config
	awsConfig, err := kube.GetAwsConfigFromAwsAccount(c.r.EncryptionKey, *awsAccount.DefaultRegion, awsAccount)
	if err != nil {
		return fmt.Errorf("failed to get AWS config from AWS account: %w", err)
	}

	// Create a Secrets Manager awssmClient
	awssmClient := secretsmanager.NewFromConfig(*awsConfig)

	// Define the secret name and value
	var secretData map[string]string
	if err = json.Unmarshal([]byte(*c.secretDefinition.Data), &secretData); err != nil {
		return fmt.Errorf("failed to unmarshal secret data")
	}

	// encrypt sensitive values
	var encryptionKey = os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		return errors.New("environment variable ENCRYPTION_KEY is not set")
	}

	// decrypt sensitive values
	decryptedDataMap, err := encryption.DecryptStringMap(encryptionKey, secretData)
	if err != nil {
		return fmt.Errorf("failed to decrypt secret data: %w", err)
	}

	// Marshal the map into JSON format
	jsonBytes, err := json.Marshal(decryptedDataMap)
	if err != nil {
		return fmt.Errorf("failed to marshal secret data: %w", err)
	}

	// Convert JSON bytes to a string
	jsonString := string(jsonBytes)

	// create secrets
	// get existing secrets
	batchGetSecretValueOutput, err := awssmClient.BatchGetSecretValue(
		context.Background(),
		&secretsmanager.BatchGetSecretValueInput{
			Filters: []types.Filter{
				{
					Key: types.FilterNameStringTypeName,
					Values: []string{
						*c.secretDefinition.Name,
					},
				},
			},
		})
	if err != nil {
		return fmt.Errorf("failed to batch get secret value: %w", err)
	}

	// ensure secret does not already exist
	for _, secret := range batchGetSecretValueOutput.SecretValues {
		if secret.Name == c.secretDefinition.Name {
			return fmt.Errorf("secret already exists")
		}
	}

	// Create input for the CreateSecret operation
	input := &secretsmanager.CreateSecretInput{
		Name:         c.secretDefinition.Name,
		SecretString: util.Ptr(jsonString),
	}

	// Call the CreateSecret operation
	_, err = awssmClient.CreateSecret(context.Background(), input)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	// TODO: implement this functionality using the external-secrets package
	// so we can re-use their implementation. This may not be feasible for the
	// AWS provider as it relies on the v1 aws sdk and currently throws
	// mysterious nil-pointer exceptions, however we may want to opt for this
	// approach for other providers.
	//
	// // configure secret store
	// store := &esv1beta1.SecretStore{
	// 	Spec: esv1beta1.SecretStoreSpec{
	// 		Provider: &esv1beta1.SecretStoreProvider{
	// 			AWS: &esv1beta1.AWSProvider{
	// 				Service: esv1beta1.AWSServiceSecretsManager,
	// 			},
	// 		},
	// 	},
	// }

	// // get aws provider
	// prov, err := util.GetAWSProvider(store)
	// if err != nil {
	// 	return err
	// }

	// // configure aws session
	// awsAccount, err := client.GetAwsAccountByID(
	// 	r.APIClient,
	// 	r.APIServer,
	// 	*secretDefinition.AwsAccountID,
	// )
	// if err != nil {
	// 	return fmt.Errorf("failed to retrieve AWS account by ID: %w", err)
	// }

	// // get aws config
	// awsConfig, err := kube.GetAwsConfigFromAwsAccount(r.EncryptionKey, "us-east-2", awsAccount)
	// if err != nil {
	// 	return fmt.Errorf("failed to get AWS config from AWS account: %w", err)
	// }

	// cfg := awsv1.NewConfig().WithRegion("eu-west-1").WithEndpointResolver(awsauth.ResolveEndpoint())
	// credentials, err := awsConfig.Credentials.Retrieve(context.Background())
	// if err != nil {
	// 	return fmt.Errorf("failed to retrieve AWS credentials: %w", err)
	// }
	// cfg.Credentials = v1credentials.NewStaticCredentials(
	// 	credentials.AccessKeyID,
	// 	credentials.SecretAccessKey,
	// 	credentials.SessionToken,
	// )
	// sess := &session.Session{Config: cfg}

	// secretsManager, err := awssecretsmanager.New(sess, cfg, prov.SecretsManager, true)
	// if err != nil {
	// 	return fmt.Errorf("failed to create secrets manager client: %w", err)
	// }

	// // create fake secret
	// fakeSecret := &corev1.Secret{
	// 	Data: map[string][]byte{
	// 		"key": []byte("value"),
	// 	},
	// }

	// // push secret
	// psd := v1alpha1.PushSecretData{
	// 	Match: v1alpha1.PushSecretMatch{
	// 		SecretKey: "key",
	// 		RemoteRef: v1alpha1.PushSecretRemoteRef{
	// 			RemoteKey: "remotekey",
	// 		},
	// 	},
	// }
	// if err := secretsManager.PushSecret(context.Background(), fakeSecret, psd); err != nil {
	// 	return err
	// }

	return nil
}

// DeleteSecretFromAwsSecretsManager deletes a secret from AWS Secrets Manager.
func (c *SecretDefinitionConfig) DeleteSecretFromAwsSecretsManager() error {

	// configure aws session
	awsAccount, err := client.GetAwsAccountByID(
		c.r.APIClient,
		c.r.APIServer,
		*c.secretDefinition.AwsAccountID,
	)
	if err != nil {
		return fmt.Errorf("failed to retrieve AWS account by ID: %w", err)
	}

	// get aws config
	awsConfig, err := kube.GetAwsConfigFromAwsAccount(c.r.EncryptionKey, *awsAccount.DefaultRegion, awsAccount)
	if err != nil {
		return fmt.Errorf("failed to get AWS config from AWS account: %w", err)
	}

	// Create a Secrets Manager awssmClient
	awssmClient := secretsmanager.NewFromConfig(*awsConfig)

	// get existing secrets
	batchGetSecretValueOutput, err := awssmClient.BatchGetSecretValue(
		context.Background(),
		&secretsmanager.BatchGetSecretValueInput{
			Filters: []types.Filter{
				{
					Key: types.FilterNameStringTypeName,
					Values: []string{
						*c.secretDefinition.Name,
					},
				},
			},
		})
	if err != nil {
		return fmt.Errorf("failed to batch get secret value: %w", err)
	}

	// ensure secret does not already exist
	var id *string
	for _, secret := range batchGetSecretValueOutput.SecretValues {
		if *secret.Name == *c.secretDefinition.Name {
			id = secret.ARN
			break
		}
	}
	if id == nil {
		return fmt.Errorf("secret does not exist")
	}

	// Create input for the CreateSecret operation
	input := &secretsmanager.DeleteSecretInput{
		SecretId: id,
	}

	// Call the CreateSecret operation
	_, err = awssmClient.DeleteSecret(context.Background(), input)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}
