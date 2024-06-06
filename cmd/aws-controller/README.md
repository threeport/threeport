# Threeport AWS Controller

Manage AWS resources as workload dependencies.

Here you will find the main package for the Threeport aws controller.  It is
responsible for reconciling AwsEksKubernetesRuntime Instances and Definitions.
It interfaces with the AWS API in order to manage EKS clusters accordingly.

It is also responsible for managing RDS and S3 as workload dependencies.

The AWS controller currently uses
[aws-builder](https://github.com/nukleros/aws-builder) as a library for managing
these resources.  The aws-builder library uses the AWS Golang SDK v2 rather than
an intermediary tool such as Terraform or Crossplane.

