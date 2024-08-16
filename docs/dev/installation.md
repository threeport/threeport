# Installation

You can install a threeport control plane using the command:

```bash
tptctl up -n dev-0
```

The name of the control plane being deployed is supplied via the `-n` flag.
This spins up a new threeport control plane from scratch while spinning up the required infrastructure for it to run on.
The infrastructure on which you deploy the control plane can be changed via the `provider` flag and defaults to a `kind` cluster.

The `tptctl up` command take an optional flag for a config file path. This is the threeport config used to reference control plane information. More information can be found [here](threeport-config.md).

Similiarly, the following command can be used to bring down a control plane that was created via the `up` command.

```bash
tptctl down -n dev-0
```