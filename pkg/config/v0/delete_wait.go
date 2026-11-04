package v0

import (
	"errors"
	"fmt"

	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// deleteWaitIntervalOverride replaces the poll interval in
// waitForInstanceDeleted when set.  It exists so tests of the Delete methods
// don't wait out the real intervals.
var deleteWaitIntervalOverride *int

// waitForInstanceDeleted polls until the API reports that an instance is gone.
// An instance delete only schedules deletion, so this is the check that the
// delete finished.  get must return the error from the client's GetXByName call
// for the instance.  Only client_lib.ErrObjectNotFound means the instance is
// deleted - any other error is retried, as it says nothing about the instance.
// If the instance is still there after all attempts an error is returned.
func waitForInstanceDeleted(
	kind string,
	name string,
	attempts int,
	intervalSeconds int,
	get func() error,
) error {
	waitSeconds := attempts * intervalSeconds
	if deleteWaitIntervalOverride != nil {
		intervalSeconds = *deleteWaitIntervalOverride
	}

	if err := util.Retry(attempts, intervalSeconds, func() error {
		err := get()
		switch {
		case err == nil:
			return fmt.Errorf("%s instance %s is still present", kind, name)
		case errors.Is(err, client_lib.ErrObjectNotFound):
			return nil
		default:
			return fmt.Errorf("failed to check whether %s instance %s is deleted: %w", kind, name, err)
		}
	}); err != nil {
		return fmt.Errorf(
			"timed out waiting for %s instance %s to be deleted: delete scheduled, but not confirmed after %ds - the controller may still be working: %w",
			kind, name, waitSeconds, err,
		)
	}

	return nil
}
