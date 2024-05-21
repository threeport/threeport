# internal/sdk

This package contains all the functionality for the Threeport SDK.  The
functions here are called from the commands in `cmd/sdk/cmd/`.

It includes the data model for the SDK config which is the configuration the SDK
user provides to the SDK to configure the code generation.

## Code Generation Library

The SDK makes extensive use of the [jennifer](https://github.com/dave/jennifer)
library which uses Go to generate Go source code.

## Organization

The code is first organized by the command they serve.

The `create` package
contains the code generation for the `threeport-sdk create` command.  That
command generates minimal scaffolding for the API objects.  It ingest the
SDK config to inform this code gen.

The 'gen' package contains the code generation for the `threeport-sdk gen`
command.  This command ingests the SDK config, reads the project's go.mod file
and parses the API object source code.  This ingestion is used to build the
`Generator` object which is defined in `gen/generator.go`.  That generator
contains the values for generating the rest of the source code.  The `gen`
package is organized by the project packages that code is being generated for.
So if you're looking for the SDK function that generates code for the api-server
package, you'll find it in `gen/pkg`.

