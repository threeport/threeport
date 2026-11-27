# internal/version

This package is used by all components and tools to synchronize the threeport
version.  It allows each component of a threeport release to use the same version
so that compatibility can be ensured.  Do not edit the `version.txt` file.  It
is updated during the release process.

A Head build, an untagged build of a commit on a release branch, also sets
`Commit` at build time with the `HEAD_COMMIT` environment variable, and
`GetVersion()` then returns `<version>.<commit>`.  See
[Head Builds](../../docs/dev/head-builds.md).

