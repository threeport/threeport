# Machine Runtimes

A Machine Runtime is an SSH-reachable host that serves as a runtime for
workloads outside of Kubernetes.  This covers virtual machines, bare-metal
servers, and edge devices.  Workloads that target a Machine Runtime are
defined as shell scripts that run over the existing SSH connection.

## Machine Runtime Definition

The definition holds template-level configuration shared across machine
runtime instances: the infrastructure provider, the machine type and the
image.  Those fields cannot change after create.

## Machine Runtime Instance

A machine runtime instance represents a single host.  It carries the
hostname or IP, SSH user, port, an optional SSH private key or password,
and an optional host key.  The SSH key and password are encrypted at rest.
If the host key is not supplied at create time, the controller captures it
on the first successful connection and stores it for subsequent identity
verification.

An instance can also carry the region, the network and the subnet.  Those
three cannot change after create.  A definition with an infrastructure
provider requires the instance region at create.

## Next Steps

Threeport currently supports importing machines that have already been
deployed by registering an existing SSH-reachable host as a Machine Runtime
Instance.  Provisioning new hosts via Threeport is not yet supported.

Once a host is registered, see the [Machine Workloads
introduction](machine-workload-intro.md) to define and deploy workloads on
it.

