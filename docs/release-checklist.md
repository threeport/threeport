# Threeport Release Checklist

## Minor Version Release

### Pre Release

- [] Ensure all planned changes are merged into minor release branch and pulled
  locally
- [] Pull latest changes for feature branch, build tptctl and container images,
  test manually using Threeport user doc guides.  Ensure guides work as
  documented.  Copy-paste commands from docs to verify correctness.
- [] Make any needed updates to Threeport user-docs and open PR to stage the
  docs changes to be released right after Threeport release is cut.  Include
  version update to install guides as needed.
- [] Open PR to merge feature branch into main.  Merge once all tests pass.

### Release

- [] Cut release as documented in [Release doc](release.md)

### Post Release

Examples below are for when version v0.4.0 was just released.  Adjust
versions as needed for future releases.

- [] Check Releases page to ensure all assets and changelog exist and are
  correct.
- [] Check all container images have been successfully released in Packages.
  Pay particular attention to any new controllers that were added with the
  latest release.
- [] Install latest released tptctl version locally and run manual tests again
  using release container images.  As with pre-release testing, follow guides in
  Threeport user docs to ensure commands work as documented.
- [] Create feature branch for next release.
  ```bash
  git checkout main
  git pull origin main
  git checkout -b 0.5
  git push origin 0.5
  ```
- [] Create new PR branch from feature branch.
  ```bash
  git checkout -b 0.5-version-updates
  ```
- [] Update the base branch check in `.github/workflows/base-branch.yml`.  In this
  example we just need to replace `0.4` with `0.5`.
  ```bash
  git add .
  git commit -s -m "ci: update base branch check for 0.5 feature branch"
  ```
- [] Update go dependencies.
  ```bash
  go get -u ./...
  go mod tidy
  git commit -s -m "chore: update go dependencies"
  ```
- [] Re-build and test to make sure the dependency updates didn't break
  anything obvious.
  ```
  make build-tptdev
  ./bin/tptdev build
  ./bin/tptdev up
  make tests
  ```
- [] Update `DefaultKubernetesVersion` in `nukleros/aws-builder` to latest
  release of Kubernetes.  Cut new release for aws-builder and update aws-builder
  import version for Threeport.
- [] Update kind version used for local control planes.  It is defined in two
  places in `internal/provider/kind.go`.
- [] Update container image version to latest for CockroachDB and NATS in
  installer.
- [] Once all changes are committed, push PR branch
  ```bash
  git push origin 0.5-version-updates
  ```
- [] Open PR for 0.5-version-updates branch to merge into 0.5 base.
- [] Merge dependabot PRs into feature branch.  Close any irrelevant dependabot
  PRs.

