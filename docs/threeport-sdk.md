# Threeport SDK

This document covers three distinct things:

* The SDK config
* Using the SDK for Threeport development
* Development of the SDK itself

> Note: you will encounter two terms in this doc that warrant defining.  They
> both refer to generated code in the context of the SDK but have different
> meanings.
>
> *Scaffolding*: generated source code that provides a place for a Threeport
> developer to add code for business logic and custom functionality.  If the SDK
> encounters a file with scaffolding, it will _not_ regenerate that code under
> the assumption the user has added code that cannot be discarded.  It will skip
> those files.  These scaffolding files have a header comment that includes
> "intended for modification" to identify them.
>
> *Boilerplate*: generated source code that is not intended for modification and
> will be re-generated each time the `threeport-sdk gen` is run.  These
> boilerplate files have a header comment that includes "do not edit" to
> identify them.

## SDK Config

The SDK config is passed into `threeport-sdk` commands to instruct code
generation.  It is the source of truth for the project details, including the
API objects.  The definition of the API objects in `pkg/api/` is sometimes
referred to as the data model.

## Using the Threeport SDK

The Threeport SDK is used to manage core threeport/threeport as well as
extensions to the Threeport control plane.

Therefore, the Threeport SDK is used when adding new API objects and controllers
to core Threeport.  Any changes that need to be made to generated source code
(files ending in `_gen.go`) must be made to the SDK and then regenerated.

> Important: keep in mind these changes must be compatible with Threeport
> extension development since the SDK is also used for that purpose.

For complete documentation on Threeport SDK usage, see the [Threeport
documentation](https://docs.threeport.io/).

## Threeport SDK Development

### Create Command

The entry point for the `threeport-sdk create` command is
`cmd/sdk/cmd/create.go`.

The purpose of the `threeport-sdk create` command is to set up minimal
scaffolding for the SDK user to develop API object data models.  It creates the
files for API objects in `pkg/api/` that are defined in the SDK config.  The SDK
user then adds the fields to the create object scaffolding.

The code generation that is called by this command lives in
`internal/sdk/create/`.

### Gen Command

The entry point for the `threeport-sdk gen` command is `cmd/sdk/cmd/gen.go`

The purpose of the `threeport-sdk gen` command is to generate all the source
code scaffolding and boilerplate to render components that can be compiled and
deployed.  The SDK user can then add the business logic to the controllers to
provide the functionality for the project.

The `gen` command references the SDK config and the source code for the API
objects to inform this code generation.  It produces the scaffolding necessary
to add the functionality for the controllers.

The SDK config constitutes the instructions from the SDK user.  The general
order of operations for the `gen` command is:

* Consume the SDK config.
* Create a new `Generator` object.  This object is defined in
  `internal/sdk/gen/generator.go`.  The `New` method on that object populates
  that object with all the information required by the SDK for code generation.
* Generate source code for each section of the codebase by package.  The code
  generation called by this command lives in `internal/sdk/gen`.  This `gen`
  package is organized according to the structure of the Threeport SDK-managed
  project:

  * `cmd` contains the `cmd` package generation for the managed project.
  * `internalpkg` contains the `internal` package generation for the managed
    project.
  * `pkg` contains the `pkg` package generation for the managed project.
  * `root` contains the generation of source code files that live at the root of
    the managed project.

