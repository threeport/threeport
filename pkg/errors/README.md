# pkg/errors

This package is used for error management for events.  It is used by controllers
when recording events in the Threeport API.

This package is versioned to maintain compatibility for projects importing it
while still being able to upgrade to the latest version of threeport. This
means that bug fixes and transparent features can be added across threeport
versions while the functions in a given package version will maintain consistent
function signatures to preserve compatibility.  As such, if function signatures
or fundamental behaviors change, they must be put into a new package version.

See the [Versions documentation](../../docs/versions.md) for more information
on the versioning of objects and library packages.

