# Threeport

Threeport is an open source application platform and platform engineering framework.

What is an application platform?  It is a system that is used to deliver software into its runtime environment and manage it over time.

What is platform engineering?  It is a software engineering discipline focused on the development of application platforms.

Threeport consists of two parts:

1. Threeport Core: The core components of an application platform.  It includes an API server, database and message broker.  For more detail, see the [Threeport Core architecture document](./architecture/threeport-core.md).
2. Threeport SDK: A framework for building independent modules to add to Threeport Core.  The SDK is Ruby on Rails for application platforms.  To learn more about the SDK, see the [Threeport SDK Introduction document](./sdk/sdk-intro.md).

## Motivation

Software delivery had been dominated by DevOps practices for the past decade or so.  This is primarily a configuration management practice.  Modern software has become increasingly complex to manage and config management is just no longer sufficient for the task.

Threeport takes a software engineering approach - rather than a config management approach - to application platforms.  "Application Orchestration" is what we call this approach.

To learn more on this topic, see the [Application Orchestration concepts document](./concepts/application-orchestration.md).

Fundamentally, Threeport exists to reduce engineering toil, improve resource
consumption efficiency, and make the most complex software systems manageable by
relatively small teams.  This leads to the delivery of more feature-rich and
more reliable software with lower infrastructure and engineering costs.

Better software.  Lower costs.

## Next Steps

Check out the [Getting Started guide](getting-started.md) to try out Threeport
for yourself.

See our [Application Orchestration
document](concepts/application-orchestration.md) in our Concepts section for more
information on how Threeport approaches software delivery.

To dive into the architecture of Threeport, see the [Threeport Core architecture](architecture/threeport-core.md).
