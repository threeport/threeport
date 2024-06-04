# Threeport Helm Workload Controller

Manage containerized worklaods on Kubernetes using Helm charts.

This controller manages HelmWorkloadDefinitions and HelmWorkloadInstances
objects in Threeport.  This controller currently only supports Helm charts that
are hosted on a Helm repo.  It does not provide for loading new
locally-developed Helm charts at this time.  Helm values can be supplied to
definition objects to provide universal defaults.  Helm values can be supplied
to instance objects to override those values at deploy time.

We recommend the Kubernetes operator pattern or Threeport controllers for
defining workload using programming languages (preferrably Go) to manage runtime
parameters for workloads.  We provide support for Helm primarily for early
implementations, or when using community-supported projects that provide Helm
charts for installation on Kubernetes.

