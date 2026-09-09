package cmd

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildEventsQueryString covers encoding the subject flags as listing query parameters.
func TestBuildEventsQueryString(t *testing.T) {
	// flags is [for, object-kind, api-group, name, id, reason]
	tests := []struct {
		name     string
		flags    [6]string
		expected map[string]string
	}{
		{
			name:     "no flags query every event",
			flags:    [6]string{"", "", "", "", "", ""},
			expected: map[string]string{},
		},
		{
			name:     "kind alone",
			flags:    [6]string{"", "kubernetes-workload-instance", "", "", "", ""},
			expected: map[string]string{"objecttypename": "KubernetesWorkloadInstance"},
		},
		{
			name:     "name alone",
			flags:    [6]string{"", "", "", "my-app", "", ""},
			expected: map[string]string{"objectname": "my-app"},
		},
		{
			name:     "trailing star makes a name a prefix",
			flags:    [6]string{"", "", "", "myfleet2*", "", ""},
			expected: map[string]string{"objectnameprefix": "myfleet2"},
		},
		{
			name:     "id alone",
			flags:    [6]string{"", "", "", "", "7", ""},
			expected: map[string]string{"objectid": "7"},
		},
		{
			name:  "id narrowed by kind and api group",
			flags: [6]string{"", "kubernetes-workload-instance", "threeport.io", "", "7", ""},
			expected: map[string]string{
				"objecttypename":  "KubernetesWorkloadInstance",
				"objectnamespace": "threeport.io",
				"objectid":        "7",
			},
		},
		{
			name:     "for carries kind and name in one value",
			flags:    [6]string{"helm-workload-instance/my-app", "", "", "", "", ""},
			expected: map[string]string{"objecttypename": "HelmWorkloadInstance", "objectname": "my-app"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := test.flags
			encoded, err := buildEventsQueryString(f[0], f[1], f[2], f[3], f[4], f[5])
			require.NoError(t, err)

			// parse so keys compare without encoding order
			values, err := url.ParseQuery(encoded)
			require.NoError(t, err)

			assert.Len(t, values, len(test.expected))
			for key, want := range test.expected {
				assert.Equal(t, want, values.Get(key), "query key %s", key)
			}
		})
	}
}

// TestBuildEventsQueryStringRejectsANonNumericId rejects a non-numeric --id.
func TestBuildEventsQueryStringRejectsANonNumericId(t *testing.T) {
	_, err := buildEventsQueryString("", "", "", "", "not-a-number", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--id")
}
