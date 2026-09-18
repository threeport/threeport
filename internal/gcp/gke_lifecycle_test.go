package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServiceAccountEmailFromCredentials(t *testing.T) {
	cases := []struct {
		name          string
		credentials   string
		expectedEmail string
		expectError   bool
	}{
		{
			name:          "valid credentials",
			credentials:   `{"type":"service_account","client_email":"test-sa@some-project.iam.gserviceaccount.com","private_key":"redacted"}`,
			expectedEmail: "test-sa@some-project.iam.gserviceaccount.com",
			expectError:   false,
		},
		{
			name:        "malformed JSON",
			credentials: `{"type":"service_account", not valid json`,
			expectError: true,
		},
		{
			name:        "missing client_email field",
			credentials: `{"type":"service_account","private_key":"redacted"}`,
			expectError: true,
		},
		{
			name:        "empty client_email field",
			credentials: `{"type":"service_account","client_email":"","private_key":"redacted"}`,
			expectError: true,
		},
		{
			name:        "empty string",
			credentials: "",
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			email, err := serviceAccountEmailFromCredentials(tc.credentials)
			if tc.expectError {
				assert.Error(t, err)
				assert.Empty(t, email)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedEmail, email)
		})
	}
}
