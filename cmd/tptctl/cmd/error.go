package cmd

import (
	"fmt"

	cli "github.com/threeport/threeport/pkg/cli/v0"
)

func missingErr(command string) {
	cli.Error(
		"",
		fmt.Errorf("missing subcommand - use tptctl %s -h for usage info", command),
	)
}

func unknownErr(command string, subcommand string) {
	cli.Error(
		"",
		fmt.Errorf("unkown subcomand %s - use tptctl %s -h for usage info", subcommand, command),
	)
}
