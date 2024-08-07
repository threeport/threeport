# Deploy Workload on AWS

In this guide, we're going to deploy a sample WordPress app and use Threeport to
manage several dependencies for it:

* Network ingress routing
* TLS termination
* DNS record using AWS Route53
* Managed database using AWS RDS
* Managed object storage using AWS S3
* Managed secrets using AWS Secrets Manager

## Prerequisites

You'll need a Threeport control plane for this guide.  You have two options:

1. Install a [Local Threeport](../install/install-threeport-local.md) instance and
   then provision a [Remote Kubernetes
   Runtime](../kubernetes-runtime/remote-kubernetes-runtime.md) for your workload.
1. Install a [Remote Threeport](../install/install-threeport-aws.md) instance
   on AWS and use the Kubernetes instance that is used to host Threeport to deploy
   your workload.

## Download Sample Configs

First, create a work space on your local file system:

```bash
mkdir threeport-test
cd threeport-test
```

Download a sample workload config as follows:

```bash
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/workload/wordpress-workload-remote.yaml
```

You now have the workload config on your local file system.  If you open the file you'll
see it has the following contents:

```yaml
Workload:
  Name: "wordpress"
  YAMLDocument: "wordpress-manifest-remote.yaml"
  KubernetesRuntimeInstance:
    Name: eks-k8s-runtime
  AwsRelationalDatabase:
    Name: wordpress-db
    AwsAccountName: default-account
    Engine: mariadb
    EngineVersion: "10.11"
    DatabaseName: wordpress
    DatabasePort: 3306
    StorageGb: 20
    MachineSize: XSmall
    WorkloadSecretName: wordpress-db-conn
  AwsObjectStorageBucket:
    Name: s3-client-bucket
    AwsAccountName: default-account
    PublicReadAccess: false
    WorkloadServiceAccountName: s3-client
    WorkloadBucketEnvVar: S3_BUCKET_NAME
  DomainName:
    Name: example-domain
    Domain: example.com
    Zone: Public
    AdminEmail: admin@example.com
  Secret:
    Name: wordpress-secret
    AwsAccountName: default-account
    Data:
      WORDPRESS_PASSWORD: admin_password
      WORDPRESS_SMTP_PASSWORD: smtp_password
  Gateway:
    Name: web-service-gateway
    HttpPorts:
      - Port: 80
        HTTPSRedirect: true
        Path: "/"
      - Port: 443
        TLSEnabled: true
        Path: "/"
    ServiceName: getting-started-wordpress
    SubDomain: blog
```

### Name Configuration

The `Name` field is an arbitrary, user-defined name that must be unique, i.e. no
other workload may use the same name.

### YAMLDocument Configuration

The `YAMLDocument` field refers to another file with the Kubernetes resource
manifests.  Download that file as well:

```bash
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/workload/wordpress-manifest-remote.yaml
```

### Kubernetes Runtime Configuration

Set name of the Kubernetes runtime you wish to use.  You can use `tptctl get
kubernetes-runtime-instances` to see which runtimes are available.

```yaml
  KubernetesRuntimeInstance:
    Name: eks-k8s-runtime        # <-- set this value
```

You can also remove this config to simply use the default runtime.

### AwsRelationalDatabase Configuration

The `AwsRelationalDatabase` field includes the specification for an AWS RDS
instance that will be used for the WordPress database.  Threeport will spin up
that RDS instance for the sample app and connect it.  Also, when you delete your
app, Threeport will clean up the RDS instance as well.

The most important thing to note in the `AwsRelationalDatabase` config is the
`WorkloadSecretName`.

```yaml
  AwsRelationalDatabase:
    Name: wordpress-db-0
    AwsAccountName: default-account
    Engine: mariadb
    EngineVersion: "10.11"
    DatabaseName: wordpress
    DatabasePort: 3306
    StorageGb: 20
    MachineSize: XSmall
    WorkloadSecretName: wordpress-db-conn  # <-- note this value
```

The value for this field tells Threeport what name to give to the Kubernetes
secret that provides the database connection credentials to the WordPress app.
In the `wordpress-manifest-remote.yaml` file is the following snippet.

```yaml
          env:
            - name: BITNAMI_DEBUG
              value: "false"
            - name: ALLOW_EMPTY_PASSWORD
              value: "yes"
            - name: MARIADB_HOST
              valueFrom:
                secretKeyRef:
                  name: wordpress-db-conn       ## <-- secret name reference
                  key: db-endpoint              ## <-- value key
            - name: MARIADB_PORT_NUMBER
              valueFrom:
                secretKeyRef:
                  name: wordpress-db-conn       ## <-- secret name reference
                  key: db-port                  ## <-- value key
            - name: WORDPRESS_DATABASE_NAME
              valueFrom:
                secretKeyRef:
                  name: wordpress-db-conn       ## <-- secret name reference
                  key: db-name                  ## <-- value key
            - name: WORDPRESS_DATABASE_USER
              valueFrom:
                secretKeyRef:
                  name: wordpress-db-conn       ## <-- secret name reference
                  key: db-user                  ## <-- value key
            - name: WORDPRESS_DATABASE_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: wordpress-db-conn       ## <-- secret name reference
                  key: db-password              ## <-- value key
```

WordPress uses specific environment variables to retrieve database connection
info.  For different applications, the values from the `wordpress-db-conn`
secret simply need to be mapped to the appropriate environment variables for
that app.  Be sure to set the value keys shown - these are set by Threeport in
the secret it creates and never change.

### AwsObjectStorageBucket Configuration

The `AwsObjectStorageBucket` field provides the configuration for the S3 bucket
to be used by the application.  Note the `WorkloadServiceAccountName` and
`WorkloadBucketEnvVar` values.

```yaml
  AwsObjectStorageBucket:
    Name: s3-client-bucket
    AwsAccountName: default-account
    PublicReadAccess: false
    WorkloadServiceAccountName: s3-client   ## <-- note this value
    WorkloadBucketEnvVar: S3_BUCKET_NAME    ## <-- note this value
```

The `WorkloadServiceAccountName` refers to the name of a service account that
must be present in the `YAMLDocument`.  The service account used for this example
is included in this snippet from the `wordpress-manifest-remote.yaml` file.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: s3-client       ## <-- service account name
```

If a service account with the same name referenced by the
`WorkloadServiceAccountName` field does not exist, the workload will not be able
to connect to the S3 bucket.

The `WorkloadBucketEnvVar` is the name of the environment variable that will be
provided to the workload to get the bucket name to connect to.  Set the value to
the environment variable your application will use.

### DomainName Configuration

The `DomainName` field provides a config for managing a DNS record for the
sample app.  This will currently only work if you are managing a domain with Route53 in
AWS with a Hosted Zone.  If you aren't using Route53, comment out the entire
`DomainName` section and we'll use an AWS load balancer endpoint to connect to
the sample app.  If you have a hosted zone in Route53 to use, make the following
updates.

```yaml
  DomainName:
    Name: example-domain
    Domain: example.com				# <-- set your Route53 hosted zone here
    Zone: Public
    AdminEmail: admin@example.com   # <-- put your email address here
```

### Secret Configuration

The `Secret` field provides config for managing secret values for the sample
app.

> Note: AWS Secrets Manager is currently the only supported provider for secrets
> management and will be used by default by your Threeport control plane.

Threeport handles the integration between secrets and Kubernetes manifests in a
similar manner to AWS RDS connection credentials. A secret is created in the
same namespace as the sample app, and the manifests must configure the secret
values as needed. Below is an example `Secret` configuration:

```yaml
  Secret:
    Name: wordpress-secret # <-- note this value
    AwsAccountName: default-account
    Data:
      WORDPRESS_PASSWORD: admin_password     # <-- secret key and value
      WORDPRESS_SMTP_PASSWORD: smtp_password # <-- secret key and value
```

The `Name` field in an arbitrary name provided by the user.

> Important: The `Name` value determines what the secret will be called in AWS
> Secrets Manager.  If the provided `Name` value is already in use, Threeport
> will not be able to create it.  Furthermore, AWS Secrets Manager reserves the
> name for a restoration grace period (minimum of 7 days).  So if you delete a
> secret and then attempt to re-create it with the same name, it will fail if
> that grace period has not expired.

`wordpress-manifest-remote.yaml` contains the following snippet, which shows how
secret values map into the sample app's Kubernetes manifest.


```yaml
          env:
          ...
            - name: WORDPRESS_PASSWORD         # <-- environment variable expected by app
              valueFrom:
                secretKeyRef:
                  name: wordpress-secret
                  key: WORDPRESS_PASSWORD      # <-- secret data key
            - name: WORDPRESS_SMTP_PASSWORD    # <-- environment variable expected by app
              valueFrom:
                secretKeyRef:
                  name: wordpress-secret
                  key: WORDPRESS_SMTP_PASSWORD # <-- secret data key
          ...
```

Below is an example of the Kubernetes secret that is inserted into the app's namespace by
the Threeport Control Plane. This object is managed on behalf of the user by
Threeport and provided here for illustration purposes only. The keys within
the `data` field are what must be referenced by the user-supplied manifest, as
illustrated above.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: wordpress-secret
type: Opaque
data:
  WORDPRESS_PASSWORD: YWRtaW5fcGFzc3dvcmQ=      # encoded value of "admin_password"
  WORDPRESS_SMTP_PASSWORD: c210cF9wYXNzd29yZA== # encoded value of "smtp_password"
```

### Gateway Configuration

The `Gateway` field includes a config to set up ingress to our sample app from
the public internet and terminate TLS.  The `SubDomain` field here will result
in a record for `blog.example.com` being added to the `example.com` Route53
hosted zone.

```yaml
  Gateway:
    HttpPorts:
      - Port: 80
        HTTPSRedirect: true
        Path: "/"
      - Port: 443
        TLSEnabled: true
        Path: "/"
    ServiceName: getting-started-wordpress
    SubDomain: blog                         # <-- set your desired subdomain
```

### Create Workload

Once you have made the necessary changes to the workload config, we can create
the workload as follows:

```bash
tptctl create workload --config wordpress-workload-remote.yaml
```

Threeport will now do the following:

* Install the WordPress app.
* Spin up an RDS database for your app.
* Create a new S3 bucket for your app and provide workload identity access to
  your app.
* If you specified a `DomainName` config, Threeport will install
  [external-dns](https://github.com/kubernetes-sigs/external-dns) on your EKS
  cluster and instruct it to configure Route53.
* Install [Gloo Edge](https://github.com/solo-io/gloo) for network ingress
  control and configure it for your app.
* Install [cert-manager](https://github.com/cert-manager/cert-manager) to
  provision and rotate TLS certificates for the sample app.
  Note: the [Let's Encrypt](https://letsencrypt.org/) staging environment will
  be used for this guide.  This means the certificate issued will not be
  publicly trusted - you will have to tell your browser to trust it.  When the
  production environment is used, it will be publicly trusted.

## Validate

It will take a few minutes for AWS to spin up the RDS database instance.
You can check the RDS console in AWS to track its progress if you like.
Shortly after the database is up, Threeport will create a secret to provide
the database connection credentials to the sample app and it will begin running.

### Kubernetes Resources

If you have [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl) and the
[AWS CLI](https://aws.amazon.com/cli/) installed, you can check the progress of
the app as follows.

Update your kubeconfig:

```bash
aws eks update-kubeconfig --name threeport-test
```

Then, view the pods in the remote Kubernetes cluster:

```bash
kubectl get pods -A
```

### Visit WordPress App

Once your WordPress app pods show Status: Running, your app is ready to visit.

Remember, you will need to tell your browser to trust the connection as we're
using the Let's Encrypt staging environment.

In your browser, visit: `https://www.example.com`.  Change the domain to the one you used,
or replace it with the AWS load balancer endpoint which can be found in the AWS
EC2 console.

### S3 Bucket

The sample WordPress app in this guide does not actually use the S3 bucket.
However we can validate that it works as follows.

Threeport has taken care of access to the S3 bucket using IAM roles for service
accounts (IRSA) and provided the environment variable to retrieve the bucket
name.  So we can just connect to the container and test it out.

First, get the managed namespace for the app.

```bash
export WORDPRESS_NAMESPACE=$(kubectl get ns -l "control-plane.threeport.io/managed-by=threeport" -o=jsonpath='{.items[0].metadata.name}')
```

Next get the pod name.

```bash
export WORDPRESS_POD=$(kubectl get po -n $WORDPRESS_NAMESPACE -o=jsonpath='{.items[0].metadata.name}')
```

Now we can connect to the S3 client sidecar container.

```bash
kubectl exec -it -n $WORDPRESS_NAMESPACE $WORDPRESS_POD -c s3-client -- bash
```

You should now have a shell inside that container.  Next, create a file to
transfer to S3.

```bash
echo "test file content" > testing.txt
```

Now we can transfer the file to S3.  This container has the aws CLI tool
installed.

```bash
aws s3 cp testing.txt s3://$S3_BUCKET_NAME
```

You can now verify in the S3 AWS console that the file has been transferred
successfully.  At this point you can delete the file from S3 to ensure the
bucket can be cleaned up.  (AWS will not remove a bucket that has objects in
it.)

Now exit the container.

```bash
exit
```

## Clean Up

### WordPress Workload

Threeport will not delete a Kubernetes cluster with workload instances running
by default.  This prevents inadvertently deleting apps that need to continue
running.

View the workload instances with:

```bash
tptctl get workload-instances
```

Delete the WordPress workload instance.

```bash
tptctl delete workload-instance -n wordpress
```

### Support Service Workloads

If you used a `DomainName` config, ensure your DNS records have been removed (it
can take a minute or two for external-dns to clean those up), then delete the
gloo-edge and external-dns workloads.

Delete the Gloo Edge, external-dns and external-secrets workload instances.

```bash
tptctl delete workload-instance -n gloo-edge-threeport-test # name may differ
tptctl delete workload-instance -n external-dns-threeport-test # name may differ
tptctl delete workload-instance -n external-secrets-threeport-test  # name may difer
```

### Secret Definition

To delete the secret from AWS Secrets Manager, first check the name of the
secret definition:

```bash
tptctl get secret-definitions
```

You should see something like this in the output:

```bash
NAME                  AGE
wordpress-secret      24m19s
```

Then delete it by name to remove it from AWS Secrets Manager:

```bash
tptctl delete secret-definition -n wordpress-secret
```

### Threeport Control Plane

Uninstall Threeport:

Give Threeport a few minutes to clean up your AWS resources, then remove the
control plane.  If you delete the control plane before it has finished removing
the gloo-edge service resource, you will be left with a dangling AWS load
balancer which will prevent tearing down all of the AWS infra.

```bash
tptctl down -n test
```

Remove the test configs from you file system:

```bash
cd ../
rm -rf threeport-test
```

## Next Steps

Now that you've tried out Threeport with a sample WordPress workload, we suggest
you try it out with one of your workloads.  Get in touch via
[Discord](https://discord.com/invite/Fwr2sc9Dfp) if you have any questions.

Also, check out the [Managed Threeport](../managed-threeport.md) offerings to
fast-track getting Threeport into use at your organization.

