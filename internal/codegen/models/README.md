# Codegen for Models

The code generation in this package gets called by the `threeport-codegen
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
* API version responses and object validation

