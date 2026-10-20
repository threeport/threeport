package secret

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// TestGetExternalSecretsSupportServiceYamlAnnotatesRole covers the external-secrets account.
func TestGetExternalSecretsSupportServiceYamlAnnotatesRole(t *testing.T) {
	config := &SecretInstanceConfig{}

	// omit the account when no role is supplied
	plain, err := config.getExternalSecretsSupportServiceYaml("")
	require.NoError(t, err)
	require.NotContains(t, plain, "eks.amazonaws.com/role-arn")

	// emit the external-secrets account before the component
	annotated, err := config.getExternalSecretsSupportServiceYaml("arn:aws:iam::123:role/secrets")
	require.NoError(t, err)
	require.Contains(t, annotated, "eks.amazonaws.com/role-arn: arn:aws:iam::123:role/secrets")
	require.Contains(t, annotated, "name: "+installer.SecretsManagerServiceAccountName)
	require.Contains(t, annotated, "namespace: "+installer.SecretsManagerServiceAccountNamespace)
	account := strings.Index(annotated, "kind: ServiceAccount")
	component := strings.Index(annotated, "kind: SecretsComponent")
	require.GreaterOrEqual(t, account, 0)
	require.Greater(t, component, account)
}
