# internal/sdk

Here you will find the `models` and `versions` packages.  These packages are
used by the `threeport-sdk` CLI.  More general info about it can be found in
the docs for that tool [here](../../cmd/sdk/README.md).

# Codegen for Models

The code generation in this package gets called by the `threeport-sdk codegen 
api-model` command.  This command is called when `go generate` is run as
instructed by the `//go:generate` comments at the top of the API model files in
`pkg/api/<version>/`.  It is called many times as there are many different files
containing model definitions.

It generates code for each model based on the source code found in that API
model file:
* client library code
* API handler code
* methods for all the objects that comprise the API data model
* API routes
* validation of objects received by the API from clients
* API version responses

# Codegen for Versions

The code generation in this package gets called by the `threeport-sdk codegen 
api-version` command.  This commannd is called just one time when `go generate`
is run.  The `//go:generate` comment in `cmd/rest-api/main.go` triggers it.

That command iterates over all versions of all APIs in `pkg/api` and generates
code based on what it finds there.  It skips files that have exclude markers at
the top of the file, e.g. `pkg/api/v0/common.go`.  It also skips generated code
files that have the `_gen.go` suffix.

The code generated includes:
* database initialization
* object type mappings for API responses
* adding all routes to the server when API starts
* object field validation maps
* API version mapping

