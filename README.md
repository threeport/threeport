<img src="docs/dev/img/threeport-logo-green.jpg">

Threeport is an open source application platform and platform engineering framework.

What is an application platform?  It is a system that is used to deliver software into its runtime environment and manage it over time.

What is platform engineering?  It is a software engineering discipline focused on the development of application platforms.

Threeport consists of two parts:

1. Threeport Core: The core components of an application platform.  It includes an API server, database, message broker and core modules.  Modules abstract and automate distinct platform engineering concerns.  For more detail, see the [Threeport Core architecture document](https://threeport.io/architecture/threeport-core).
2. Threeport SDK: A framework for building independent modules to add to Threeport Core.  The SDK is Ruby on Rails for application platforms.  To learn more about the SDK, see the [Threeport SDK Introduction document](https://threeport.io/sdk/sdk-intro/).

## Motivation

Software delivery had been dominated by DevOps practices for the past decade or so.  This is primarily a configuration management practice.  Modern software has become increasingly complex to manage and config management is just no longer sufficient for the task.

Threeport takes a software engineering - rather than a config management - approach to application platforms.  We call this "Application Orchestration."

To learn more on this topic, see the [Application Orchestration concepts document](https://threeport.io/concepts/application-orchestration).

Fundamentally, Threeport exists to reduce engineering toil, improve resource
consumption efficiency, and make the most complex software systems manageable by
relatively small teams.  This leads to the delivery of more feature-rich and
more reliable software with lower infrastructure and engineering costs.

Better software.  Lower costs.

## Resources

User documentation can be found on our [user docs site](https://threeport.io/).

If you're interested in contributing to Threeport, please see our
[developer docs](docs/dev/README.md).

## Note

Threeport is still in relatively early development. APIs may change without notice until the 1.0 release. At this time, do not build any integrations with the Threeport API that are used for critical production systems. With the 1.0 release, APIs will stabilize and backward compatibility will be guaranteed.
