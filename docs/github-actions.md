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
page](https://github.com/threeport/threeport/settings/secrets/actions).  The
name of the Repository secret is unimportant but the contents must look as
follows:

```
GITHUB_TOKEN=<some long secret>
```

Enter this as a new repo secret and then put the same contents in a safe place
on your filesystem (NOT in any repo) and reference it in the command with the
`-s` flag.  The `act` CLI tool has a flag for referencing an `--env-file` but
this didn't work as expected.  The command above does.

