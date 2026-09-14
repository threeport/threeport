package v0

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// ValidateConfigNameFlags takes the following flag values
// * config file path value as provided by user with a flag
// * object name value as provided by user with a flag
// It also takes the name of the object as it should be displayed in an output
// error message.
// It validates that the object name and path to the config file are not both provided.
func ValidateConfigNameFlags(
	objectConfigPath string,
	objectName string,
	objectOutputName string,
) error {
	if objectConfigPath != "" && objectName != "" {
		return fmt.Errorf("%s name and path to config file provided - provide only one", objectOutputName)
	}

	return nil
}

// ValidateDescribeOutputFlag validates output formats for describe commands.
func ValidateDescribeOutputFlag(
	outputFormat string,
	objectOutputName string,
) error {
	validOutputFormats := []string{
		"plain",
		"json",
		"yaml",
	}

	if !util.StringSliceContains(validOutputFormats, outputFormat, false) {
		return fmt.Errorf("invalid output format - valid formats: %s", validOutputFormats)
	}

	return nil
}

// ControlPlaneConfigProblems returns an error when the Threeport config is
// missing or has no API endpoint for the current control plane.
func ControlPlaneConfigProblems() error {
	// read the Threeport config tptctl wrote to disk
	cfgFile := DetermineThreeportConfigPath("")
	data, err := os.ReadFile(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to read the Threeport config: %w", err)
	}

	// parse the Threeport config
	var threeportConfig ThreeportConfig
	if err := yaml.Unmarshal(data, &threeportConfig); err != nil {
		return fmt.Errorf("failed to parse the Threeport config: %w", err)
	}

	// require a current control plane
	controlPlaneName := threeportConfig.CurrentControlPlane
	if controlPlaneName == "" {
		return errors.New("current control plane must be set in the Threeport config")
	}

	// require an API endpoint for the current control plane
	if _, err := threeportConfig.GetThreeportAPIEndpoint(controlPlaneName); err != nil {
		return fmt.Errorf(
			"failed to get the API endpoint for control plane %s: %w",
			controlPlaneName, err,
		)
	}

	return nil
}

// UnmetPrerequisites joins independent prerequisite errors with errors.Join
// and names the target. It returns nil when every argument is nil.
func UnmetPrerequisites(target string, errs ...error) error {
	joined := errors.Join(errs...)
	if joined == nil {
		return nil
	}

	return fmt.Errorf("%s prerequisites are not met:\n%w", target, joined)
}
