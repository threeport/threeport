# Threeport Secret Controller

Manage sensitive secrets for workloads.

This controller gives Threeport the ability to create secret values in a secret
vault - today the only supported provider is AWS Secrets Manager.  These secrets
are managed with SecretDefinition objects.  SecretInstance objects represent an
instance of a secret being used by a workload and Threeport manages exposing the
secret values to the workload.

