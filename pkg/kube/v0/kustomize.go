package v0

import (
	"fmt"
	"strings"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// KustomizeBase joins a workload definition's stored resource definitions into
// the base manifests an overlay is rendered against.
//
// The resource definitions are the base, not the definition's YAMLDocument.
// An edited YAMLDocument reaches them only once the definition's update has
// been reconciled, so until then the two can disagree, and an overlay checked
// against one while rendered against the other is checked against manifests
// nothing deploys - which rejects overlays that work and accepts overlays that
// do not. Every caller goes through here so they cannot drift apart.
func KustomizeBase(resourceDefinitions []v0.KubernetesWorkloadResourceDefinition) string {
	var baseDocs []string
	for _, resourceDefinition := range resourceDefinitions {
		if resourceDefinition.JSONDefinition == nil {
			continue
		}
		baseDocs = append(baseDocs, string(*resourceDefinition.JSONDefinition))
	}

	return strings.Join(baseDocs, "\n---\n")
}

// RenderKustomizeOverlay applies overlay (the body of a kustomization.yaml,
// without a resources: key) to baseYAML (a multi-document YAML string) and
// returns the resulting resources as individual JSON documents, one per
// resource - the same shape GetJsonResourcesFromYamlDoc returns. An empty
// overlay still runs the base manifests through kustomize (with no
// transformers configured beyond the injected resources: key), so the
// result is the base resources unchanged.
//
// Runs entirely in memory via an in-memory kustomize filesystem - no
// shell-out to a kustomize binary and no disk I/O.
func RenderKustomizeOverlay(baseYAML string, overlay string) ([][]byte, error) {
	fSys := filesys.MakeFsInMemory()

	if err := fSys.WriteFile("/base.yaml", []byte(baseYAML)); err != nil {
		return nil, fmt.Errorf("failed to write base manifest to in-memory filesystem: %w", err)
	}

	kustomization := "resources:\n  - base.yaml\n" + overlay
	if err := fSys.WriteFile("/kustomization.yaml", []byte(kustomization)); err != nil {
		return nil, fmt.Errorf("failed to write kustomization.yaml to in-memory filesystem: %w", err)
	}

	resMap, err := krusty.MakeKustomizer(krusty.MakeDefaultOptions()).Run(fSys, "/")
	if err != nil {
		return nil, fmt.Errorf("failed to render kustomize overlay: %w", err)
	}

	yamlOut, err := resMap.AsYaml()
	if err != nil {
		return nil, fmt.Errorf("failed to render kustomized resources to yaml: %w", err)
	}

	jsonObjects, err := GetJsonResourcesFromYamlDoc(string(yamlOut))
	if err != nil {
		return nil, fmt.Errorf("failed to convert kustomized resources to json: %w", err)
	}

	return jsonObjects, nil
}
