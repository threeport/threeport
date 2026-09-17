package v0

import "testing"

// TestResolveKindAPIHostPort covers kind API host-port resolution for an
// explicit override and for the auth-derived default in both auth states.
func TestResolveKindAPIHostPort(t *testing.T) {
	// set cases for override and auth-derived default
	tests := []struct {
		name              string
		authEnabled       bool
		apiServerHostPort int
		wantPort          int
	}{
		{
			name:              "explicit override takes precedence with auth enabled",
			authEnabled:       true,
			apiServerHostPort: 8443,
			wantPort:          8443,
		},
		{
			name:              "explicit override takes precedence with auth disabled",
			authEnabled:       false,
			apiServerHostPort: 8443,
			wantPort:          8443,
		},
		{
			name:              "no override falls back to auth-enabled default",
			authEnabled:       true,
			apiServerHostPort: 0,
			wantPort:          443,
		},
		{
			name:              "no override falls back to auth-disabled default",
			authEnabled:       false,
			apiServerHostPort: 0,
			wantPort:          80,
		},
	}

	// run each case
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// resolve the host port
			gotPort := ResolveKindAPIHostPort(tt.authEnabled, tt.apiServerHostPort)

			// expect the wanted port
			if gotPort != tt.wantPort {
				t.Errorf("expected port %d, got %d", tt.wantPort, gotPort)
			}
		})
	}
}
