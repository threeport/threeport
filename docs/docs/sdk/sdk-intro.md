# Threeport SDK Introduction

The Threeport SDK is a command line tool that enables software engineers to rapidly
develop custom modules for Threeport.

## How It Works

A software engineer uses the Threeport SDK to build modules that provide
custom abstractions and functionality for their development teams to use.

Once the requirements are well understood, the process to build a custom
Threeport module is as follows:

1. Initialize a Threeport module project. The engineer starts a new
   project with its own git repo and uses the `threeport-sdk` CLI tool to
   initialize the repository which scaffolds all the directories and code needed
   to compile their software.
1. Create one or more new API objects. The engineer designs a data
   model based on module requirements and uses `threeport-sdk` to generate
   its source code scaffolding. Then the engineer updates this code with fields
   that define the object's attributes to meet the project's requirements.
1. The generated scaffolding includes functions to add logic for
   reconcilers that will manage the state of the new custom object. This
   reconciliation logic will be compiled into one or more custom controllers
   that will be added to the Threeport core system.
1. The engineer then compiles the module's controller and API server into
   binaries and builds container images using built-in utilities provided by
   the SDK.
1. The engineer deploys the custom module into a Threeport installation using
   built-in utilities and makes it available for testing and feedback.

> Organizations that support open source and community engagement can make their
> modules publicly available in case they may be applicable to other use
> cases.

## Next Steps

Check out the [SDK Tutorial](tutorial.md) to get started building with the Threeport SDK.
