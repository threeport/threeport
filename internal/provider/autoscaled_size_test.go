package provider

import (
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ignoredChangesByType runs a Pulumi program against mocks and returns the
// IgnoreChanges option of each resource it registers, keyed by type token.
// The options are read through a stack transformation, which sees the options
// each resource is constructed with; the mocks only see its inputs.
func ignoredChangesByType(
	t *testing.T,
	program pulumi.RunFunc,
	mocks pulumi.MockResourceMonitor,
	project string,
) map[string][][]string {
	t.Helper()

	var mu sync.Mutex
	ignored := map[string][][]string{}
	recordOptions := func(args *pulumi.ResourceTransformationArgs) *pulumi.ResourceTransformationResult {
		options, err := pulumi.NewResourceOptions(args.Opts...)
		if err != nil {
			t.Errorf("failed to read the options of %s %s: %v", args.Type, args.Name, err)
			return nil
		}
		mu.Lock()
		ignored[args.Type] = append(ignored[args.Type], options.IgnoreChanges)
		mu.Unlock()
		return nil
	}

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		if err := ctx.RegisterStackTransformation(recordOptions); err != nil {
			return err
		}
		return program(ctx)
	}, pulumi.WithMocks(project, "test-stack", mocks))
	require.NoError(t, err)

	return ignored
}

// TestTheNodeGroupSizeIsSetOnlyWhenTheNodeGroupIsCreated checks that a later up
// leaves an autoscaled size alone. Every up on an existing stack runs after a
// refresh, so without the option the size the autoscaler reached would show as
// drift and be set back to the initial size.
func TestTheNodeGroupSizeIsSetOnlyWhenTheNodeGroupIsCreated(t *testing.T) {
	t.Run("eks", func(t *testing.T) {
		ignored := ignoredChangesByType(
			t, testEksInfra(2).pulumiProgram(), &eksRecordingMocks{}, "eks",
		)

		require.Len(t, ignored[eksNodeGroupTypeToken], 1)
		assert.Equal(t, []string{"scalingConfig.desiredSize"}, ignored[eksNodeGroupTypeToken][0])
		assertNothingElseIgnoresChanges(t, ignored, eksNodeGroupTypeToken)
	})

	t.Run("gke", func(t *testing.T) {
		infra := &KubernetesRuntimeInfraGKE{
			PulumiWorkspace: PulumiWorkspace{
				RuntimeInstanceName: "gke-size-test",
				ProjectName:         "gke",
			},
			ProjectID:              "test-project",
			Region:                 "us-central1",
			WorkerNodeInitialCount: 1,
			MinNodeCount:           1,
			MaxNodeCount:           3,
		}
		ignored := ignoredChangesByType(t, infra.pulumiProgram(), &gkeRecordingMocks{}, "gke")

		require.Len(t, ignored[gkeNodePoolTypeToken], 1)
		assert.Equal(t, []string{"nodeCount"}, ignored[gkeNodePoolTypeToken][0])
		assertNothingElseIgnoresChanges(t, ignored, gkeNodePoolTypeToken)
	})
}

// assertNothingElseIgnoresChanges checks that only the node pool ignores
// changes, so that a change to anything else in the stack is still applied.
func assertNothingElseIgnoresChanges(t *testing.T, ignored map[string][][]string, nodePoolType string) {
	t.Helper()

	require.Greater(t, len(ignored), 1, "the program registered no resource besides the node pool")
	for typeToken, perResource := range ignored {
		if typeToken == nodePoolType {
			continue
		}
		for _, ignoredChanges := range perResource {
			assert.Empty(t, ignoredChanges, "%s ignores changes", typeToken)
		}
	}
}
