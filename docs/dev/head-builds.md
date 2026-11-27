# Head Builds

A Head build is an untagged build of a commit on a release branch, such as
`0.7`.  Use one to run a control plane from a commit that has been merged but
not yet released, with images that its clusters can pull from a registry and a
`tptctl` built from the same commit.

| Kind | What it is | Example | Built by |
| --- | --- | --- | --- |
| **OSR** | Official stable release, cut from `main` | `v0.7.0` | `release.yml` on tag push |
| **RC** | Release candidate, cut from the release branch | `v0.7.0-rc.6` | `release.yml` on tag push |
| **Head** | An untagged build of a commit on the release branch | `v0.7.0-rc.6.a1b2c3d` | `head-images.yml`, on demand |

Head images are not published on every merge: that would push a full set of
control plane images for each merge, nearly all of which would never be used.
They are built when an engineer asks for them with one mage target.

## Head version

```
<contents of internal/version/version.txt>.<first 7 characters of the full commit SHA>
```

For example, commit `a1b2c3d4e5f6...` on `0.7`, where `version.txt` is
`v0.7.0-rc.6`, is `v0.7.0-rc.6.a1b2c3d`.

For a Head build that one string is all of the following:

* the output of `tptctl version`;
* the `Version` in the REST API's `/version` response in the Head images;
* the image tag in GHCR;
* the `org.opencontainers.image.version` label of the images;
* the `app.kubernetes.io/version` labels the installer writes.

A `tptctl` built from a Head commit uses its own version as the default control
plane image tag, so its default image tag is the matching Head images.

The short SHA is the first 7 characters of the 40-character SHA, taken by string
truncation.  It is not `git rev-parse --short=7`, which returns more than 7
characters when a prefix is ambiguous and could give the same commit different
tags at different times.  `tptdev` dev image tags use the same rule.

RC and OSR versions do not change.  Their builds never set a commit, so they
are built, and report their version, exactly as before.

### How the version is stamped

`internal/version` has a `Commit` variable that is empty for RC and OSR builds.
`version.GetVersion()` returns `version.txt`, and `version.txt` followed by
`.<Commit>` when `Commit` is set.  The `HEAD_COMMIT` environment variable sets
it: when `HEAD_COMMIT` is a 7-character short SHA, `mage build:tptctl` and every
image build link it in with

```
-ldflags=-X=<module path>/internal/version.Commit=$HEAD_COMMIT
```

A `HEAD_COMMIT` that is not exactly 7 lowercase hex characters is an error and
never an unstamped build.  The variable is set for you by the mage targets below.
If you export it in your shell, `mage build:allImagesDev` stamps your local
builds too, so unset it for local development.

## Prerequisites

* `gh`, logged in (`gh auth status`), with write access to
  `threeport/threeport`.  Dispatching a workflow requires write access.
* `docker` with buildx, used to read image labels from the registry.
* `git`.
* A clean checkout.  Unlike `tptdev`, which ignores `go.mod` and `go.sum`, a Head
  build allows no uncommitted or untracked files: the `tptctl` you build must be
  built from exactly the commit CI builds.

Both targets compare against upstream, never `origin`.  They read
`https://github.com/threeport/threeport` directly, so they work from a fork
checkout whatever you named the upstream remote, or if it has none.

## Build a Head: `mage release:head`

`mage release:head` builds the Head images of the current commit through CI and
then installs a matching `tptctl`.  Run it from a clean checkout of a release
branch that is at upstream's head:

```bash
git checkout 0.7
git pull upstream 0.7
mage release:head
```

It does the following:

1. Checks that `git`, `gh` and `docker` (with buildx) are on `PATH` and that
   `gh auth status` succeeds.
2. Checks that the branch is a release branch (a name like `0.7`).
3. Checks that the working tree is clean.
4. Checks that local `HEAD` is the head of that branch upstream.
5. Computes the Head version.
6. If every Head image already exists, says so and goes straight to step 9.
7. Dispatches `head-images.yml` for the commit.
8. Finds the run, prints its URL and watches it.  If the run fails, it exits
   non-zero, prints the run URL and does not install `tptctl`.
9. Verifies the images: for every image `package:allManifests` covers, that
   `ghcr.io/threeport/<image>:<Head version>` exists for amd64 and arm64 and
   that its `org.opencontainers.image.revision` label is the full SHA of this
   commit.
10. Builds and installs `tptctl` with `HEAD_COMMIT` set, in `$GOBIN` (or
    `$GOPATH/bin`), and checks that it reports the Head version.  This replaces
    any `tptctl` already there.
11. Prints a summary.

Example output, with the image check and the `gh run watch` output shortened:

```
Head version: v0.7.0-rc.6.a1b2c3d (commit a1b2c3d4e5f60718293a4b5c6d7e8f9012345678 on 0.7)
Head images for v0.7.0-rc.6.a1b2c3d are not all published yet (could not inspect ghcr.io/threeport/threeport-rest-api:v0.7.0-rc.6.a1b2c3d: ...)
dispatching head-images.yml on 0.7 for a1b2c3d
workflow run: https://github.com/threeport/threeport/actions/runs/123456789
...
verified 16 Head images for v0.7.0-rc.6.a1b2c3d
tptctl binary built and available at bin/tptctl
tptctl binary installed and available at /home/you/go/bin/tptctl

Head build ready
  version:      v0.7.0-rc.6.a1b2c3d
  workflow run: https://github.com/threeport/threeport/actions/runs/123456789
  tptctl:       /home/you/go/bin/tptctl
  tptctl version prints: v0.7.0-rc.6.a1b2c3d

tptctl up and tptctl upgrade control-plane ... now default to these Head images:
--control-plane-image-tag is not needed.
```

Running it again for the same commit does not dispatch a second run: it reports
that the images exist and installs `tptctl`.

## Install `tptctl` for an existing Head: `mage install:tptctlHead`

`mage install:tptctlHead` installs a `tptctl` for a Head whose images already
exist.  Use it instead of `release:head` when you do not need to build
anything, for example when:

* someone else already ran `mage release:head` for the commit;
* a control plane you operate runs a Head build and you want a `tptctl` that
  matches it.

Check out the commit first.  A detached `HEAD` is fine, and the branch name does
not matter:

```bash
git checkout a1b2c3d4e5f60718293a4b5c6d7e8f9012345678
mage install:tptctlHead
```

It needs only `git` and `docker`, not `gh`.  It checks that the tree is clean,
computes the Head version of the checked out commit and verifies that the images
exist and were built from this exact commit.  Then it installs `tptctl` and
prints its version.  Because it only reads the registry, it never builds
images.  They can only exist if `head-images.yml` built that commit, and
`head-images.yml` only accepts commits on a release branch.

## Install or upgrade a control plane with a Head `tptctl`

With the `tptctl` that either target installs, no `--control-plane-image-tag`
is needed.  Its default is the Head version:

```bash
tptctl up --name <name> ...
tptctl upgrade control-plane ...
```

To confirm what a control plane is running:

```bash
# the REST API's version
curl <api endpoint>/version
# {"Version":"v0.7.0-rc.6.a1b2c3d"}

# the image of each control plane pod
kubectl get pods -n threeport-control-plane \
  -o jsonpath='{range .items[*]}{.spec.containers[*].image}{"\n"}{end}'
```

Passing `--control-plane-image-tag` still overrides the default.

## The workflow

`.github/workflows/head-images.yml` is a `workflow_dispatch` workflow.  It
checks that it was dispatched from a release branch, that the input is a full
40-character SHA and that the commit is on that branch, and computes the Head
version from `version.txt` as of that commit, all before building anything.
Then it calls `_build-images.yml` to build and push the images.  See
[Github Actions](github-actions.md).

Any commit on the branch is accepted, not only its head, so an older Head can be
rebuilt.  `mage release:head` always sends the current head.

To dispatch it by hand:

```bash
gh workflow run head-images.yml --repo threeport/threeport --ref 0.7 \
  -f commit=a1b2c3d4e5f60718293a4b5c6d7e8f9012345678
```

## Where the images are

Head images go to the same GHCR packages as RC and OSR images, which are public:
`ghcr.io/threeport/<image>:<Head version>`.  Clusters can pull them with no pull
secret.  As with releases, each run also leaves per-arch `<tag>-amd64` and
`<tag>-arm64` tags.  Nothing cleans up Head images automatically.

## Troubleshooting

**The working tree is not clean.**  Commit or remove the files it lists,
including untracked ones.  `go.mod` and `go.sum` are not exempt.

**Not a release branch.**  Check out the release branch, for example `0.7`.
`mage install:tptctlHead` has no branch requirement.

**Local `HEAD` is not at the head of the branch upstream.**  If your checkout is
behind, pull from upstream and run it again.  If it is ahead or has diverged,
Head builds only come from commits merged upstream.

**The workflow run failed.**  The target exits non-zero without installing
`tptctl` and prints the run URL.  Fix the cause and run `mage release:head`
again.

**No run appeared within 60 seconds.**  GitHub is slow to list a new run.  Look
at the workflow's runs under Actions, and run `mage release:head` again once
the run has finished: it finds the images and does not dispatch again.

**Images are missing or the revision label does not match.**
`mage install:tptctlHead` lists each image that is missing, lacks an amd64 or
arm64 image or was built from another commit.  Either run `mage release:head`
from the release branch to build the commit, or check out the commit that the
control plane actually runs.

**`could not inspect` an image that exists.**  An expired `ghcr.io` login in your
Docker config can make GHCR refuse a public image.  Run `docker logout ghcr.io`
and try again.

**`HEAD_COMMIT` is invalid.**  It must be exactly 7 lowercase hex characters.
Unset it, or let the mage targets set it.
