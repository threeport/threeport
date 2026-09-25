package v0_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	sdkv0 "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"gopkg.in/yaml.v3"
)

// TestSwaggerDocsIncludeAPIFields checks that every API object from the SDK
// config is published, and that each of its fields is a swagger property.
func TestSwaggerDocsIncludeAPIFields(t *testing.T) {
	t.Chdir(moduleRoot(t))

	sdkConfig, err := sdkv0.GetSdkConfig("sdk-config.yaml")
	require.NoError(t, err)
	generator := &gen.Generator{}
	require.NoError(t, generator.New(sdkConfig))

	body, err := os.ReadFile(filepath.Join("pkg", "api-server", "v0", "docs", "swagger.yaml"))
	require.NoError(t, err)
	var doc struct {
		Definitions map[string]struct {
			Properties map[string]yaml.Node `yaml:"properties"`
		} `yaml:"definitions"`
	}
	require.NoError(t, yaml.Unmarshal(body, &doc))

	var missing []string
	for _, group := range generator.ApiObjectGroups {
		for _, object := range group.ApiObjects {
			def, ok := doc.Definitions["v0."+object.TypeName]
			if !ok {
				missing = append(missing, object.TypeName)
				continue
			}
			for field, tags := range group.StructTags[object.TypeName] {
				if tags["swaggerignore"] == "true" || tags["json"] == "-" {
					continue
				}
				if _, ok := def.Properties[field]; !ok {
					missing = append(missing, object.TypeName+"."+field)
				}
			}
		}
	}
	require.Empty(t, missing)
}

// moduleRoot walks up from the test working directory to the module root.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent)
		dir = parent
	}
}
