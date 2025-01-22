<img src="docs/dev/img/Threeport-logo-green.jpg">

Threeport is a software delivery platform.

Threeport delivers applications into remote runtime environments. Once an application has 
been built, Threeport is used to deploy and manage it over time.

Threeport consists of two primary components:
1. A functional core system for application delivery.
2. A software development kit to add custom modules to the core system.

## Threeport Core

The Threeport core is a software delivery control plane.  You can install the core system
and immediately start delivering applications to your runtime environments.

<img src="docs/docs/img/ThreeportStack.png">

The core system provides primitives to manage the following:
* Runtime Environments: Threeport provisions and manages Kubernetes clusters for your
  applications to run in.
* Support Services: Applications running on Kubernetes require support services that are 
  installed on the cluster to serve the workloads that run there. This includes things like 
  ingress request routing, TLS asset management, DNS record management, secrets management, 
  log aggregation, monitoring, etc. Threeport will install these support services and 
  configure them for your app as needed.
* Managed Services: Many applications use cloud provider-managed services such as databases 
  and object storage buckets as dependencies. Using Threeport, you can declare those 
  dependencies and those services will get provisioned and connected to your workloads when 
  deployed.
* Workloads: These are the individual pieces of software that comprise your application. 
  Threeport installs and manages these tenant workloads with their runtime, support
  service and managed service dependencies.

The core control plane has the following features:
* Orchestration: Provide a concise workload config that declares your app's dependencies and 
  the Threeport control plane coordinates the deployment of your app and satisfies its 
  dependencies without the need for multiple tools and 1000's of lines of configuration.
* Resilience: The control plane can be installed across multiple regions to provide 
  resilience and availability in the event of a cloud provider regional outage.
* Scalability: The control plane's API and controllers are massively scalable and can manage 
  thousands of applications along with their dependencies.
* Convenience: It is easy to get started and deploy simple applications. Using the Threeport 
  CLI, you can deploy Threeport locally or to your cloud provider with a single command.
* Extensibility: The core system is infinitely extensible with custom modules to provide 
  extremely low configuration overhead, even for the most complex applications.

## Software Development Kit

The software development kit enables engineers to extend core Threeport with custom modules. 
These modules can be developed by the open source community, 3rd party providers or in-house 
teams. Threeport modules can add support for any cloud provider, runtime environment, managed 
service or support service. This extensibility ensures seamless delivery of complex 
applications while minimizing configuration overhead. Custom modules developed with the SDK 
enable maximum operational efficiency, even for the most complex applications.

## Application Orchestration vs GitOps

Threeport implements Application Orchestration, an approach that uses software engineering 
rather than configuration management to handle software delivery. While GitOps (implemented 
by tools like ArgoCD and Flux) works fine for simple applications, it tends to break down as 
the applications become more sophisticated. Application Orchestration provides better 
maintainability as systems grow in complexity.

## Resources

User documentation can be found on our [user docs site](https://threeport.io/).

If you're interested in contributing to Threeport, please see our
[developer docs](docs/dev/README.md).

## Note

Threeport is still in early development. APIs may change without notice until the 1.0 
release. At this time, do not build any integrations with the Threeport API that are used 
for critical production systems. With the 1.0 release, APIs will stabilize and backward 
compatibility will be guaranteed.
