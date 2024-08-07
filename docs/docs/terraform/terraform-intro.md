# Terraform

Terraform is a popular infrastructure-as-code tool used to declare the
configuration for a set of cloud resources.  Terraform is commonly used as a CLI
tool with a set of configs that declare the cloud resources required.

> Note: Since AWS is currently the only supported cloud provider in Threeport,
> Terraform use is limited to AWS.

We offer Terraform support in Threeport since many teams have already invested
in using this tool.  However, it has clear drawbacks in the logical constructs
available in Terraform configs to specify resource configurations under
different conditions.  It is inferior to a general purpose programming language
in this respect.  Additionally, there are commonly outputs from the provisioned
resources that need to be plumbed into other configs - such as for workloads.
This output must be captured and stored in a way that it can be used elsewhere.
This interoperability with other parts of a system must be solved for and is
not ideal.

That said, Terraform can be useful for relatively simple use cases with limited
inputs (Terraform variables) and outputs.

## Terraform Definition

The definition object contains all the Terraform configs that declare the cloud
resources needed.  When you create this object with `tptctl` you can simply
provide the directory in which those configs live and they will be stored in the
Threeport database for use when creating instances of the resource stack.

Reference: [TerraformDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#TerraformDefinition)

## Terraform Instance

A Terraform Instance takes the Terraform variables (usually in a file with the
extension `.tfvars`) that provides the inputs for the config in a definition.
When you create a Terraform Instance, the variables will be passed to the config
and the resources defined in the Terraform Definition will be provisioned.

Once the resources have been provisioned the Terraform controller will store the
outputs in the Threeport database so they can be retrieved.

Reference: [TerraformInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#TerraformInstance)

## Next Steps

Check out our [Deploy RDS Instance with Terraform guide](rds-database.md) for a
walk through on how to use Terraform in to provision cloud resources.

