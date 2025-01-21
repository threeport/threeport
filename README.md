<img src="docs/dev/img/Threeport-logo-green.jpg">

Threeport is a platform engineering toolkit and framework for building
advanced application platforms.

Threeport is used to deliver applications to their runtime environment.  Once
an application has been built and needs to be deployed to servers in a data
center, Threeport can be used to manage that deployment as well as the ongoing
management of the application.

Threeport consists of two primary components:
1. A functional core application platform.
2. A software development kit to add custom modules to the core platform.

## Core Application Platform

Application platforms are used to deliver applications to their runtime environment and
make those apps available to end users.  Platform engineers use Threeport to build application
platforms that are used by software developers and operations engineers to deploy and
manage those apps.

The Threeport core application platform is a control plane that manages the delivery of applications
to their runtime environment.  The core platform provides primitives to manage
the following:
* Runtime Environment: Threeport provisions and manages Kubernetes clusters for you applications
  to run in.
* Support Services: Applications running on Kubernetes require
  support services that are installed on the cluster to serve the
  workloads that run there.  This includes things like ingress request routing,
  TLS asset management, DNS record management, secrets management, log
  aggregation, monitoring, etc.  Threeport will install these support services
  and configure them for your app as needed.
* Managed Services: Many applications use cloud provider-managed
  services such as databases and object storage buckets as dependencies.  Using
  Threeport, you can declare those dependencies and those services will get
  provisioned and connected to your workloads when deployed.
* Workloads: These are the individual pieces of software that comprise your application.  Whether
  you have a single monolithic application or a distributed architecture with
  microservices, Threeport will manage them all seamlessly.

The core Threeport control plane has the following features:
* It can be installed across multiple regions to provide resilience
  and availability in the event of a cloud provider regional outage.
* It is massively scalable and can manage thousands of applications
  along with their dependencies.
* It is easy to get started and deploy simple applications.  Using
  the Threeport CLI, you can deploy an app platform locally or to your
  cloud provider with a single command.
* It is infinitely extensible with custom modules to provide extremely
  low configuration overhead, even for the most complex applications.

You can use an unmodified, core Threeport control plane to deliver simple applications
to their runtime environment.  Using the Threeport SDK, you can also extend the Threeport control plane
with custom modules to enable any additional functionality.

## Software Development Kit

The software development kit is a command line tool and collection of libraries that enable platform engineers
to extend the core Threeport app platform with custom modules.

In order to achieve maximum operational efficiency in software delivery, even for the most complex applications,
platform engineers can extend the core Threeport app platform with custom modules.  These custom modules
can be developed by the open source community or an organization's in-house platform engineering team.
Custom modules can be added to Threeport to add support for any cloud provider, any runtime environment,
any managed service and any support service to make the delivery of even the most complex applications
seamless and reliable with minnimal configuration overhead on the part of the development and operations teams.

## Application Platforms

App platforms are the systems we use to deliver applications into their runtime environment.

App platforms often suffer from one of the following problems:
1. They are easy to use but not flexible enough to support the needs of advanced
   applications without significant configuration overhead.
2. They are flexible but not easy to use and require significant, complex
   configuration, regardless of the use case.

This results in a lot of toil in software delivery and a common need
to replatform applications when platform constraints are encountered.
Replatforming is a very costly and time-consuming process.

Threeport provides an easy-to-use core app platform substrate that can
be use to manage delivery of simple applications.  It also provides a
software development kit that makes the platform infinitely extensible
with custom modules.  So you can get up and running quickly with a simple
app platform and then incrementally extend it with custom modules as needed
over time.  Threeport removes the need to ever replatform your applications again.

## Application Orchestration vs GitOps

We call our approach to software delivery "Application Orchestration."  Threeport
is an implementation of Application Orchestration, just as ArgoCD and Flux are
implementations of the GitOps approach.

Application orchestration is an approach to software delivery that focuses on
software engineering to solve the problem.  In contrast, the GitOps approach
uses config management and different pre-existing tools that often don't integrate
well.  The GitOps approach works well for simple applications, but often leads
to significant toil and configuration overhead as systems mature and grow.

## Resources

User documentation can be found on our [user docs site](https://threeport.io/).

Developer documentation can be found [here](docs/dev/README.md).

## Note

Threeport is still in early development.  APIs may change without notice until
the 1.0 release.  At this time, do not build any integrations with the Threeport
API that are used for critical production systems.  With the 1.0 release, APIs
will stabilize and backward compatibility will be guaranteed.
