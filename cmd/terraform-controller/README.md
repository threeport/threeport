# Threeport Terraform Controller

Manage cloud infrastructure in Threeport with Terraform.

This controller allows users to provide Terraform configs that are saved in the
Threeport database as TerraformDefinitions and then used as TerraformInstances
to deploy cloud infra.  Terraform values can be supplied with TerraformInstances
to specify runtime configurations.  The ouput values from Terraform can then be
retrieved through the Threeport API.

Terraform provides limited programmatic interoperability and is primarily
supported in Threeport for those that have pre-defined Terraform configs they
wish to leverage.  We recommend alternative methods for provisioning cloud
infrastructure that are more extensible and offer more usefult integrations in
the Threeport control plane.

