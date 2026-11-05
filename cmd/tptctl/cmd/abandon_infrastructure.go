package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	cobra "github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config_v0 "github.com/threeport/threeport/pkg/config/v0"
)

var (
	// abandonInfrastructure is set by --abandon-infrastructure on the delete
	// commands for infrastructure-backed runtime instances.
	abandonInfrastructure bool

	// abandonSkipConfirmation is set by --yes to skip the confirmation prompt
	// that guards --abandon-infrastructure.
	abandonSkipConfirmation bool
)

// addAbandonInfrastructureFlags registers --abandon-infrastructure and --yes on
// a delete command for a runtime instance backed by cloud infrastructure.
func addAbandonInfrastructureFlags(deleteCmd *cobra.Command) {
	deleteCmd.Flags().BoolVar(
		&abandonInfrastructure,
		"abandon-infrastructure", false,
		"Allow the delete to proceed even if the controller has no record of the cloud resources it created, "+
			"or cannot read the record it has. "+
			"The controller refuses that delete by default because the resources may still be running. "+
			"Resources the controller has recorded and can read are still destroyed as usual; "+
			"all others are left running and no longer tracked by Threeport.",
	)
	deleteCmd.Flags().BoolVarP(
		&abandonSkipConfirmation,
		"yes", "y", false,
		"Skip the confirmation prompt for --abandon-infrastructure.",
	)
}

// confirmAbandonInfrastructure asks the user to confirm abandoning the cloud
// infrastructure of the named object, and exits unless they do.
func confirmAbandonInfrastructure(objectKind, name string) {
	if abandonSkipConfirmation {
		return
	}

	fmt.Printf(
		"This will delete %s '%s' even if the controller has no record of some of its cloud resources,\n"+
			"or cannot read the record it has.\n"+
			"Resources the controller has recorded and can read will still be destroyed.\n"+
			"All others will be left in place: if they are still running they will\n"+
			"keep costing money and will no longer be tracked by Threeport.\n"+
			"Check the cloud account first and remove them by hand if they exist.\n",
		objectKind, name,
	)
	fmt.Print("Abandon the infrastructure? (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(response)) != "y" {
		// exit non-zero so a delete that did not happen is not mistaken for one
		// that did
		cli.Info("Aborted.")
		os.Exit(1)
	}
}

// withdrawAbandonInfrastructure switches the override back off after a delete
// that failed. It stays on only when the delete request got through and the
// controller is still working on it or waiting on it, where the controller may
// still need it. Failing to switch it off is reported, because it leaves later
// plain deletes of the instance without the controller's safeguards.
func withdrawAbandonInfrastructure(deleteErr error, objectKind, name string, withdraw func() error) {
	if errors.Is(deleteErr, config_v0.ErrDeleteTimedOut) {
		return
	}

	if err := withdraw(); err != nil {
		cli.Warning(fmt.Sprintf(
			"the delete failed and infrastructure abandonment could not be switched back off for %s %s: %v. "+
				"Until it is, a later delete of this instance skips the safeguards against orphaning cloud resources.",
			objectKind, name, err,
		))
	}
}
