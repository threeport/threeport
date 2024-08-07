# Secrets

If your application needs to access sensitive values that are stored in a secret
vault, Threeport supports this requirement with Secrets as a support service.
Under the hood, Threeport uses a project called
[external-secrets](https://github.com/external-secrets/external-secrets).

> Note: Currently the only supported secret vault is [AWS Secrets
> Manager](https://docs.aws.amazon.com/secretsmanager/latest/userguide/intro.html).
> We plan to support other secret vaults in the future.

## Secret Definition

The secret definition represents some secret value.  The secret definition
stores the AWS account ID where the secret should be stored in AWS Secrets
Manager and the secret data as JSON>

Reference: [SecretDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#SecretDefinition)

## SecretInstance

A secret instance is an instance of a secret value being exposed to a workload.
It is a union of a workload instance and secret definition.  When a secret
instance is created the secret data is exposed to the workload.

Reference: [SecretInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#SecretInstance)

## Next Steps

Our [Deploy Workload with Secret on AWS guide](../workloads/deploy-workload-aws.md)
walks through the use of secrets with workloads on Threeport.

