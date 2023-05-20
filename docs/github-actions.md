# Github Actions

We use github actions for continuous integration: automated tests, build and
release processes.

The `.github` directory contains the configuration for these CI processes.

When making changes to these configs you can test the actions locally with the
[act](https://github.com/nektos/act) tool before pushing.

The following examples give the commands to run actions locally.

The release action requires a github secret in order to work.  See "Github
Secrets" section below for more info.

```bash
act -W .github/workflows/check.yaml  # checks commit message formats
act -W .github/workflows/release.yml -s $(cat ~/.dev/threeport.env)  # runs release actions
act -W .github/workflows/test.yml  # runs automated tests
```

## Github Secrets

When a secret is needed to run an action, a new secret must first be created in
the [github actions secrets
page](https://github.com/threeport/threeport/settings/secrets/actions) to run
from your local machine. The name of the Repository secret is unimportant but
the contents must look as follows:

```
GITHUB_TOKEN=<some long secret>
```

Enter this as a new repo secret and then put the same contents in a safe place
on your filesystem (NOT in any repo) and reference it in the command with the
`-s` flag.  The `act` CLI tool has a flag for referencing an `--env-file` but
this didn't work as expected.  The command above does.

## Release Cloning

Until this repo is publicly released, we need to clone releases from this
private repo to a public repo in order to make the releases public (while
keeping the source code private).

This clone is performed on each release.  The "Release" workflow in
`.github/workflows/release.yml` has a `clone` job that performs the
cloning.  The token to gain cross-repo permissions is derived from the [Release
Clone](https://github.com/organizations/threeport/settings/apps/release-clone)
github app that is installed in the Threeport org.  The app does nothing - it
is used only for cross-repo permissions which cannot be achieved with a regular
`GITHUB_TOKEN`.  The App ID and private key for that app is used to create the
`APPLICATION_ID` and `APPLICATION_PRIVATE_KEY` [repository
secrets](https://github.com/threeport/threeport/settings/secrets/actions) for
this repo.  Those secrets are used by the `Get token` step in the "Release"
workflow to generate a token that is used by the `Clone release` step.

Once this repo is publicly released, we can remove the `clone` step in the
"Release" workflow, the `APPLICATION_ID` and `APPLICATION_PRIVATE_KEY` repo
secrets and the Release Clone github app.

