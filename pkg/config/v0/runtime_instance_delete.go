package v0

import (
	"errors"
	"fmt"

	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ErrDeleteTimedOut is returned when the API accepted a delete for a runtime
// instance but the instance was still there when the wait ended. The controller
// is then still working on the delete, or has refused it.
var ErrDeleteTimedOut = errors.New("timed out waiting for the delete to complete")

// instanceDeleteWait is how long to wait for a runtime instance to go away
// after its delete is accepted: attempts, and seconds between them. It is a
// variable so a test does not wait out the real thing.
var instanceDeleteWait = struct{ attempts, intervalSeconds int }{attempts: 90, intervalSeconds: 10}

// waitForInstanceDelete polls until get reports the instance is gone, and
// returns ErrDeleteTimedOut if it never does. Only a not-found counts as gone:
// an API error says nothing about whether the delete finished, so it is not
// taken as success.
//
// A runtime instance is deleted by its controller after the API accepts the
// request, and the controller can refuse, so the API accepting the delete is
// not the delete being done.
func waitForInstanceDelete(kind, name string, get func() error) error {
	err := util.Retry(instanceDeleteWait.attempts, instanceDeleteWait.intervalSeconds, func() error {
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
