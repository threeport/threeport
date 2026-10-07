package v0

import (
	"encoding/json"
	"fmt"
)

// GcpServiceAccountCredentialType is the credential type whose client_email is
// the account it authenticates as.
const GcpServiceAccountCredentialType = "service_account"

// GcpServiceAccountEmail returns the account a set of GCP service account
// credentials authenticates as.
//
// Reading it from the credentials rather than recording it alongside them keeps
// the two from disagreeing: the account that authenticates is the account named
// here, whatever else may have been stored.
//
// Only a service account credential is accepted. An authorized_user credential
// authenticates as whoever owns its refresh token, which client_email does not
// name even when one is present - so taking that email would name an account
// that makes no request. Somewhere like a cluster role binding, that grants
// access to an unrelated account while leaving the caller without it.
func GcpServiceAccountEmail(credentialsJSON string) (string, error) {
	var credentials struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
	}
	if err := json.Unmarshal([]byte(credentialsJSON), &credentials); err != nil {
		return "", fmt.Errorf("failed to unmarshal GCP service account credentials: %w", err)
	}
	if credentials.Type != GcpServiceAccountCredentialType {
		return "", fmt.Errorf(
			"GCP credentials are of type %q, and only %q names the account it authenticates as",
			credentials.Type, GcpServiceAccountCredentialType,
		)
	}
	if credentials.ClientEmail == "" {
		return "", fmt.Errorf("GCP service account credentials name no client_email")
	}

	return credentials.ClientEmail, nil
}
