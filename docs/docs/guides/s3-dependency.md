# Workload With AWS S3 Dependency

This guide walks through deploying a workload with a dependency on an AWS S3
bucket.

## Prerequisites

This guide assumes you have a remote Kubernetes runtime instance provisioned
using the eks provider.  See [Remote Kubernetes
Runtime](../kubernetes-runtime/remote-kubernetes-runtime.md) guide
for instructions.

## Download Configs

Download the Kubernetes manifest for a simple containerized workload that has
the AWS CLI installed.

TODO

Download the Threeport config for the workload.

TODO

## Deploy

Deploy the workload to the remote runtime:

```bash
tptctl create workload -c s3-client-workload.yaml
```

## Test

Now let's ensure our S3 client workload has access to create and delete objects
on that S3 bucket.

First let's get our workload's pod name and namespace.

```bash
POD_NAMESPACE=$(kubectl get ns -l app=aws-client -o=jsonpath='{@.items[0].metadata.name}')
POD_NAME=$(kubectl get po -n $POD_NAMESPACE -l app=aws-client -o=jsonpath='{@.items[0].metadata.name}')
```

Get bash session in the running container.

```bash
kubectl exec -it -n $POD_NAMESPACE $POD_NAME -- bash
```

Inside the container, create a text file to save to S3.

```bash
echo "testing s3" > test.txt
```

Copy that file to our S3 bucket.

```bash
aws s3 cp test.txt s3://$S3_BUCKET_NAME/test.txt
```

Remove the local file, re-sync with the S3 bucket and confirm the contents of
the received file.

```bash
rm test.txt
aws s3 sync s3://$S3_BUCKET_NAME ./
cat test.txt
```

Disonnect from the container.

```bash
exit
```

## Clean Up

Remove the workload instance.

```bash
tptctl delete workload-instance -n aws-client
```

