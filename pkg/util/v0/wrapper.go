package v0

import (
	"fmt"
	"time"
)

// Retry will retry a function until it succeeds or the maximum number of attempts
// is reached.
func Retry(
	attemptsMax int,
	waitDurationSeconds int,
	f func() error,
) error {
	attempts := 0
	var err error
	for attempts < attemptsMax {
		err = f()
		if err == nil {
			return nil
		}
		fmt.Println(err)
		attempts++
		time.Sleep(time.Second * time.Duration(waitDurationSeconds))
	}
	return fmt.Errorf("failed after %d attempts: %w", attemptsMax, err)
}
