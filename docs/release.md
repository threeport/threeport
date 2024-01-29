# Threeport Releases

Following is the process for releasing a new version of threeport.

Once all PRs have been merged into the feature branch, e.g. 0.5 branch, pull
all remote changes to the feature branch to your local machine.

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

