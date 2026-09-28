package v0

import (
	"fmt"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

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
