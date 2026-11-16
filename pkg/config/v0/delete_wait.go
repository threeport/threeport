package v0

import (
	"errors"
	"fmt"

	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// deleteWaitIntervalOverride replaces the poll interval in pollInstanceDeleted
// when set.  It exists so tests of the Delete methods don't wait out the real
// intervals.
var deleteWaitIntervalOverride *int

// pollInstanceDeleted polls until get reports the instance is gone, and returns
// ErrDeleteTimedOut if it never does.  An instance delete only schedules
// deletion, so this is the check that the delete finished.  get must return the
// error from the client's GetXByName call for the instance.  Only a not-found
// counts as gone: an API error says nothing about whether the delete finished,
// so it is retried, not taken as success.
func pollInstanceDeleted(
	kind string,
	name string,
	attempts int,
	intervalSeconds int,
	get func() error,
) error {
	if deleteWaitIntervalOverride != nil {
		intervalSeconds = *deleteWaitIntervalOverride
	}

	err := util.Retry(attempts, intervalSeconds, func() error {
		err := get()
		switch {
		case err == nil:
			return fmt.Errorf("%s %s is still present", kind, name)
		case errors.Is(err, client_lib.ErrObjectNotFound):
			return nil
		default:
			return fmt.Errorf("failed to check whether %s %s was deleted: %w", kind, name, err)
		}
	})
	if err != nil {
		return fmt.Errorf(
			"%w: %s %s was not deleted, so the controller has not finished or has refused the delete; "+
				"check its events for the reason (tptctl get events): %w",
			ErrDeleteTimedOut, kind, name, err,
		)
	}

	return nil
}
