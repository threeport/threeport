# Helm Workloads

[Helm](https://helm.sh/) is a popular package manager for Kubernetes.  Helm
charts use templates to render Kubernetes resource manifests.
Helm is generally used as a command line tool and/or incorporated into a GitOps
pipeline.

In Threeport, Helm is offered so that teams already using Helm can get started
using the charts they have or currently use.  Helm is useful for relatively uncomplicated
deployments or when using community-supported projects that have Helm charts
available.  However, the templating used by Helm breaks down in sophisticated,
production environments when the templates become overwhelmed with conditionals
and loops.  In more advanced use-cases, we recommend using custom Kubernetes
operators and/or Threeport controllers to programmatically manage complex
configuration of software delivery.  See our documentation on [Threeport
Extensions](../concepts/extensions.md) for more information on
this topic.

> Note: In order to use Helm charts in Threeport, the chart must be hosted on a
> Helm repo.

## Helm Workload Definition

A Helm Workload Definition allows you to specify the repo URL, chart name, chart
version and a set of Helm values that will be used each time an instance of the
Helm chart is deployed.  The Helm values available on the definition allow you
to set default values (that may differ from the defaults applied on the upstream
project) for each instance deployed.

Reference:
[HelmWorkloadDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#HelmWorkloadDefinition)

## Helm Workload Instance

An instance of a Helm Workload allows you to provide additional Helm values that will
override any values provided on the definition.  You can also specify which
Kubernetes Runtime Instance to deploy to.

Reference:
[HelmWorkloadInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#HelmWorkloadInstance)

## Next Steps

See our [Local Helm Workload guide](../helm-workloads/deploy-helm-local.md) for a walk through on
using Helm in Threeport.

