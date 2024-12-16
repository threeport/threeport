# Threeport Releases

Following are instructions for cutting releases of Threeport.  See the [Release
Checklist doc](release-checklist.md) for all steps that need to be done for each
release.

## Latest Release

Once all changes have been merged to main branch and pulled locally, the
following command will cut a new release from main branch:

```bash
export RELEASE_VERSION=[version] && make release
```

This will do the following:

* Set the new version in `internal/version/version.txt`.  This file is
  referenced by all released binaries to determine their version, e.g. the
  version number returned by `tptctl version`.
* Set the new version for the swagger docs in `cmd/rest-api/main.go`.
* Add and commit the changes.
* Tag the commit with the release.
* Push the new commit and tag.

The release process will kick off in Github to create a new release and new
packages for the container images.

## Release Candidates

Ensure you have the latest changes from the feature branch locally.

When cutting a release candidate, we follow the same steps, except we cut the
release from the feature branch, not main branch.  The example below shows the
release candidate version format with the rc suffix.

```bash
export RELEASE_VERSION=v0.4.0-rc.2 && make release
```

## Failed Release CI

Should the CI process for a release fail, take the following steps.

Make the changes necessary to remedy the problem and commit them to the main
branch.

Delete the tag that marked the release locally.  The following examples are for
the v0.4.0 release.  Adjust the release tag accordingly.

```bash
git tag -d v0.4.0
```

Delete the remote tag.

```bash
git push --delete origin v0.4.0
```

Re-apply the tag for the current release.

```bash
git tag v0.4.0
```

Push new commit to main branch.

```bash
git push origin main
```

Push the tag to trigger the release process with the CI fixes in place.

```bash
git push origin --tag
```

## Bug Fixes

The following applies to bugs that are discovered that affect a released
version.  In this example, it would be a bug that is present in the v0.4.0
release.

For bugs that affect v0.4.0, a PR that *only* addresses the bug should be merged
into the 0.5 feature branch.

Then, that squashed commit from the PR should be cherry picked onto main branch
and a v0.4.1 release cut (using the process above) to provide a fix to any users
using the latest releases.

