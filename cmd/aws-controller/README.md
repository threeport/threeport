# Threeport AWS Controller

Manage AWS resources as workload dependencies.

Here you will find the main package for the threeport aws controller.  It is
responsible for reconciling AwsEksKubernetesRuntime Instances and Definitions.
It interfaces with the AWS API in order to manage EKS clusters accordingly.

In future, it will also be responsible for managing other managed services such
as S3 and RDS to serve workloads.

