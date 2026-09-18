package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateCreateGenesisControlPlaneFlags_ApiPort covers the ways --api-port
// can be wrong. Each is otherwise found late: a cloud provider ignores it, an
// out-of-range port fails when kind writes its config, and naming the port
// twice produces whichever of the two the code happened to prefer.
func TestValidateCreateGenesisControlPlaneFlags_ApiPort(t *testing.T) {
	tests := []struct {
		name             string
		infraProvider    string
		apiPort          int
		kindPortMappings []string
		wantErr          string
	}{
		{
			name:          "unset is fine",
			infraProvider: "kind",
		},
		{
			name:          "a port on kind is fine",
			infraProvider: "kind",
			apiPort:       8443,
		},
		{
			name:          "the privileged port can still be asked for",
			infraProvider: "kind",
			apiPort:       443,
		},
		{
			name:          "not for a cloud provider",
			infraProvider: "eks",
			apiPort:       8443,
			wantErr:       "only supported for infrastructure provider 'kind'",
		},
		{
			name:          "out of range",
			infraProvider: "kind",
			apiPort:       70000,
			wantErr:       "must be between 1 and 65535",
		},
		{
			name:             "named twice",
			infraProvider:    "kind",
			apiPort:          8443,
			kindPortMappings: []string{"30000:9443"},
			wantErr:          "both set the host port for the threeport API",
		},
		{
			name:             "a container port mapped twice",
			infraProvider:    "kind",
			kindPortMappings: []string{"30000:9443", "30000:9444"},
			wantErr:          "mapped more than once",
		},
		{
			name:             "a non-api container port mapped twice",
			infraProvider:    "kind",
			kindPortMappings: []string{"30001:4443", "30001:4444"},
			wantErr:          "mapped more than once",
		},
		{
			name:             "another mapping alongside it is fine",
			infraProvider:    "kind",
			apiPort:          8443,
			kindPortMappings: []string{"30001:4443"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateCreateGenesisControlPlaneFlags(
				"test",
				test.infraProvider,
				"",
				true,
				test.kindPortMappings,
				false,
				"",
				test.apiPort,
			)
			if test.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

// TestApiPortFromKindMappings covers folding the two ways a user can name the
// API's host port into one value. Without it the kind mapping moved the port
// while the threeport config went on recording the default, so tptctl wrote a
// config it could not then connect with.
func TestApiPortFromKindMappings(t *testing.T) {
	tests := []struct {
		name     string
		mappings []string
		want     int
	}{
		{name: "none", mappings: nil, want: 0},
		{name: "another port only", mappings: []string{"30001:4443"}, want: 0},
		{name: "the api port", mappings: []string{"30000:9443"}, want: 9443},
		{
			name:     "the api port among others",
			mappings: []string{"30001:4443", "30000:9443", "30002:9180"},
			want:     9443,
		},
		{
			name:     "malformed is left to the parser that reports it",
			mappings: []string{"30000", "30000:not-a-port"},
			want:     0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, apiPortFromKindMappings(test.mappings))
		})
	}
}
