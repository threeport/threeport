# Codegen for Versions

The code generation in this package gets called by the `threeport-codegen
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
* object validation
* API version responses

