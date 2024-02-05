package secret

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	esv1beta1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1beta1"
	awsauth "github.com/external-secrets/external-secrets/pkg/provider/aws/auth"
	awssecretsmanager "github.com/external-secrets/external-secrets/pkg/provider/aws/secretsmanager"
	"github.com/external-secrets/external-secrets/pkg/provider/aws/util"
	fake "github.com/external-secrets/external-secrets/pkg/provider/testing/fake"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// secretDefinitionCreated reconciles state for a new secret
// definition.
func secretDefinitionCreated(
	r *controller.Reconciler,
	secretDefinition *v0.SecretDefinition,
	log *logr.Logger,
) (int64, error) {

	switch *secretDefinition.Provider {
	case "aws":
		// configure secret store
		store := &esv1beta1.SecretStore{
			Spec: esv1beta1.SecretStoreSpec{
				Provider: &esv1beta1.SecretStoreProvider{
					AWS: &esv1beta1.AWSProvider{
						Service: esv1beta1.AWSServiceSecretsManager,
					},
				},
			},
		}

		// get aws provider
		prov, err := util.GetAWSProvider(store)
		if err != nil {
			return 0, err
		}

		// configure aws session
		cfg := aws.NewConfig().WithRegion("eu-west-1").WithEndpointResolver(awsauth.ResolveEndpoint())
		sess := &session.Session{Config: cfg}
		secretsManager, err := awssecretsmanager.New(sess, cfg, prov.SecretsManager, true)
		if err != nil {
			return 0, fmt.Errorf("failed to create secrets manager client: %w", err)
		}

		fakeSecret := &corev1.Secret{
			Data: map[string][]byte{
				"key": []byte("value"),
			},
		}

		// push secret
		if err := secretsManager.PushSecret(context.Background(), fakeSecret, fake.PushSecretData{}); err != nil {
			return 0, err
		}

	}
	return 0, nil
}

// secretDefinitionCreated reconciles state for a secret
// definition when it is changed.
func secretDefinitionUpdated(
	r *controller.Reconciler,
	secretDefinition *v0.SecretDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// secretDefinitionCreated reconciles state for a secret
// definition when it is removed.
func secretDefinitionDeleted(
	r *controller.Reconciler,
	secretDefinition *v0.SecretDefinition,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}
