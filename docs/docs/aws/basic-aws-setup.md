# Basic AWS Setup

Use this documentation to configure Threeport to manage resources within the same AWS account.  If you need to manage workloads in a different AWS account from the one Threeport is deployed in, follow the [Advanced AWS Setup guide](../aws/advanced-aws-setup.md)

To get started, construct a config with the required fields. Here is an example of what this config looks like:

```yaml
AwsAccount:
  Name: default-account
  AccountID: "555555555555"
  DefaultAccount: true

  # option 1: provide explicit configs/credentials
  #DefaultRegion: some-region
  #AccessKeyID: "ASDF"
  #SecretAccessKey: "asdf"

  # option 2: use local AWS configs/credentials
  LocalConfig: /path/to/local/.aws/config
  LocalCredentials: /path/to/local/.aws/credentials
  LocalProfile: default
```

Paste the following command to download `aws-account.yaml`. Open the file and update `AccountID`,
`LocalConfig`, and `LocalCredentials` to the appropriate values for your environment.

```bash
curl -O https://raw.githubusercontent.com/threeport/releases/main/samples/aws-account.yaml
```

Once `aws-account.yaml` is prepared, run the following command to create the `AwsAccount`
object in the Threeport API:
```bash
tptctl create aws-account --config aws-account.yaml
```