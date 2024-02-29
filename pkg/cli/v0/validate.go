package v0

import (
	"fmt"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// ValidateConfigNameFlags takes the following flag values
// * config file path value as provided by user with a flag
// * object name value as provided by user with a flag
// It also takes the name of the object as it should be displayed in an output
// error message.
// It validates that either the path to the config file or the object name (but
// not both) is provided.
func ValidateConfigNameFlags(
	objectConfigPath string,
	objectName string,
	objectOutputName string,
) error {
	if objectConfigPath == "" && objectName == "" {
		return fmt.Errorf("must provide either %s name or path to config file", objectOutputName)
	}

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
