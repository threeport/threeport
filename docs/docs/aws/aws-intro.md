# AWS

Amazon Web Services (AWS) is currently the only supported cloud provider on
Threeport.

The following services can be managed with Threeport objects:

* Elastic Kubernetes Service (EKS) for Kubernetes Runtimes.
* Relational Database Service (RDS) for managed databases as an application
  dependency.
* Simple Storage Service (S3) for object storage buckets as an application
  dependency.

There are many other AWS resources that are managed by Threeport to deliver these
services.  VPCs, subnets, elastic load balancers are all managed in service of
the supported services on Threeport, but Threeport users need not configure or
provision these separately.

## AWS Account

An AWS Account object allows you to register AWS account information with
Threeport so that it can be used to deploy runtimes, workloads and managed
services in that account.  A
[genesis](../../control-planes/control-plane-intro#control-plane-instance)
Threeport control plane deployed to AWS will utilize AWS best-practice
[IRSA](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)
authentication to manage resources within its own AWS account.  To give
Threeport access to other AWS accounts, you must include an AWS account ID,
access key ID, and secret access key credentials to authenticate. If you have
your local AWS config set up to use the `aws` CLI tool you can reference those
credentials stored on your local file system when creating an external AWS
account.

You can register and use as many AWS accounts in Threeport as you wish.

Reference:
[AwsAccount](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#AwsAccount)

## AWS EKS Kubernetes Runtime Definition

This object allows you to configure an AWS EKS cluster directly.  We recommend
using the `KubernetesRuntimeDefinition` object with the `InfraProvider` field
set to `eks` to provision EKS clusters.  However, if there is a specific EC2
instance type that you'd like to use that isn't offered through the Threeport
NodeProfile and NodeSize abstractions, you can directly provision EKS clusters
using this object.

When you create one of these objects, Threeport will create a corresponding
Kubernetes Runtime Definition so that it can be referenced by the system as
needed.

Reference:
[AwsEksKubernetesRuntimeDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#AwsEksKubernetesRuntimeDefinition)

## AWS EKS Kubernetes Runtime Instance

This object allows you to provision an instance from the config in a definition.
Similar to the definition, we recommend using the `KubernetesRuntimeInstance` to
provision EKS clusters in AWS.  However, if you need to specify a region not
offered through the Threeport Location abstraction, you can use this object.

When you create one of these objects, Threeport will create a corresponding
Kubernetes Runtime Instance so that it can be referenced by the system as
needed.  This Kubernetes Runtime Instance contains the connection information
for the Kubernetes API that is used by the workload controller to deploy
resources.

Reference:
[AwsEksKubernetesRuntimeInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#AwsEksKubernetesRuntimeInstance)

## AWS Relational Database Definition

This object allows you to define an RDS instance configuration.  You can specify
the engine (one of `mysql`, `postgres` or `mariadb`) and the version of that
engine.  You can also specify the name of the database the client workload will
connect to, the port, the machine size to use for the database, the amount of
storage to provision as well as the number of days to retain database backups.
If you specify `0` for the `BackupDays` field, no backups will be kept.  You can
also specify the AWS account to use for the database.

The field that is important to connecting it to the client workload is the
`WorkloadSecretName` field.  This field tells Threeport what name to give to the
Kubernetes secret that will provide the database connection credentials to the
workload connecting to the database.  Threeport will create a Kubernetes
secret with the following keys:

* `db-endpoint`: The network endpoint at which the RDS instance is available.
* `db-port`: The port the client workload can connect to the database on.
* `db-name`: The name of the database the client workload will use.
* `db-user`: The database user name the client workload uses to authenticate.
* `db-password`: The client workload's user password to authenticate to the DB.

When constructing the Kubernetes resource manifest for the workload, configure
your pods to retrieve these values from the specified secret as an environment
variable.  If you're not sure how to do this, see our [Deploy Workload on AWS
guide](../workloads/deploy-workload-aws.md) for a detailed walk through of an app
on Kubernetes using an RDS database.

Reference:
[AwsRelationalDatabaseDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#AwsRelationalDatabaseDefinition)

## AWS Relational Database Instance

This object represents a deployed instance of RDS as configured by the
definition.  This object connects the instance to the Workload that will use the
DB.

Reference:
[AwsRelationalDatabaseInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#AwsRelationalDatabaseInstance)

## AWS Object Storage Bucket Definition

This object allows you to configure an S3 bucket for use by an application.  You
can nominate whether the bucket should have public read access or not.  Public
read access is useful for serving static assets for a web front end.  Otherwise,
if the data to be stored on S3 is private, you will not want public read access
(which is the default).

You also need to provide a value for the `WorkloadServiceAccountName` field.
Threeport uses IAM Roles for Service Accounts (IRSA) to provide access to the S3
bucket for your workload.  This means you'll need to include a Kubernetes
Service Account with a matching name in the Kubernetes manifests in the
WorkloadDefinition for your workload that will use S3.  If you're unsure how to
do this see our [Deploy Workload on AWS
guide](../workloads/deploy-workload-aws.md) for a detailed walk through of an app
on Kubernetes that also uses S3.

Lastly, you'll need to provide the environment variable your workload will use
to reference the name of the S3 bucket.  This environment variable will be added
to your workload by Threeport.  Your app just needs to know what env var to
reference.

Reference:
[AwsObjectStorageBucketDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#AwsObjectStorageBucketDefinition)

## AWS Object Storage Bucket Instance

This is a deployed instance of S3 that connects it to a WorkloadInstance object.

Reference:
[AwsObjectStorageBucketInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#AwsObjectStorageBucketInstance)

## Next Steps

Check out our [Deploy Workload on AWS guide](../workloads/deploy-workload-aws.md)
see an example of how to deploy a workload that is connected to an RDS database
and S3 bucket.

