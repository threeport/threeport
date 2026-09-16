package provider

import (
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	gkeClusterTypeToken  = "gcp:container/cluster:Cluster"
	gkeNodePoolTypeToken = "gcp:container/nodePool:NodePool"
	gkeNetworkTypeToken  = "gcp:compute/network:Network"
	gkeSubnetTypeToken   = "gcp:compute/subnetwork:Subnetwork"
	gkeRouterTypeToken   = "gcp:compute/router:Router"
	gkeNatTypeToken      = "gcp:compute/routerNat:RouterNat"
	gkeProviderTypeToken = "pulumi:providers:gcp"
)

type gkeRecordedResource struct {
	typeToken string
	name      string
	inputs    map[string]any
}

type gkeRecordingMocks struct {
	mu        sync.Mutex
	resources []gkeRecordedResource
}

func (m *gkeRecordingMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, gkeRecordedResource{
		typeToken: args.TypeToken,
		name:      args.Name,
		inputs:    args.Inputs.Mappable(),
	})
	m.mu.Unlock()
	return args.Name + "-id", args.Inputs, nil
}

func (m *gkeRecordingMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

// TestGKEPulumiProgram_LabelableResourcesCarryProvisionedBy covers cluster
// and node-pool GCP labels, and documents types with no Labels field.
func TestGKEPulumiProgram_LabelableResourcesCarryProvisionedBy(t *testing.T) {
	i := &KubernetesRuntimeInfraGKE{
		PulumiWorkspace: PulumiWorkspace{
			RuntimeInstanceName: "gke-label-test",
			ProjectName:         "gke",
		},
		ProjectID:              "test-project",
		Region:                 "us-central1",
		WorkerNodeInitialCount: 1,
	}

	mocks := &gkeRecordingMocks{}
	if err := pulumi.RunErr(i.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", mocks)); err != nil {
		t.Fatalf("RunErr: %v", err)
	}
	if len(mocks.resources) == 0 {
		t.Fatal("program registered no resources; nothing to test")
	}

	type labelExpectation struct {
		labelable    bool
		exemptReason string
		labelsFrom   string
	}
	allowlist := map[string]labelExpectation{
		gkeClusterTypeToken:  {labelable: true, labelsFrom: "resourceLabels"},
		gkeNodePoolTypeToken: {labelable: true, labelsFrom: "nodeConfig.resourceLabels"},
		gkeNetworkTypeToken: {
			exemptReason: "compute NetworkArgs has no labels field",
		},
		gkeSubnetTypeToken: {
			exemptReason: "compute SubnetworkArgs has no labels field",
		},
		gkeRouterTypeToken: {
			exemptReason: "compute RouterArgs has no labels field",
		},
		gkeNatTypeToken: {
			exemptReason: "compute RouterNatArgs has no labels field",
		},
		gkeProviderTypeToken: {
			exemptReason: "the cloud-provider meta-resource carries no user labels",
		},
	}

	want := GcpResourceLabels(i.RuntimeInstanceName)
	sawCluster := false
	sawNodePool := false

	for _, r := range mocks.resources {
		exp, ok := allowlist[r.typeToken]
		if !ok {
			t.Errorf("resource %q (%s) is not in the label allowlist; classify it as labelable or exempt", r.name, r.typeToken)
			continue
		}
		if !exp.labelable {
			if exp.exemptReason == "" {
				t.Errorf("resource type %s is exempt but carries no documented reason", r.typeToken)
			}
			continue
		}

		labels := gkeGCPLabels(t, r, exp.labelsFrom)
		for k, v := range want {
			if got := labels[k]; got != v {
				t.Errorf("resource %q (%s) %s[%q] = %q, want %q", r.name, r.typeToken, exp.labelsFrom, k, got, v)
			}
		}
		if r.typeToken == gkeClusterTypeToken {
			sawCluster = true
		}
		if r.typeToken == gkeNodePoolTypeToken {
			sawNodePool = true
		}
	}

	if !sawCluster {
		t.Error("the GKE cluster resource was not registered")
	}
	if !sawNodePool {
		t.Error("the GKE node pool resource was not registered")
	}
}

// gkeGCPLabels returns a nested labels map from recorded Pulumi inputs.
func gkeGCPLabels(t *testing.T, r gkeRecordedResource, path string) map[string]string {
	t.Helper()
	var raw any
	switch path {
	case "resourceLabels":
		raw = r.inputs["resourceLabels"]
	case "nodeConfig.resourceLabels":
		nc, ok := r.inputs["nodeConfig"].(map[string]any)
		if !ok {
			t.Fatalf("resource %q nodeConfig is not a map: %T", r.name, r.inputs["nodeConfig"])
		}
		raw = nc["resourceLabels"]
	default:
		t.Fatalf("unknown labels path %q", path)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("resource %q %s is not a map: %T", r.name, path, raw)
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("resource %q %s[%q] is not a string: %T", r.name, path, k, v)
		}
		out[k] = s
	}
	return out
}
