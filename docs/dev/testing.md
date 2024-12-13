# Testing

Tests are run in CI and must pass before merging PRs to feature branches.

## End-To-End

The end-to-end tests use `tptctl` to deploy workloads and verify their results.

The end-to-end tests will build `tptctl` and `tptdev`, create a local container
registry, build and push all control plane images to that local registry, and
provision and new genesis control plane using the newly built images.

It will then run the test suite.  Afterwards, it will tear down the control
plane and local registry.

```bash
mage test:e2eLocal
```

The e2e tests create and use a Threeport config located at `/tmp/e2e-threeport-config.yaml`

## Integration

The integration tests verify internal integrations, such as client library
functionality.

The end-to-end tests require an existing control plane.  You can either
provision on with `mage dev:up` or use the control plane used by e2e tests.

### Using tptdev

```bash
mage dev:up
mage test:integration
```

### Using e2e

The following mage target and arguments will spin up a local kind cluster and
local container registry, then run e2e tests.  The final `false` argument does
not clean up the kind cluster and registry.

```bash
mage test:e2e kind local false
```

You can now use this control plane for integration tests.

```bash
export THREEPORT_CONFIG=/tmp/e2e-threeport-config.yaml
mage test:integration
```

You can then clean up the kind cluster and local registry:

```bash
mage test:e2eClean
```

