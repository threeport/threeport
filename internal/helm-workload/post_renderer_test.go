package helmworkload

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// testPostRenderer returns a post renderer over a minimal definition and
// instance.
func testPostRenderer() *ThreeportPostRenderer {
	return &ThreeportPostRenderer{
		HelmWorkloadDefinition: &v0.HelmWorkloadDefinition{
			Definition: v0.Definition{Name: util.Ptr("a-chart")},
		},
		HelmWorkloadInstance: &v0.HelmWorkloadInstance{
			Common:           v0.Common{ID: util.Ptr(uint(7))},
			Instance:         v0.Instance{Name: util.Ptr("a-release")},
			ReleaseNamespace: util.Ptr("a-namespace"),
		},
	}
}

// renderedChart returns a manifest stream of several documents, as helm hands
// to a post renderer.
func renderedChart() *bytes.Buffer {
	var manifests strings.Builder
	for _, name := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight"} {
		manifests.WriteString(fmt.Sprintf(
			"---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: %s\ndata:\n  key: value\n",
			name,
		))
	}

	return bytes.NewBufferString(manifests.String())
}

// TestPostRendererIsDeterministic covers the property every comparison against
// a deployed release rests on.
//
// The renderer splits the manifest stream with helm's SplitManifests, which
// returns a map. Ranging over a map visits its keys in a random order, so
// without sorting the same chart renders to a different manifest on every run:
// helm records a new revision each upgrade and any diff against the deployed
// release reports a change that is not there.
func TestPostRendererIsDeterministic(t *testing.T) {
	renderer := testPostRenderer()

	first, err := renderer.Run(renderedChart())
	require.NoError(t, err)

	for attempt := 0; attempt < 20; attempt++ {
		again, err := renderer.Run(renderedChart())
		require.NoError(t, err)
		require.Equal(
			t, first.String(), again.String(),
			"the same chart rendered differently on attempt %d", attempt+1,
		)
	}
}

// the order has to be the chart's own, not merely a stable one, or the manifest
// no longer reflects the order helm rendered
func TestPostRendererKeepsChartOrder(t *testing.T) {
	rendered, err := testPostRenderer().Run(renderedChart())
	require.NoError(t, err)

	var order []int
	for _, name := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight"} {
		index := strings.Index(rendered.String(), "name: "+name)
		require.NotEqual(t, -1, index, "%s is missing from the rendered output", name)
		order = append(order, index)
	}

	for i := 1; i < len(order); i++ {
		require.Less(t, order[i-1], order[i], "the manifests are not in chart order")
	}
}
