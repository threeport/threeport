package e2e_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_v0 "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	threeportConfig = "/tmp/e2e-threeport-config.yaml"

	// threeportName is the control plane the suite installs.  It lives here
	// rather than in a _test.go file because this file is compiled on its own
	// by `go build`, which does not see test files.
	threeportName = "e2e-test"
)

type WorkloadTestCase struct {
	// An arbitrary name for the next case
	Name string

	// The object being operatorated on, e.g. workload, kubernetes-workload-instance
	Object string

	// The name of the object - must match the name in the config file
	ObjectName string

	// Path to workload config from root of threeport/threeport repo
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
		Object:         "kubernetes-workload",
		ObjectName:     "wordpress",
		ConfigPath:     filepath.Join("test", "e2e", "configs", "wordpress-kubernetes-workload-local.yaml"),
		DeploymentName: "getting-started-wordpress",
		ShouldWork:     true,
	},
	{
		Name:       "duplicate defined instance wordpress workload",
		Object:     "kubernetes-workload",
		ConfigPath: filepath.Join("test", "e2e", "configs", "wordpress-kubernetes-workload-local.yaml"),
		ShouldWork: false,
	},
	{
		Name:       "wordpress kubernetes workload definition",
		Object:     "kubernetes-workload-definition",
		ObjectName: "wordpress-def",
		ConfigPath: filepath.Join("test", "e2e", "configs", "wordpress-kubernetes-workload-definition-local.yaml"),
		ShouldWork: true,
	},
	{
		Name:           "first wordpress kubernetes workload instance",
		Object:         "kubernetes-workload-instance",
		ObjectName:     "wordpress-inst-01",
		ConfigPath:     filepath.Join("test", "e2e", "configs", "wordpress-kubernetes-workload-instance-local-01.yaml"),
		DeploymentName: "getting-started-wordpress",
		ShouldWork:     true,
	},
	{
		Name:           "second wordpress kubernetes workload instance",
		Object:         "kubernetes-workload-instance",
		ObjectName:     "wordpress-inst-02",
		ConfigPath:     filepath.Join("test", "e2e", "configs", "wordpress-kubernetes-workload-instance-local-02.yaml"),
		DeploymentName: "getting-started-wordpress",
		ShouldWork:     true,
	},
}

// Create uses tptctl to create the workload test cases.
func (w *WorkloadTestCase) Create(threeportPath string) error {
	command := tptctlCommand(threeportPath)
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

// Describe reads back a kubernetes workload instance to pick up the ID the
// API assigned it, which later steps use to find the namespace its resources
// were deployed into.
//
// It uses `tptctl get` because the describe command this once used was
// removed in #371 and the suite was never updated, so every run failed here
// with "unknown flag: --name".
func (w *WorkloadTestCase) Describe(
	threeportPath string,
	testCases *[]WorkloadTestCase,
) error {
	// describing only workload instances - skip workload definitions
	if w.Object == "kubernetes-workload-definition" {
		return nil
	}

	// Ask the API for the instance rather than parsing CLI output: `tptctl
	// get` prints the config view of an object, which carries no ID, and the
	// describe command that used to print one was removed in #371.
	instance, err := getKubernetesWorkloadInstanceByName(w.ObjectName)
	if err != nil {
		return fmt.Errorf(
			"failed to get kubernetes workload instance %s: %w",
			w.ObjectName,
			err,
		)
	}

	objectId := int64(*instance.ID)

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
	if w.Object == "kubernetes-workload-definition" {
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
					"failed to get namespace for kubernetes workload instance with ID %d: %w",
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
func (w *WorkloadTestCase) DeleteInstances(threeportPath string) error {
	command := tptctlCommand(threeportPath)
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
func (w *WorkloadTestCase) DeleteDefinitions(threeportPath string) error {
	command := tptctlCommand(threeportPath)
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

func tptctlCommand(threeportPath string) string {
	return filepath.Join(threeportPath, "bin", "tptctl")
}

// decodeSingleObject reads one object out of tptctl's JSON output, which
// returns either the object itself or a list holding it depending on the
// command.
func decodeSingleObject(output []byte) (map[string]interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.UseNumber()

	var decoded interface{}
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("failed to decode JSON output: %w", err)
	}

	switch typed := decoded.(type) {
	case map[string]interface{}:
		return typed, nil
	case []interface{}:
		if len(typed) == 0 {
			return nil, errors.New("JSON output holds no objects")
		}
		object, ok := typed[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("first element of JSON output is %T, not an object", typed[0])
		}
		return object, nil
	}

	return nil, fmt.Errorf("JSON output is %T, not an object or a list", decoded)
}

// getKubernetesWorkloadInstanceByName looks a workload instance up in the
// Threeport API, using the config the suite told tptctl to write.
func getKubernetesWorkloadInstanceByName(name string) (*api_v0.KubernetesWorkloadInstance, error) {
	configBytes, err := os.ReadFile(threeportConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to read the test threeport config: %w", err)
	}

	var suiteConfig cli.ThreeportConfig
	if err := yaml.Unmarshal(configBytes, &suiteConfig); err != nil {
		return nil, fmt.Errorf("failed to parse the test threeport config: %w", err)
	}

	apiClient, err := suiteConfig.GetHTTPClient(threeportName)
	if err != nil {
		return nil, fmt.Errorf("failed to build a threeport API client: %w", err)
	}
	apiEndpoint, err := suiteConfig.GetThreeportAPIEndpoint(threeportName)
	if err != nil {
		return nil, fmt.Errorf("failed to get the threeport API endpoint: %w", err)
	}

	instance, err := client_v0.GetKubernetesWorkloadInstanceByName(apiClient, apiEndpoint, name)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve the instance from the threeport API: %w", err)
	}
	if instance.ID == nil {
		return nil, fmt.Errorf("the threeport API returned instance %s without an ID", name)
	}

	return instance, nil
}
