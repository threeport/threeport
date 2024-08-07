# Install Threeport Locally

This guide provides instructions for installing Threeport on
[kind](https://kind.sigs.k8s.io/).  We will run Kubernetes in docker
containers on your local machine and install the Threeport control plane there.
It requires you have docker installed on your machine.  This install method is
useful for testing out Threeport to get an idea of how it works.

If you would like to install Threeport on AWS for a more realistic experience of
how Threeport is generally used, see our guide to [Install Threeport on
AWS](install-threeport-aws.md)

Note: this guide requires you have our tptctl command line tool installed.  See
our [Install tptctl guide](install-tptctl.md) to install if you haven't already.

## Docker

In order to run Threeport locally, you must first have [Docker
Desktop](https://docs.docker.com/desktop/install/mac-install/) installed if on a
Mac or [Docker Engine](https://docs.docker.com/engine/install/) on Linux.

If you are on Ubuntu you can install and add your user to the docker group as
follows:

```bash
sudo apt-get install gcc docker.io
sudo usermod -aG docker $USER
```

## Install Threeport

To install the Threeport control plane locally:

```bash
tptctl up \
    --name=test \
    --provider=kind \
    --auth-enabled=false
```

The `--provider` flag indicates that we're using kind to provision the
underlying Kubernetes cluster.  The `--name` flag provides an arbitrary name for
this control plane instance.  And the `--auth-enabled` flag indicates we want to
turn off user authentication which is turned on by default and should only be
turned off when testing locally.  Disabling auth will allow us to more easily
explore the API in a subsequent step.

It will take a few minutes for this process to complete.

This will create a local kind Kubernetes cluster and install all of the control
plane components.  It will also register the same kind cluster as the default
compute space cluster for tenant workloads.

## Validate Deployment

If you have [kubectl](https://kubernetes.io/docs/tasks/tools/) installed and
wish to view the pods that constitute the Threeport control plane:

```bash
kubectl get pods -n threeport-control-plane
```

Note: if you notice any pods crashlooping, give them a few minutes.  The
Threeport controllers depend on the API server which, in turn, depends on the
database and message broker.  Each component will come up once its dependencies
are running.

## Swagger Documentation

Threeport API endpoints are documented with [Swagger](https://swagger.io/) at
`$THREEPORT_API_ENDPOINT/swagger/index.html`. This is most easily
accessed by setting `--auth-enabled=false` on a Threeport control plane
deployed to Kind and visiting
[http://localhost/swagger/index.html](http://localhost/swagger/index.html).

## Next Steps

Next, we suggest you deploy a sample workload locally using Threeport.  See our
[Deploy Workload Locally guide](../workloads/deploy-workload-local.md) for instructions.

## Clean Up

If you're done for now and not installing a workload locally, you can
uninstall the Threeport control plane:

```bash
tptctl down --name test
```

