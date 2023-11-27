<img src="docs/img/threeport-logo-green.jpg">

An application orchestration control plane.

Threeport exists to provide useful abstractions for running applications and
their dependencies.  The threeport control plane orchestrates the workloads that
comprise your applications by treating the following as app dependencies:
* Infrastructure: Threeport provisions and manages infrastructure as a dependency
  of your app.
* Kubernetes: Using threeport, you no longer have to install and manage
  Kubernetes clusters.  It is done for you by threeport.
  Kubernetes provides container orchestration and serves as the runtime environment
  for threeport workloads.  We aim to relieve you and your software delivery systems
  of the need to use `kubectl` or interact with the Kubernetes API when deploying
  your apps.  Threeport will manage as many clusters as your requirements dictate
  without you having to develop your own Kubernetes federation system.
* Support Services: Applications running on Kubernetes require
  support services that are installed on the cluster to serve the
  workloads that run there.  This includes things like ingress request routing,
  TLS asset management, DNS record management, secrets management, log
  aggregation, monitoring, etc.  Threeport will install these support services
  and configure them for your app as needed.
* Managed Services: Many applications use cloud-provider managed
  services such as databases and object storage buckets as dependencies.  Using
  threeport, you can declare those dependencies and those services will get
  provisioned and connected to your workloads at runtime.
* Workloads: In addition to your primary user-facing application deployments,
  threeport manages any services you build in-house as a part of a distributed
  architecture (microservices).  These are declared as workload dependencies and
  can be nested as your requirements dictate.

In summary, threeport provides a global control plane for your application
deployments using battle-tested designs and best practices so you can trust that
your software will run reliably.  This frees you to concentrate on develivering
value to your users.

User documentation can be found on our [user docs site](https://docs.threeport.io/).

Developer documentation can be found [here](docs/README.md).

## Note

Threeport is still in early development.  APIs may change without notice until
the 1.0 release.  At this time, do not build any integrations with the threeport
API that are used for critical production systems.  With the 1.0 release, APIs
will stabilize and guarantee backward compatibility.  Additionally, there are
security concerns and observability systems which are crucial for production
that are not yet implemented.

## Managed Threeport Providers

[Qleet](https://qleet.io) provides a fully managed threeport service that
lets teams deliver their software into their own cloud provider accounts using
Threeport.

