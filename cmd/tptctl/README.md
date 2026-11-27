# tptctl

Manage workloads in Threeport from the command line.

Here you will find the main package for `tptctl` along with the commands
available in that tool.  It is the primary client tool for the Threeport control
plane.

It makes use of the client library in `pkg/client` for interactions with the API
and uses the config packages in `pkg/config` to provide the config abstractions
for users that relieve the user from managing much of the detail needed to
create, update and delete API objects.

It can be used to [install](../../docs/installation.md) a control plane.

It references a user's Threeport config to connect to a Threeport control plane.
More information on the Threeport config can be found [here](../../docs/threeport-config.md)

## tptctl version

`tptctl version` reports the version `tptctl` was built with and the version of
the control plane it is configured to use, the way `kubectl version` does:

```text
Client Version: v0.7.0-rc.6.a1b2c3d
Server Version: v0.7.0-rc.6.a1b2c3d (control plane genesis-00)
```

The server version is what the control plane's REST API reports on `/version`.
`tptctl` uses its own version as the default image tag for `tptctl up`, `tptctl
upgrade control-plane image`, `tptctl upgrade control-plane blue-green` and
control plane creation, so a `tptctl` that does not match the control plane it
manages quietly moves that control plane to different images.  When the two
versions differ, both lines are still printed and the command exits 0, and a
warning is written to stderr.  The versions are compared as exact strings, not
ordered.

| Flag | Behavior |
| --- | --- |
| `--client` | Print only the client version.  Does not read the config or make a network call. |
| `--control-plane-name`, `-i` | Report the server version of the named control plane instead of the current one. |
| `--output`, `-o` | `text` (default), `json` or `yaml`.  Stdout holds only the versions. |

With no Threeport config, or no current control plane, the server version is
`none` and the exit code is 0.  If a control plane is configured but its version
can't be read, `tptctl version` prints the client version, writes the error to
stderr and exits 1 after 10 seconds at most; use `--client` offline.

To fix a mismatch, install the released `tptctl` for an RC or release, or build
one for a Head with `mage install:tptctlHead`.  See
[Head Builds](../../docs/dev/head-builds.md) and
[Install tptctl](../../docs/docs/install/install-tptctl.md).
