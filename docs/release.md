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

## Feature Branch for Subsequent Release

Immediately after cutting a release, create a new feature branch.  Check out
main branch and ensure you have all remote changes locally.  The examples below
were used after the v0.4.0 release was cut and a feature branch was created for
the next 0.5 release.

```bash
git checkout -b 0.5
```

Update the base branch check in `.github/workflows/base-branch.yml`.  In this
example we just need to replace `0.4` with `0.5`.

```bash
git add .
git commit -s -m "ci: update base branch check for 0.5 feature branch"
```

This is also a good time to update go dependency versions.

```bash
go get -u ./...
go mod tidy
```

Now, you should re-build and test to make sure the dependency updates didn't break
anything.

```
make build-tptdev
./bin/tptdev build
./bin/tptdev up
make tests
```

Provided everything is still in working order, we can now push the changes.

Commit the changes and push the new 0.5 branch.

```bash
git add .
git commit -s -m "dev: update go dependencies"
git push origin 0.5
```


All new features will now be added to the 0.5 feature branch.

## Bug Fixes

The following applies to bugs that are discovered that affect a released
version.  In this example, it would be a bug that is present in the v0.4.0
release.

For bugs that affect v0.4.0, a PR that *only* addresses the bug should be merged
into the 0.5 feature branch.

Then, that squashed commit from the PR should be cherry picked onto main branch
and a v0.4.1 release cut (using the process above) to provide a fix to any users
using the latest releases.

