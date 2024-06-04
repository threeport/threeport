# pkg/controller

This package is a library for common controller operations.  It is used by core
Threeport controllers and can be used by externally maintained custom controllers
that are added to user's Threeport implementations.

This package is versioned to maintain compatibility for projects importing it
while still being able to upgrade to the latest version of Threeport. This
means that bug fixes and transparent features can be added across Threeport
versions while the functions in a given package version will maintain consistent
function signatures to preserve compatibility.  As such, if function signatures
or fundamental behaviors change, they must be put into a new package version.

See the [Versions documentation](../../docs/versions.md) for more information
on the versioning of objects and library packages.

