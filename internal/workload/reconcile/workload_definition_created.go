package reconcile

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ghodss/yaml"
	yamlv3 "gopkg.in/yaml.v3"
	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/controller"
)

// WorkloadDefinitionCreated performs reconciliation when a workload definition
// has been created.
func WorkloadDefinitionCreated(
	r *controller.Reconciler,
	workloadDefinition *v0.WorkloadDefinition,
) (*[]v0.WorkloadResourceDefinition, error) {
	// iterate over each resource in the yaml doc and construct a workload
	// resource definition
	decoder := yamlv3.NewDecoder(strings.NewReader(*workloadDefinition.YAMLDocument))
	var workloadResourceDefinitions []v0.WorkloadResourceDefinition
	var wrdConstructError error
	for {
		// decode the next resource, exit loop if the end has been reached
		var node yamlv3.Node
		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			wrdConstructError = fmt.Errorf("failed to decode yaml node in workload definition: %w", err)
			break
		}

		// marshal the yaml
		yamlContent, err := yamlv3.Marshal(&node)
		if err != nil {
			wrdConstructError = fmt.Errorf("failed to marshal yaml from workload definition: %w", err)
			break
		}

		// convert yaml to json
		jsonContent, err := yaml.YAMLToJSON(yamlContent)
		if err != nil {
			wrdConstructError = fmt.Errorf("failed to convert yaml to json: %w", err)
			break
		}

		// unmarshal the json into the type used by API
		var jsonDefinition datatypes.JSON
		if err := jsonDefinition.UnmarshalJSON(jsonContent); err != nil {
			wrdConstructError = fmt.Errorf("failed to unmarshal json to datatypes.JSON: %w", err)
			break
		}

		// build the workload resource definition and marshal to json
		workloadResourceDefinition := v0.WorkloadResourceDefinition{
			JSONDefinition:       &jsonDefinition,
			WorkloadDefinitionID: workloadDefinition.ID,
		}
		workloadResourceDefinitions = append(workloadResourceDefinitions, workloadResourceDefinition)
	}

	// if any workload resource definitions failed construction, abort
	if wrdConstructError != nil {
		return &workloadResourceDefinitions, fmt.Errorf("failed to construct workload resource definition objects: %w", wrdConstructError)
	}

	// create workload resource definitions in API
	wrds, err := client.CreateWorkloadResourceDefinitions(
		&workloadResourceDefinitions,
		r.APIServer,
		"",
	)
	if err != nil {
		return &workloadResourceDefinitions, fmt.Errorf("failed to create workload resource definitions in API: %w", wrdConstructError)
	}

	return wrds, nil
}
