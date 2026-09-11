package v0

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ControlPlaneConfigProblems returns errors when the Threeport config is
// missing or has no API endpoint for the current control plane.
func ControlPlaneConfigProblems() []error {
	// read the Threeport config tptctl wrote to disk
	cfgFile := DetermineThreeportConfigPath("")
	data, err := os.ReadFile(cfgFile)
	if err != nil {
		return []error{fmt.Errorf("failed to read the Threeport config: %w", err)}
	}

	// parse the Threeport config
	var threeportConfig ThreeportConfig
	if err := yaml.Unmarshal(data, &threeportConfig); err != nil {
		return []error{fmt.Errorf("failed to parse the Threeport config: %w", err)}
	}

	// require a current control plane
	controlPlaneName := threeportConfig.CurrentControlPlane
	if controlPlaneName == "" {
		return []error{errors.New("current control plane must be set in the Threeport config")}
	}

	// require an API endpoint for the current control plane
	if _, err := threeportConfig.GetThreeportAPIEndpoint(controlPlaneName); err != nil {
		return []error{fmt.Errorf(
			"failed to get the API endpoint for control plane %s: %w",
			controlPlaneName, err,
		)}
	}

	return nil
}

// UnmetPrerequisites joins prerequisite errors into one error named for the
// target. It returns nil when there are none.
func UnmetPrerequisites(target string, problems []error) error {
	if len(problems) == 0 {
		return nil
	}

	return fmt.Errorf("%s prerequisites are not met:\n%w", target, errors.Join(problems...))
}
