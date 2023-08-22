# Threeport Releases

Following is the process for releasing a new version of threeport.

Once you have all PRs merged that will be included in the next release, check
out main and run the following:

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

