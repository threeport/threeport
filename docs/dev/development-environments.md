# Development Environments

## New Dev Environment

The following instructions allow you to stand up a local Threeport control plane
built from the code you have locally.

Create a local container registry.  This allows you to build new container images
and use them without having to wait for pushes to - and pulls from - a remote
registry.

```bash
mage createLocalRegistry
```

Build contianer images for each of the control plane components and push them to
the local container registry.

```bash
make build-tptdev
./bin/tptdev build -r localhost:5001 -t dev --push
```

Install a local control plane using images pulled from the local registry.

```bash
./bin/tptdev up -r localhost:5001 -t dev --local-registry
```

## Update Dev Environment

If you need to update the image for a control plane component after making code
changes, do the following.

Build a new container image and load it into the development kind cluster.  The
following example is for the workload controller.

```bash
./bin/tptdev build -r localhost:5001 -t dev --load --names workload-controller
```

Then delete the pod for the controller.  When it restarts, it will use the new
image.

```bash
kubectl delete po [workload controller pod name]
```

## Remove a Dev Environment

Spin down the control plane cluster.

```bash
./bin/tptdev down
```

Stop and remove the registry container.

```bash
mage cleanLocalRegistry
```

