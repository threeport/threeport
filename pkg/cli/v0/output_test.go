package v0

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cliObject mirrors the shape of a tptctl config value struct: pointer fields,
// most of them unset on any given object.
type cliObject struct {
	Name                     *string
	InfraProvider            *string
	InfraProviderAccountName *string
	HighAvailability         *bool
}

// TestMarshalYaml_OmitsAbsentFields is the contract the removed
// json:",omitempty" tag used to provide for `tptctl get -o yaml`.
// sigs.k8s.io/yaml marshals through encoding/json, so it honored that tag; a
// direct yaml.Marshal would now print every unset pointer as an explicit null.
func TestMarshalYaml_OmitsAbsentFields(t *testing.T) {
	name, provider := "dev-0", "kind"

	marshaled, err := marshalYaml(cliObject{Name: &name, InfraProvider: &provider})
	require.NoError(t, err)

	rendered := string(marshaled)
	assert.NotContains(t, rendered, "null", "an unset field must be omitted, not printed as null")
	assert.Contains(t, rendered, "Name: dev-0")
	assert.Contains(t, rendered, "InfraProvider: kind")
	assert.NotContains(t, rendered, "HighAvailability")
	assert.NotContains(t, rendered, "InfraProviderAccountName")
}

// TestMarshalYaml_KeepsExplicitEmptyValues matches the JSON path: a non-nil
// pointer to "" is a present value and survives.
func TestMarshalYaml_KeepsExplicitEmptyValues(t *testing.T) {
	empty := ""

	marshaled, err := marshalYaml(cliObject{Name: &empty})
	require.NoError(t, err)

	assert.Equal(t, `Name: ""`, strings.TrimSpace(string(marshaled)))
}
