# Deploy RDS Instance with Terraform

In this guide, we're going to use a Terraform definition and instance to deploy
an instance of the AWS Relational Database Service (RDS) using Threeport.

You'll need an active AWS account to follow this guide.

> Note: At this time, you can only use Terraform to deploy resources on AWS with
> Threeport.

## Prerequisites

You'll need a Threeport control plane for this guide.  Follow the [Install
Threeport Locally guide](../install/install-threeport-local.md) to set that up a
local control plane or the [Install Threeport on AWS
guide](../install/install-threeport-aws.md) for a remote control plane.  Either
will work.

## Work Space

First, create a temporary work space on your machine.

```bash
mkdir threeport-terraform-test
cd threeport-terraform-test
```

## AWS Account

If you installed Threeport on AWS, you'll already have an AWS account available
to use (the same one used to deploy EKS for the Threeport control plane).  If
you'd like to use a different AWS account for this guide - or if you installed
Threeport locally, you'll need to register an AWS account with Threeport.

Download a config to register AWS with Threeport.

```bash
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/aws/default-aws-account.yaml
```

Open this file:

```yaml
AwsAccount:
  Name: default-account
  AccountID: "555555555555"
  DefaultAccount: true

  # option 1: provide explicit configs/credentials
  #DefaultRegion: us-east-1
  #AccessKeyID: "ABCDEABCDEABCDEABCDE"
  #SecretAccessKey: "123abcABC123abcABC123abcABC123abcABC123a"

  # option 2: use local AWS configs/credentials
  LocalConfig: /path/to/local/.aws/config
  LocalCredentials: /path/to/local/.aws/credentials
  LocalProfile: default
```

Edit the file to make the following changes:

1. On line 2, update the `AccountID` with the value for your account.
1. If using option 1, enter the `DefaultRegion` you'd like to use on line 7 and
   add the keys on lines 8 and 9.  If you're unsure how to create access keys
   for AWS, see the [AWS
   guide for managing access
   keys](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html).
1. If using option 2, you'll need to have your local config and credentials set
   up.  If you have set up the AWS CLI in the past, you'll likely have this
   ready.  If not, see the [AWS guide to set up the AWS
   CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-quickstart.html).
   With this set up, enter the file paths for the config and credentials along
   with the profile you'd prefer to use.

Now, register the AWS account with Threeport.

```bash
tptctl create aws-account --config default-aws-account.yaml
```

## Create Terraform Definition

The Terraform definition includes the Terraform configs to deploy some AWS
resources.

Download a sample Threeport config.

```bash
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/terraform/rds-terraform-definition.yaml
```

The sample config for a Terraform definition looks as follows:

```yaml
TerraformDefinition:
  Name: rds-instance
  ConfigDir: config
```

It indicates that it will look for Terraform configs in a directory `config`.

Let's create that directory and download sample Terraform configs.

```bash
mkdir config
curl -O --output-dir config https://raw.githubusercontent.com/threeport/threeport/main/samples/terraform/config/main.tf
curl -O --output-dir config https://raw.githubusercontent.com/threeport/threeport/main/samples/terraform/config/outputs.tf
curl -O --output-dir config https://raw.githubusercontent.com/threeport/threeport/main/samples/terraform/config/variables.tf
```

Now we can create the Terraform definition:

```bash
tptctl create terraform-definition --config rds-terraform-definition.yaml
```

The Terraform configs for an RDS instance are now stored in Threeport.  Next, we
can create an instance from that definition.

## Create Terraform Instance

This step will actually deploy the RDS instance on AWS.

Download a sample config.

```bash
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/terraform/rds-terraform-instance.yaml
```

The config looks as follows:

```yaml
TerraformInstance:
  Name: rds-instance-01
  VarsDocument: config/terraform.tfvars
  TerraformDefinition:
    Name: rds-instance
  AwsAccount:
    Name: default-account
```

This config specifies:

* An arbitrary name for the RDS instance
* Terraform variables to use for this deployment
* The name of the definition we just created
* The AWS account we registered earlier

Download Terraform a sample variables file:

```bash
curl -O --output-dir config https://raw.githubusercontent.com/threeport/threeport/main/samples/terraform/config/terraform.tfvars
```

Open the `config/terraform.tfvars` file.

```toml
region = "us-east-2"
vpc_id = "vpc-asdf1234asdf12345"
subnet_ids = ["subnet-asdf1234asdf12345", "subnet-asdf1234asdf12345",
"subnet-asdf1234asdf12345"]
db_port = 3306
app_security_group = "sg-asdf1234asdf12345"
db_password = "unsecurepwd"
```

There are several values we'll have to set for your environment:

* Set the `region` to your desired AWS region.
* Log into the AWS console and go to the VPC dashboard.  If you have a Threeport
  control plane installed in AWS, select the VPC where that is installed.
  Otherwise, use your default VPC.  Enter the VPC ID on line 2 in the tfvars file.
* Go the subnets dashboard in the AWS console.  Select the subnets used for
  Threeport, or the default subnets.  Enter the subnet IDs on line 3.
* For the security group, use your default security group ID on line 6.  We
  won't be connecting an app in this guide so it is not important.
* Change the password to something unique on line 7.

Now we can create the Terraform instance.

```bash
tptctl create terraform-instance --config rds-terraform-instance.yaml
```

If you go to the AWS RDS console, you'll shortly see the RDS instance being
created.  Terraform takes a few minutes to initialize and deploy resources, so
it won't be immediate.  Give it a little time.

You can view your Terraforms with this command:

```bash
tptctl get terraforms
```

Once the RDS instance is up, you should see output similar to this:

```bash
NAME              TERRAFORM DEFINITION     TERRAFORM INSTANCE     AWS ACCOUNT          STATUS       AGE
rds-instance      rds-instance             rds-instance-01        default-account      Healthy      11m42s
```

## Get Terraform Outputs

In order to connect to the database, there are several outputs from Terraform
that you'll need to retrieve.  These are stored securely in the Threeport
database.

Once the RDS instance is up, get those outputs as follows:

```bash
tptctl describe terraform-instance -n rds-instance-01 -o yaml
```

You'll notice the sensitive encrypted values are redacted.  You can view those
values by requesting specific fields.

```bash
tptctl describe terraform-instance -n rds-instance-01 -f Outputs
```

You'll see the Terraform outputs in JSON format similar to this:

```json
{
  "rds_db_name": {
    "sensitive": true,
    "type": "string",
    "value": "wordpress"
  },
  "rds_hostname": {
    "sensitive": true,
    "type": "string",
    "value": "wordpress-db.ccccmqr3ixkh.us-east-2.rds.amazonaws.com"
  },
  "rds_password": {
    "sensitive": true,
    "type": "string",
    "value": "unsecurepwd"
  },
  "rds_port": {
    "sensitive": true,
    "type": "number",
    "value": 3306
  },
  "rds_username": {
    "sensitive": true,
    "type": "string",
    "value": "wordpress_user"
  }
}
```

You can now use these values as inputs to a workload to use this database.

## Delete Terraform Instance

The following command will delete the database instance:

```bash
tptctl delete terraform-instance -n rds-instance-01
```

The command will not return a response until the AWS resources have been
removed.  For an RDS instance this will take a few minutes.

Once the resources are removed and the prompt returns, you can delete the
Terraform definition as well.

```bash
tptctl delete terraform-definition -n rds-instance
```

## Clean Up

Before we finish, let's clean up the files we downloaded to your file system.

```bash
cd ../
rm -rf threeport-terraform-test
```

## Summary

In this guide you used a Terraform definition and instance to deploy and delete
an AWS RDS instance.  You also learned how to get the outputs from Terraform to
provide DB connection info to a client application.

Therein lies one of the
challenges with Terraform: programmatically providing outputs from Terraform as
inputs to another operation to deploy a workload that connects to that DB.  In
the near future, we will provide guides on how to use the [Threeport
SDK](../sdk/sdk-intro.md) to wire these concerns together to remove human
copy-paste operations from the process.

