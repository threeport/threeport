# Uninstall Threeport Without Threeport Config

In the event that you overwrite or delete the threeport config for a genesis cluster, you can use the Pulumi CLI to destroy the infrastructure.

This will work if you are on the machine that provisioned the genesis Threeport control plane with `tptctl up`.

> Warning: This method can leave behind dangling resources like load balancers.  `tptctl down` removes resources in a manner that cleans up these resources.  Using the Pulumi CLI is a nuke-from-orbit option that should only be used when the threeport config is unavailable.

Prerequisites:

* [pulumi CLI](https://www.pulumi.com/docs/install/)

## Pulumi State

Unless otherwise specified with the `--provider-config` flag when running `tptctl up`, the Pulumi state will be in `~/.threeport/pulumi-state`.  Inside that directory, there will be a directory with the name of the Threeport control plane.  That directory is the stack name.  Set that name in the environment and then navigate into that dir.

```bash
export PULUMI_STACK_NAME=[directory name]
cd $PULUMI_STACK_NAME
```

## Delete Infrastructure

Log in with a local backend.

```bash
pulumi login file://$(pwd)
```

Set an empty passphrase. Threeport doesn't set one.

```bash
export PULUMI_CONFIG_PASSPHRASE=""
```

Select the correct stack.

```bash
pulumi stack select $PULUMI_STACK_NAME
```

Preview the resources being destroyed.

```bash
pulumi destroy --preview-only
```

Destroy.

```bash
pulumi destroy
```
