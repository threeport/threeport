package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"

	util "github.com/threeport/threeport/pkg/util/v0"
)

type WorkloadTestCase struct {
	// An arbitrary name for the next case
	Name string

	// The object being operatorated on, e.g. workload, workload-instance
	Object string

	// The name of the object - must match the name in the config file
	ObjectName string

	// Path to Threeport config from root of threeport/threeport repo
	ConfigPath string

	// The Kubernetes deployment resource to check that it is healthy - must
	// match the resource name in K8s manifest
	DeploymentName string

	// If true the test case is expected to work, if false expected to fail
	ShouldWork bool

	// Used during tests to identify workload for validation purposes
	InstanceObjectId int64
}

var workloadTestCases = []WorkloadTestCase{
	{
		Name:           "defined instance wordpress workload",
		Object:         "workload",
		ObjectName:     "wordpress",
		ConfigPath:     filepath.Join("test", "e2e", "configs", "wordpress-workload-local.yaml"),
		DeploymentName: "getting-started-wordpress",
		ShouldWork:     true,
	},
	{
		Name:       "duplicate defined instance wordpress workload",
		Object:     "workload",
		ConfigPath: filepath.Join("test", "e2e", "configs", "wordpress-workload-local.yaml"),
		ShouldWork: false,
	},
	{
		Name:       "wordpress workload definition",
		Object:     "workload-definition",
		ObjectName: "wordpress-def",
		ConfigPath: filepath.Join("test", "e2e", "configs", "wordpress-workload-definition-local.yaml"),
		ShouldWork: true,
	},
	{
		Name:           "first wordpress workload instance",
		Object:         "workload-instance",
		ObjectName:     "wordpress-inst-01",
		ConfigPath:     filepath.Join("test", "e2e", "configs", "wordpress-workload-instance-local-01.yaml"),
		DeploymentName: "getting-started-wordpress",
		ShouldWork:     true,
	},
	{
		Name:           "second wordpress workload instance",
		Object:         "workload-instance",
		ObjectName:     "wordpress-inst-02",
		ConfigPath:     filepath.Join("test", "e2e", "configs", "wordpress-workload-instance-local-02.yaml"),
		DeploymentName: "getting-started-wordpress",
		ShouldWork:     true,
	},
}

// Create uses tptctl to create the workload test cases.
func (w *WorkloadTestCase) Create(threeportPath string) error {
	command := tptctlCommand()
	cmdArgs := []string{
		"create",
		w.Object,
		"--config",
		filepath.Join(threeportPath, w.ConfigPath),
		"--threeport-config",
		threeportConfig,
	}
	cmd := exec.Command(command, cmdArgs...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf(
			"failed to create %s with output %s: %w",
			w.Object,
			output,
			err,
		)
	}

	return nil
}

// Describe uses tptctl to describe all the workload instance test cases.
func (w *WorkloadTestCase) Describe(
	threeportPath string,
	testCases *[]WorkloadTestCase,
) error {
	// describing only workload instances - skip workload definitions
	if w.Object == "workload-definition" {
		return nil
	}

	command := tptctlCommand()
	cmdArgs := []string{
		"describe",
		"workload-instance",
		"--name",
		w.ObjectName,
		"--threeport-config",
		threeportConfig,
		"--output",
		"json",
	}
	cmd := exec.Command(command, cmdArgs...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf(
			"failed to describe workload instance %s with output %s: %w",
			w.ObjectName,
			output,
			err,
		)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.UseNumber()
	var workloadInstanceMap map[string]interface{}
	if err := decoder.Decode(&workloadInstanceMap); err != nil {
		return fmt.Errorf(
			"failed to unmarshal workload instance %s: %w",
			w.ObjectName,
			err,
		)
	}

	var objectId int64
	if idNum, ok := workloadInstanceMap["ID"].(json.Number); ok {
		idInt64, err := idNum.Int64()
		if err != nil {
			return fmt.Errorf(
				"failed to convert ID for workload instance %s to int64: %w",
				w.ObjectName,
				err,
			)
		}
		objectId = idInt64
	} else {
		return fmt.Errorf(
			"failed to find object ID in describe output for workload instance %s",
			w.ObjectName,
		)
	}

	// update test case so the object ID is available for validation
	for i, testCase := range *testCases {
		if testCase.Name == w.Name {
			(*testCases)[i].InstanceObjectId = objectId
			break
		}
	}

	return nil
}

// Validate checks the primary deployment Kubernetes resource to validate it is
// healthy.
func (w *WorkloadTestCase) Validate() error {
	// describing only workload instances - skip workload definitions
	if w.Object == "workload-definition" {
		return nil
	}

	// retry 48 times at 10 sec intervals - 8 min
	if err := util.Retry(
		48,
		10,
		func() error {
			namespaceName, err := getNamespaceByWorkloadInstanceId(w.InstanceObjectId)
			if err != nil {
				return fmt.Errorf(
					"failed to get namespace for workload instance with ID %d: %w",
					w.InstanceObjectId,
					err,
				)
			}

			deployment, err := getDeploymentByName(w.DeploymentName, namespaceName)
			if err != nil {
				return fmt.Errorf("failed to get deployment: %w", err)
			}

			if deployment.Status.ReadyReplicas < 1 {
				return fmt.Errorf("deployment %s has zero ready replicas", deployment.Name)
			}

			return nil
		},
	); err != nil {
		return fmt.Errorf("failed to validate deployment status as ready: %w", err)
	}

	return nil
}

// Delete uses tptctl to delete the defined instance and instance test cases.
func (w *WorkloadTestCase) DeleteInstances() error {
	command := tptctlCommand()
	cmdArgs := []string{
		"delete",
		w.Object,
		"--config",
		filepath.Join(threeportPath, w.ConfigPath),
		"--threeport-config",
		threeportConfig,
	}
	cmd := exec.Command(command, cmdArgs...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf(
			"failed to delete %s with output %s: %w",
			w.Object,
			output,
			err,
		)
	}

	return nil
}

// Delete uses tptctl to delete the workload definitions.
func (w *WorkloadTestCase) DeleteDefinitions() error {
	command := tptctlCommand()
	cmdArgs := []string{
		"delete",
		w.Object,
		"--config",
		filepath.Join(threeportPath, w.ConfigPath),
		"--threeport-config",
		threeportConfig,
	}
	cmd := exec.Command(command, cmdArgs...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf(
			"failed to delete %s with output %s: %w",
			w.Object,
			output,
			err,
		)
	}

	return nil
}

// Worked compares the test method function output with the test case's
// ShouldWork field to see if the intended result occurred.
func (w *WorkloadTestCase) Worked(err error) bool {
	switch w.ShouldWork {
	case true && err == nil:
		return true
	case false && err != nil:
		return true
	}

	return false
}

func tptctlCommand() string {
	return filepath.Join(threeportPath, "bin", "tptctl")
}
