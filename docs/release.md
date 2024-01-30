# Threeport Releases

## Latest Release

Following is the process for releasing a new "latest release" version of Threeport.

Once all PRs have been merged into the feature branch, e.g. 0.5 branch, pull
all remote changes on the feature branch to your local machine.

Next, checkout main branch and merge the feature branch.

Now we can cut the latest release for Threeport.

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

Push the tag to trigger the release process with the CI fixes in place.

```bash
git push origin --tag
```

