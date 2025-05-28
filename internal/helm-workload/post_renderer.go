package helmworkload

import (
	"bytes"
	"encoding/json"
	"fmt"

	"gorm.io/datatypes"
	"helm.sh/helm/v3/pkg/releaseutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	yamlserailizer "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/yaml"

	"github.com/threeport/threeport/internal/agent"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ThreeportPostRenderer implements the postrender.PostRenderer interface for
// Threeport purposes.
type ThreeportPostRenderer struct {
	HelmWorkloadDefinition *v0.HelmWorkloadDefinition
	HelmWorkloadInstance   *v0.HelmWorkloadInstance
}

// Run modifies the rendered manifests to add threeport labels that allow the
// threeport agent to monitor the workload.
func (p *ThreeportPostRenderer) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	splitManifests := releaseutil.SplitManifests(renderedManifests.String())
	var postRenderedManifests string
	for _, manifest := range splitManifests {
		// Skip completely empty manifests
		trimmedManifest := bytes.TrimSpace([]byte(manifest))
		if len(trimmedManifest) == 0 {
			continue
		}

		// Check if this is just a YAML separator or only contains comments
		lines := bytes.Split(trimmedManifest, []byte("\n"))
		onlyCommentsOrEmpty := true
		for _, line := range lines {
			lineContent := bytes.TrimSpace(line)
			if len(lineContent) > 0 && !bytes.HasPrefix(lineContent, []byte("#")) && string(lineContent) != "---" {
				onlyCommentsOrEmpty = false
				break
			}
		}

		if onlyCommentsOrEmpty {
			continue
		}

		// convert to unstructured kube object
		serializer := yamlserailizer.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
		kubeObject := &unstructured.Unstructured{}
		_, _, err := serializer.Decode([]byte(manifest), nil, kubeObject)
		if err != nil {
			// Check if this is a YAML document with "---" but no content
			if len(bytes.TrimSpace(trimmedManifest)) == 0 || string(bytes.TrimSpace(trimmedManifest)) == "---" {
				continue
			}

			// For other decoding errors, return the error
			return nil, fmt.Errorf("failed to decode YAML manifest to unstructured object: %w", err)
		}

		// Check if the object has kind and apiVersion
		if kubeObject.GetKind() == "" || kubeObject.GetAPIVersion() == "" {
			return nil, fmt.Errorf("invalid Kubernetes manifest: missing Kind or apiVersion")
		}

		// Set the namespace - SetNamespace will only set it for namespaced resources
		if p.HelmWorkloadInstance.ReleaseNamespace != nil {
			kubeObject.SetNamespace(*p.HelmWorkloadInstance.ReleaseNamespace)
		}

		// set label metadata on object to signal threeport agent
		kubeObject, err = kube.AddLabels(
			kubeObject,
			*p.HelmWorkloadDefinition.Name,
			*p.HelmWorkloadInstance.Name,
			*p.HelmWorkloadInstance.ID,
			agent.HelmWorkloadInstanceLabelKey,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to add label metadata in post rendering: %w", err)
		}

		// convert unstructured kube object back to yaml
		yamlBytes, err := yaml.Marshal(kubeObject)
		if err != nil {
			return nil, fmt.Errorf("failed to convert unstructured object back to YAML: %w", err)
		}

		// add to post-rendered manifests
		postRenderedManifests += "---\n"
		postRenderedManifests += string(yamlBytes)
	}

	// append additional resources to post-rendered manifests
	// from helm workload definition
	postRenderedManifests, err := p.AppendAdditionalResources(
		postRenderedManifests,
		p.HelmWorkloadDefinition.AdditionalResources,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to append additional resources from helm workload definition: %w", err)
	}

	// append additional resources to post-rendered manifests
	// from helm workload instance
	postRenderedManifests, err = p.AppendAdditionalResources(
		postRenderedManifests,
		p.HelmWorkloadInstance.AdditionalResources,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to append additional resources from helm workload instance: %w", err)
	}

	return bytes.NewBuffer([]byte(postRenderedManifests)), nil
}

// AppendAdditionalResources appends additional resources to the given manifests.
func (p *ThreeportPostRenderer) AppendAdditionalResources(
	manifests string,
	additionalResources *datatypes.JSONSlice[datatypes.JSON],
) (string, error) {
	if additionalResources == nil {
		return manifests, nil
	}

	var output []string
	for _, additionalResource := range *additionalResources {
		var additionalResourceUnmarshaled map[string]interface{}
		err := json.Unmarshal([]byte(additionalResource), &additionalResourceUnmarshaled)
		if err != nil {
			return manifests, fmt.Errorf("failed to unmarshal vol json: %w", err)
		}

		// convert unstructured kube object back to yaml
		kubeObject := &unstructured.Unstructured{Object: additionalResourceUnmarshaled}
		kubeObject.SetNamespace(*p.HelmWorkloadInstance.ReleaseNamespace)
		yamlBytes, err := yaml.Marshal(kubeObject)
		if err != nil {
			return "", fmt.Errorf("failed to convert unstructured object back to YAML: %w", err)
		}

		output = append(output, string(yamlBytes))
	}

	return util.HyphenDelimitedString(output), nil
}
