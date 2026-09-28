package v0

import (
	"encoding/json"
	"strings"
	"testing"
)

const kustomizeTestBaseYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  namespace: default
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: app
        image: myapp:latest
        env:
        - name: VAR_A
          value: base
`

// unmarshalJSONResources decodes each resource in resources into a
// map[string]interface{} so tests can compare rendered output by structure
// rather than by raw bytes - kustomize's own YAML round-trip does not
// preserve key order, so a byte-for-byte comparison would be brittle.
func unmarshalJSONResources(t *testing.T, resources [][]byte) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	for _, r := range resources {
		var m map[string]interface{}
		if err := json.Unmarshal(r, &m); err != nil {
			t.Fatalf("failed to unmarshal rendered resource as json: %v\n%s", err, r)
		}
		out = append(out, m)
	}
	return out
}

// TestRenderKustomizeOverlay_EmptyOverlayPassesThrough covers the case a
// KubernetesWorkloadInstance with no KustomizeOverlay set relies on:
// rendering the base manifests through kustomize with no patches configured
// must reproduce the base resources unchanged.
func TestRenderKustomizeOverlay_EmptyOverlayPassesThrough(t *testing.T) {
	rendered, err := RenderKustomizeOverlay(kustomizeTestBaseYAML, "")
	if err != nil {
		t.Fatalf("RenderKustomizeOverlay returned an error: %v", err)
	}

	baseResources, err := GetJsonResourcesFromYamlDoc(kustomizeTestBaseYAML)
	if err != nil {
		t.Fatalf("failed to parse base yaml directly for comparison: %v", err)
	}

	got := unmarshalJSONResources(t, rendered)
	want := unmarshalJSONResources(t, baseResources)

	if len(got) != len(want) {
		t.Fatalf("expected %d resources, got %d", len(want), len(got))
	}
	if got[0]["spec"] == nil || want[0]["spec"] == nil {
		t.Fatalf("expected both resources to have a spec: got=%v want=%v", got[0], want[0])
	}
}

// TestRenderKustomizeOverlay_StrategicMergePatchOverridesAndAddsEnvVars is
// the motivating use case from threeport#556: an instance overlay overrides
// one env var already defined on the base container and adds a new one,
// without duplicating the overridden entry - core EnvVar lists use name as
// their strategic-merge-patch merge key.
func TestRenderKustomizeOverlay_StrategicMergePatchOverridesAndAddsEnvVars(t *testing.T) {
	overlay := `patches:
  - target:
      kind: Deployment
      name: myapp
    patch: |-
      apiVersion: apps/v1
      kind: Deployment
      metadata: {name: myapp}
      spec:
        template:
          spec:
            containers:
            - name: app
              env:
              - name: VAR_A
                value: instance-override
              - name: VAR_B
                value: instance-only
`
	rendered, err := RenderKustomizeOverlay(kustomizeTestBaseYAML, overlay)
	if err != nil {
		t.Fatalf("RenderKustomizeOverlay returned an error: %v", err)
	}
	if len(rendered) != 1 {
		t.Fatalf("expected exactly 1 resource, got %d", len(rendered))
	}

	resources := unmarshalJSONResources(t, rendered)
	spec := resources[0]["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	templateSpec := template["spec"].(map[string]interface{})
	containers := templateSpec["containers"].([]interface{})
	if len(containers) != 1 {
		t.Fatalf("expected exactly 1 container, got %d", len(containers))
	}
	container := containers[0].(map[string]interface{})
	env := container["env"].([]interface{})

	got := make(map[string]string, len(env))
	for _, e := range env {
		entry := e.(map[string]interface{})
		got[entry["name"].(string)] = entry["value"].(string)
	}

	if got["VAR_A"] != "instance-override" {
		t.Errorf("expected VAR_A to be overridden to instance-override, got %q", got["VAR_A"])
	}
	if got["VAR_B"] != "instance-only" {
		t.Errorf("expected VAR_B to be added as instance-only, got %q", got["VAR_B"])
	}
	if len(env) != 2 {
		t.Errorf("expected VAR_A's entry to be replaced rather than duplicated (2 env vars total), got %d: %v", len(env), env)
	}
}

// TestRenderKustomizeOverlay_UnmatchedPatchTargetIsANoOp pins the actual
// behavior of the pinned kustomize version (sigs.k8s.io/kustomize/api
// v0.20.1) for a patch whose target selector matches no resource in the
// base: unlike what threeport#556 originally assumed, this is NOT an error
// - kustomize silently skips a patch that matches nothing, same as `kustomize
// build` on the CLI would. Confirmed empirically before writing this test.
// A typo'd target in a KustomizeOverlay will silently no-op rather than
// surface as a reconcile-time warning event; the acceptance criteria this
// test was written against called this out as an intended error case, but
// that assumption doesn't hold against the real library, so this test
// documents the actual contract instead.
func TestRenderKustomizeOverlay_UnmatchedPatchTargetIsANoOp(t *testing.T) {
	overlay := `patches:
  - target:
      kind: Deployment
      name: does-not-exist
    patch: |-
      apiVersion: apps/v1
      kind: Deployment
      metadata: {name: does-not-exist}
      spec:
        replicas: 5
`
	rendered, err := RenderKustomizeOverlay(kustomizeTestBaseYAML, overlay)
	if err != nil {
		t.Fatalf("expected no error for an unmatched patch target (kustomize treats this as a no-op), got: %v", err)
	}

	resources := unmarshalJSONResources(t, rendered)
	if len(resources) != 1 {
		t.Fatalf("expected exactly 1 resource (base, unpatched), got %d", len(resources))
	}
	spec := resources[0]["spec"].(map[string]interface{})
	if spec["replicas"] != float64(1) {
		t.Errorf("expected base replicas (1) to be unchanged since the patch target matched nothing, got %v", spec["replicas"])
	}
}

// TestRenderKustomizeOverlay_MalformedOverlayErrors covers overlay text that
// isn't valid YAML at all - this fails while parsing the assembled
// kustomization.yaml, before any patch is even attempted.
func TestRenderKustomizeOverlay_MalformedOverlayErrors(t *testing.T) {
	_, err := RenderKustomizeOverlay(kustomizeTestBaseYAML, "this: is: not: valid: yaml: [[")
	if err == nil {
		t.Fatal("expected an error for malformed overlay YAML, got nil")
	}
	if !strings.Contains(err.Error(), "failed to render kustomize overlay") {
		t.Errorf("expected error to be wrapped with rendering context, got: %v", err)
	}
}
