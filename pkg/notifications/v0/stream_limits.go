package v0

import "fmt"

const (
	// ControllerStreamCount is the number of controller notification streams
	// the REST API creates.  It is one per controller, so it must grow when a
	// controller is added.
	ControllerStreamCount = 13

	// ControllerStreamMaxBytes caps the disk one controller notification
	// stream may use.  Notifications are about 700 bytes each, so this holds
	// several hundred thousand unprocessed notifications.  A stream at its cap
	// rejects new notifications and does not discard old ones.
	ControllerStreamMaxBytes = 256 * 1024 * 1024

	// MessageBrokerMaxFileGiB is the disk ceiling for JetStream file storage in
	// GiB.  It is set below the message broker volume so the ceiling does not
	// depend on the free space when the server starts.
	MessageBrokerMaxFileGiB = 4

	// MessageBrokerMinVolumeGiB is the smallest message broker volume in GiB
	// that holds MessageBrokerMaxFileGiB of JetStream storage with room left
	// for the file system and the server's own files.  A smaller volume fills
	// before the ceiling is reached.
	MessageBrokerMinVolumeGiB = 5
)

// MessageBrokerMaxFile returns the max_file value for the message broker
// configuration, e.g. "4Gi".
func MessageBrokerMaxFile() string {
	return fmt.Sprintf("%dGi", MessageBrokerMaxFileGiB)
}
