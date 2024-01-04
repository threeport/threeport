package radiusworkload

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/go-logr/logr"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// radiusWorkloadInstanceCreated reconciles state for a new radius workload
// definition.
func radiusWorkloadInstanceCreated(
	r *controller.Reconciler,
	radiusWorkloadInstance *v0.RadiusWorkloadInstance,
	log *logr.Logger,
) (int64, error) {
	// get radius workload definition
	radWorkloadDefinition, err := client.GetRadiusWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*radiusWorkloadInstance.RadiusWorkloadDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"failed to get radius workload definition with ID %d: %w",
			*radiusWorkloadInstance.RadiusWorkloadDefinitionID,
			err,
		)
	}

	// write bicep config to disk
	if err := os.WriteFile(
		"/tmp/app.bicep",
		[]byte(*radWorkloadDefinition.BicepDocument),
		0644,
	); err != nil {
		return 0, fmt.Errorf("failed to write bicep config: %w", err)
	}

	// write params.json to disk if necessary
	if radiusWorkloadInstance.RuntimeParameters != nil {
		var jsonData interface{}
		if err := json.Unmarshal(*radiusWorkloadInstance.RuntimeParameters, &jsonData); err != nil {
			return 0, fmt.Errorf("failed to unmarshal runtime parameters json: %w", err)
		}

		prettyJson, err := json.MarshalIndent(jsonData, "", "  ")
		if err != nil {
			return 0, fmt.Errorf("failed to re-marshal runtime parameters json: %w", err)
		}

		if err := ioutil.WriteFile(
			"/tmp/params.json",
			prettyJson,
			0644,
		); err != nil {
			return 0, fmt.Errorf("failed to write params file: %w", err)
		}
	}

	//// set up group
	//groupCmd := exec.Command(
	//	"rad",
	//	"group",
	//	"create",
	//	"test-group",
	//)
	//if err := groupCmd.Run(); err != nil {
	//	return 0, fmt.Errorf("failed to create radius group: %w", err)
	//}

	//// set up environment
	//envCmd := exec.Command(
	//	"rad",
	//	"env",
	//	"create",
	//	"test-env",
	//	"--group",
	//	"test-group",
	//)
	//if err := envCmd.Run(); err != nil {
	//	return 0, fmt.Errorf("failed to create radius environment: %w", err)
	//}

	// deploy radius app
	var deployCmd *exec.Cmd
	if radiusWorkloadInstance.RuntimeParameters != nil {
		deployCmd = exec.Command(
			"rad",
			"deploy",
			"--group",
			//"test-group",
			"default",
			"--environment",
			//"test-env",
			"default",
			"--application",
			*radiusWorkloadInstance.Name,
			"/tmp/app.bicep",
			"-p",
			"@/tmp/params.json",
		)
	} else {
		deployCmd = exec.Command(
			"rad",
			"deploy",
			"--group",
			//"test-group",
			"default",
			"--environment",
			//"test-env",
			"default",
			"--application",
			*radiusWorkloadInstance.Name,
			"/tmp/app.bicep",
		)
	}
	deployOut, err := deployCmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed deploy radius app with output '%s': %w", string(deployOut), err)
	}
	log.V(1).Info(
		"rad deploy command executed",
		"output", string(deployOut),
	)

	return 0, nil
}

// radiusWorkloadInstanceCreated reconciles state for a radius workload
// definition whenever it is changed.
func radiusWorkloadInstanceUpdated(
	r *controller.Reconciler,
	radiusWorkloadInstance *v0.RadiusWorkloadInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// radiusWorkloadInstanceCreated reconciles state for a radius workload
// definition whenever it is removed.
func radiusWorkloadInstanceDeleted(
	r *controller.Reconciler,
	radiusWorkloadInstance *v0.RadiusWorkloadInstance,
	log *logr.Logger,
) (int64, error) {
	deleteCmd := exec.Command(
		"rad",
		"application",
		"delete",
		*radiusWorkloadInstance.Name,
		"--group",
		"default",
		"--yes",
	)
	deleteOut, err := deleteCmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed delete radius app with output '%s': %w", string(deleteOut), err)
	}
	log.V(1).Info(
		"rad application delete command executed",
		"output", string(deleteOut),
	)

	return 0, nil
}
