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

### On AWS

Two mage targets run the same suite against a control plane on AWS EKS
instead of kind.  On top of the workload tests, they check that IAM roles for
service accounts work on the cluster.  After the teardown, they fail if any
AWS resource the install created is still there.

They create real infrastructure that costs money for as long as it runs.
They need `ginkgo` and the AWS CLI.

`test:e2eAws` installs the released control plane images.  It takes the AWS
region and whether to clean up afterwards, and uses the `default` AWS config
profile:

```bash
mage test:e2eAws us-east-1 true
```

`test:e2eAwsDev` installs control plane images built from your working tree,
so a branch can be tested on EKS before it is released.  It takes the region,
the AWS config profile and whether to clean up:

```bash
mage test:e2eAwsDev us-east-1 my-profile true
```

- It builds the images for amd64, the architecture of the EKS nodes, from any
  machine.
- It pushes them to ECR in the profile's account, so it also needs Docker.
- It creates an ECR repository for each image that doesn't have one yet.  The
  repositories are left in place for the next run.

With `false` as the last argument, the control plane and its EKS cluster are
left running.  Remove them with:

```bash
./bin/tptctl down --name e2e-test --threeport-config /tmp/e2e-threeport-config.yaml
```

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

