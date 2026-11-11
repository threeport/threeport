package provider

import (
	"sort"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const eksAddonTypeToken = "aws:eks/addon:Addon"

// TestTheEksClusterRunsItsAddonsAsEksAddons covers the add-ons EKS installs
// with a cluster: the program takes each over as an EKS add-on on the version
// EKS gives for the cluster's Kubernetes version, overwriting the copy EKS
// installed, after the node group.
func TestTheEksClusterRunsItsAddonsAsEksAddons(t *testing.T) {
	for _, version := range []string{"1.35", "1.36"} {
		infra := testEksInfra(2)
		infra.KubernetesVersion = version
		mocks := runEksPulumiProgram(t, infra)

		addons := mocks.byType(eksAddonTypeToken)
		sort.Slice(addons, func(a, b int) bool { return addons[a].name < addons[b].name })
		require.Len(t, addons, 3)
		for i, name := range []string{"coredns", "kube-proxy", "vpc-cni"} {
			assert.Equal(t, "eks-test-"+name, addons[i].name)
			assert.Equal(t, name, addons[i].inputs["addonName"])
			assert.Equal(t, name+"-for-"+version, addons[i].inputs["addonVersion"], "the version EKS gives for %s", version)
			assert.Equal(t, "OVERWRITE", addons[i].inputs["resolveConflictsOnCreate"])
			assert.Equal(t, "OVERWRITE", addons[i].inputs["resolveConflictsOnUpdate"])
		}
	}

	// each comes after the node group
	var mu sync.Mutex
	dependencies := map[string]int{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		if err := ctx.RegisterStackTransformation(func(args *pulumi.ResourceTransformationArgs) *pulumi.ResourceTransformationResult {
			if args.Type != eksAddonTypeToken {
				return nil
			}
			options, err := pulumi.NewResourceOptions(args.Opts...)
			require.NoError(t, err)
			mu.Lock()
			dependencies[args.Name] = len(options.DependsOn)
			mu.Unlock()
			return nil
		}); err != nil {
			return err
		}
		return testEksInfra(2).pulumiProgram()(ctx)
	}, pulumi.WithMocks("eks", "test-stack", &eksRecordingMocks{}))
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"eks-test-vpc-cni": 1, "eks-test-coredns": 1, "eks-test-kube-proxy": 1}, dependencies)
}

// TestAnAddonsDefaultVersionIsTheOneForTheClustersVersion covers choosing the
// EBS CSI driver's version on an upgrade: the one EKS marks as the default for
// the cluster's version, not the newest or another version's default.
func TestAnAddonsDefaultVersionIsTheOneForTheClustersVersion(t *testing.T) {
	compatible := func(cluster string, isDefault bool) ekstypes.Compatibility {
		return ekstypes.Compatibility{ClusterVersion: aws.String(cluster), DefaultVersion: isDefault}
	}
	addons := []ekstypes.AddonInfo{{AddonVersions: []ekstypes.AddonVersionInfo{
		{AddonVersion: aws.String("v1.40.0-eksbuild.1"), Compatibilities: []ekstypes.Compatibility{compatible("1.36", false), compatible("1.35", false)}},
		{AddonVersion: aws.String("v1.39.0-eksbuild.1"), Compatibilities: []ekstypes.Compatibility{compatible("1.36", true), compatible("1.35", false)}},
		{AddonVersion: aws.String("v1.38.0-eksbuild.1"), Compatibilities: []ekstypes.Compatibility{compatible("1.35", true)}},
	}}}

	version, err := defaultAddonVersion(addons, "1.36")
	require.NoError(t, err)
	assert.Equal(t, "v1.39.0-eksbuild.1", version)
	version, err = defaultAddonVersion(addons, "1.35")
	require.NoError(t, err)
	assert.Equal(t, "v1.38.0-eksbuild.1", version)
	_, err = defaultAddonVersion(addons, "1.37")
	assert.ErrorContains(t, err, "no default version is offered for Kubernetes 1.37")
}
