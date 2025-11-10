package v0

import (
	"fmt"

	"github.com/threeport/threeport/pkg/api-server/v0/database"
)

// DbCreds contains the DB client connection credentials.
type DbCreds struct {
	AuthConfig    *AuthConfig
	NodeCert      string
	NodeKey       string
	RootCert      string
	RootKey       string
	ThreeportCert string
	ThreeportKey  string
}

// GenerateDbCreds generates the CA cert and derived certs for the CRDB nodes,
// the root DB user and the threeport user for database auth.
func GenerateDbCreds(k8sNamespace string) (*DbCreds, error) {
	dbAuthConfig, err := GetAuthConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to generate auth config for DB client cert: %w", err)
	}

	nodeCert, nodeKey, err := GenerateCertificate(
		dbAuthConfig.CAConfig,
		&dbAuthConfig.CAPrivateKey,
		"node",
		"crdb",
		fmt.Sprintf("crdb.%s", k8sNamespace),
		fmt.Sprintf("crdb.%s.svc.cluster.local", k8sNamespace),
		"*.crdb",
		fmt.Sprintf("*.crdb.%s", k8sNamespace),
		fmt.Sprintf("*.crdb.%s.svc.cluster.local", k8sNamespace),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate DB node certificate: %w", err)
	}

	rootCert, rootKey, err := GenerateCertificate(
		dbAuthConfig.CAConfig,
		&dbAuthConfig.CAPrivateKey,
		database.ThreeportDatabaseRootUser,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate DB client certificate: %w", err)
	}

	threeportCert, threeportKey, err := GenerateCertificate(
		dbAuthConfig.CAConfig,
		&dbAuthConfig.CAPrivateKey,
		database.ThreeportDatabaseUser,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate DB client certificate: %w", err)
	}

	dbCreds := DbCreds{
		AuthConfig:    dbAuthConfig,
		NodeCert:      nodeCert,
		NodeKey:       nodeKey,
		RootCert:      rootCert,
		RootKey:       rootKey,
		ThreeportCert: threeportCert,
		ThreeportKey:  threeportKey,
	}

	return &dbCreds, nil
}
