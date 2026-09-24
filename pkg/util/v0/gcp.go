package v0

import (
	"encoding/json"
	"fmt"
)

// GcpServiceAccountEmail returns the account a set of GCP service account
// credentials authenticates as.
//
// Reading it from the credentials rather than recording it alongside them keeps
// the two from disagreeing: the account that authenticates is the account named
// here, whatever else may have been stored.
func GcpServiceAccountEmail(credentialsJSON string) (string, error) {
	var credentials struct {
		ClientEmail string `json:"client_email"`
	}
	if err := json.Unmarshal([]byte(credentialsJSON), &credentials); err != nil {
		return "", fmt.Errorf("failed to unmarshal GCP service account credentials: %w", err)
	}
	if credentials.ClientEmail == "" {
		return "", fmt.Errorf("GCP service account credentials name no client_email")
	}

	return credentials.ClientEmail, nil
}
