# pkg/config

This package contains:
* Definition and management of the threeport config.
* Threeport object config abstractions for client applications.

The threeport config is a file that resides on a user's workstation at
`~/.config/threeport/config.yaml`.  It provides configuration for a user to
connect to threeport control planes and to switch between different instances of
threeport if need be.

The threeport object config abstractions are used by
`tptctl` to provide abstractions to the raw API fields for users.  It allows
simplified configuration of objects and removes the need to reference objects by
the unique ID used in the database.  Users can reference objects by name and the
client tooling can look up objects by name and then interact with the API using
unique IDs.

This package is versioned to maintain compatibility for projects importing it
while still being able to upgrade to the latest version of threeport. This
means that bug fixes and transparent features can be added across threeport
versions while the functions in a given package version will maintain consistent
function signatures to preserve compatibility.  As such, if function signatures
or fundamental behaviors change, they must be put into a new package version.

