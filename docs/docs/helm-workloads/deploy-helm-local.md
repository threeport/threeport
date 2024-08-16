# Deploy Helm Workload Locally

In this guide, we're going to use a Helm workload to deploy a sample app locally
using Threeport.

First we'll create a definition and some instances from that definition.  After
that, we'll deploy a Helm workload using the defined instance abstraction which
creates a definition and instance in a single step.  If you prefer, jump
straight to the [Helm Workload Defined Instance
section](#helm-workload-defined-instance) for the quick intro.  If you would
like a clearer understanding of how the definitions and instances work for Helm
workloads, run through this document from beginning to end.

See our [Definitions & Instances concepts document](
../concepts/definitions-instances.md) for more details about definitions and
instances in Threeport.

## Prerequisites

You'll need a local Threeport control plane for this guide.  Follow the [Install
Threeport Locally guide](../install/install-threeport-local.md) to set that up.

## Create Helm Workload Definition

First, create a work space on your local file system:

```bash
mkdir threeport-helm-test
cd threeport-helm-test
```

Download a sample Helm workload config and values file as follows:

```bash
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/helm/wordpress-helm-workload-definition.yaml
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/helm/wordpress-helm-workload-definition-values.yaml
```

The Helm workload config looks as follows:

```yaml
HelmWorkloadDefinition:
  Name: wordpress
  Repo: "oci://registry-1.docker.io/bitnamicharts"
  Chart: wordpress
  ValuesDocument: wordpress-helm-workload-definition-values.yaml
```

This definition specifies Bitnami charts repo and the Helm WordPress chart to be
used by all instances derived from this definition.  It also references the
values file that you download that has an override for the default number of
replicas in that upstream Helm chart:

> Note: At this time, Helm charts must be hosted in a Helm repo to be used in
> Threeport.

```yaml
replicaCount: 2
```

This means that, unless otherwise specified on the instance, 2 replicas of the
WordPress app will be created.

We can now create the workload as follows:

```bash
tptctl create helm-workload-definition --config wordpress-helm-workload-definition.yaml
```

This command calls the Threeport API to create the HelmWorkload object.
No workloads are deployed at this time.  We've just defined the chart and
default values to be used by instances.  Next, we'll create some running
instances from this definition.

## Create Helm Workload Instances

First let's create a instance a default instance.  In this case we're just using
the Helm chart with the default values `replicaCount: 2`.  All other values are
inherited from the upstream defaults.

Download the instance config:

```bash
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/helm/wordpress-helm-workload-instance-default.yaml
```

This config simply references the definition and adds no runtime parameters
(values that provide config at runtime).

```yaml
HelmWorkloadInstance:
  Name: wordpress-default
  HelmWorkloadDefinition:
    Name: wordpress
```

Create the default Helm workload instance:

```bash
tptctl create helm-workload-instance --config wordpress-helm-workload-instance-default.yaml
```

Now you can view the Helm workload you have running.

```bash
tptctl get helm-workloads
```

Your output should look similar to this:

```bash
NAME           HELM WORKLOAD DEFINITION     HELM WORKLOAD INSTANCE     KUBERNETES RUNTIME INSTANCE     STATUS       AGE
wordpress      wordpress                    wordpress-default          threeport-dev-0                 Healthy      1m3s
```

If you have kubectl installed you can view the pods in your cluster:

```bash
kubectl get pods -A -l control-plane.threeport.io/managed-by=threeport
```

You should see output similar to this:

```bash
NAMESPACE                      NAME                                        READY   STATUS    RESTARTS      AGE
wordpress-default-vpmmjfosws   wordpress-default-release-d699bdb6b-9zt4d   1/1     Running   1 (67s ago)   3m49s
wordpress-default-vpmmjfosws   wordpress-default-release-d699bdb6b-fckqp   1/1     Running   0             3m49s
wordpress-default-vpmmjfosws   wordpress-default-release-mariadb-0         1/1     Running   0             3m49s
```

As you can see, there are two replicas of the WordPress app and one instance of
its database.

Now let's create another Helm workload instance from our definition.  In this
case we'll simulate a dev instance that has some Helm values as runtime
parameters.  Download the config and values file:

```bash
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/helm/wordpress-helm-workload-instance-dev.yaml
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/helm/wordpress-helm-workload-instance-dev-values.yaml
```

The config for the dev instance looks similar to the default instance deployed
above but references the values file for the runtime parameters.

```yaml
HelmWorkloadInstance:
  Name: wordpress-dev
  ValuesDocument: wordpress-helm-workload-instance-dev-values.yaml
  HelmWorkloadDefinition:
    Name: wordpress
```

The values file specifies a label to identify the tier of the app.

```yaml
commonLabels:
  tier: "dev"
```

We can now create the new instance.

```bash
tptctl create helm-workload-instance --config wordpress-helm-workload-instance-dev.yaml
```

Now, if we get the Helm workload instances from the system we can see both
instances derived from the same definition.

```bash
tptctl get helm-workloads
```

Your output should look similar to this:

```bash
NAME           HELM WORKLOAD DEFINITION     HELM WORKLOAD INSTANCE     KUBERNETES RUNTIME INSTANCE     STATUS       AGE
wordpress      wordpress                    wordpress-default          threeport-dev-0                 Healthy      15m12s
wordpress      wordpress                    wordpress-dev              threeport-dev-0                 Healthy      1m5s
```

If we view Threeport-managed pods with kubectl again we can see pods for both
instances of WordPress.

```bash
kubectl get pods -A -l control-plane.threeport.io/managed-by=threeport
```

Your output should look similar to this:

```bash
NAMESPACE                      NAME                                        READY   STATUS    RESTARTS        AGE
wordpress-default-vpmmjfosws   wordpress-default-release-d699bdb6b-9zt4d   1/1     Running   1 (14m ago)     17m
wordpress-default-vpmmjfosws   wordpress-default-release-d699bdb6b-fckqp   1/1     Running   0               17m
wordpress-default-vpmmjfosws   wordpress-default-release-mariadb-0         1/1     Running   0               17m
wordpress-dev-xsiqnjpkgd       wordpress-dev-release-d5df57df-5c8bx        1/1     Running   1 (2m34s ago)   3m14s
wordpress-dev-xsiqnjpkgd       wordpress-dev-release-d5df57df-hzz46        1/1     Running   0               3m14s
wordpress-dev-xsiqnjpkgd       wordpress-dev-release-mariadb-0             1/1     Running   0               3m14s
```

One thing you'll notice is that each time a new instance is created, it gets its
own [namespace](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) which allows you to create as many instances in the same
Kubernetes runtime as you need.

Now, let's create one more instance for prod.  In this case we'll apply a
different label and also override the Helm values provided in the definition.

Download the config and values file:

```bash
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/helm/wordpress-helm-workload-instance-prod.yaml
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/helm/wordpress-helm-workload-instance-prod-values.yaml
```

The instance config references the same definition and points to the new prod
values file:

```yaml
HelmWorkloadInstance:
  Name: wordpress-prod
  ValuesDocument: wordpress-helm-workload-instance-prod-values.yaml
  HelmWorkloadDefinition:
    Name: wordpress
```

The values file has a its own labels and overrides the replicas:

```yaml
replicaCount: 4
commonLabels:
  tier: "prod"
```

We can now deploy the prod instance:

```bash
tptctl create helm-workload-instance --config wordpress-helm-workload-instance-prod.yaml
```

Now, when we view Helm workloads we can see all 3 instances derived from the
same definition:

```bash
tptctl get helm-workloads
```

Your output should look similar to this:

```bash
NAME           HELM WORKLOAD DEFINITION     HELM WORKLOAD INSTANCE     KUBERNETES RUNTIME INSTANCE     STATUS       AGE
wordpress      wordpress                    wordpress-default          threeport-dev-0                 Healthy      22m49s
wordpress      wordpress                    wordpress-dev              threeport-dev-0                 Healthy      8m42s
wordpress      wordpress                    wordpress-prod             threeport-dev-0                 Healthy      38s
```

And, again, we can see the pods from the new deployment using kubectl.

```bash
kubectl get pods -A -l control-plane.threeport.io/managed-by=threeport
```

Your output should look similar to this:

```bash
NAMESPACE                      NAME                                        READY   STATUS    RESTARTS        AGE
wordpress-default-vpmmjfosws   wordpress-default-release-d699bdb6b-9zt4d   1/1     Running   1 (21m ago)     24m
wordpress-default-vpmmjfosws   wordpress-default-release-d699bdb6b-fckqp   1/1     Running   0               24m
wordpress-default-vpmmjfosws   wordpress-default-release-mariadb-0         1/1     Running   0               24m
wordpress-dev-xsiqnjpkgd       wordpress-dev-release-d5df57df-5c8bx        1/1     Running   1 (9m23s ago)   10m
wordpress-dev-xsiqnjpkgd       wordpress-dev-release-d5df57df-hzz46        1/1     Running   0               10m
wordpress-dev-xsiqnjpkgd       wordpress-dev-release-mariadb-0             1/1     Running   0               10m
wordpress-prod-fqgiwndiur      wordpress-prod-release-766467d4c-9qrv9      1/1     Running   1 (72s ago)     2m
wordpress-prod-fqgiwndiur      wordpress-prod-release-766467d4c-n594w      1/1     Running   1 (72s ago)     2m
wordpress-prod-fqgiwndiur      wordpress-prod-release-766467d4c-q9wp7      1/1     Running   0               2m
wordpress-prod-fqgiwndiur      wordpress-prod-release-766467d4c-st2qn      1/1     Running   1 (72s ago)     2m
wordpress-prod-fqgiwndiur      wordpress-prod-release-mariadb-0            1/1     Running   0               2m
```

As you can see, due to the runtime parameters for the prod instance specifying 4
replicas, there are 4 pods for the WordPress app.

Before we move on, let's clean up the Helm workloads we've deployed so far.

```bash
tptctl delete helm-workload-instance -n wordpress-prod
tptctl delete helm-workload-instance -n wordpress-dev
tptctl delete helm-workload-instance -n wordpress-default
tptctl delete helm-workload-definition -n wordpress
```

## Helm Workload Defined Instance

If you would like to create a definition and instance in one step, you can do
that too.  Download sample config.

```bash
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/helm/wordpress-helm-workload.yaml
```

This config includes the definition info, default Helm values for the definition
as well as Helm values for the instance.  It is referencing Helm values
documents we previously downloaded.

```yaml
HelmWorkload:
  Name: wordpress-dev
  Repo: "oci://registry-1.docker.io/bitnamicharts"
  Chart: wordpress
  DefinitionValuesDocument: wordpress-helm-workload-definition-values.yaml
  InstanceValuesDocument: wordpress-helm-workload-instance-dev-values.yaml
```

If you haven't already, download the values documents referenced in the config.

```bash
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/helm/wordpress-helm-workload-definition-values.yaml
curl -O https://raw.githubusercontent.com/threeport/threeport/main/samples/helm/wordpress-helm-workload-instance-dev-values.yaml
```

Now we can create a Helm workload definition and instance with one command:

```bash
tptctl create helm-workload --config wordpress-helm-workload.yaml
```

If we get Helm workloads we can see we now have a definition with an instance
already derived from it.

```bash
tptctl get helm-workloads
```

Your output should look similar to this:

```bash
NAME               HELM WORKLOAD DEFINITION     HELM WORKLOAD INSTANCE     KUBERNETES RUNTIME INSTANCE     STATUS       AGE
wordpress-dev      wordpress-dev                wordpress-dev              threeport-dev-0                 Healthy      54s
```

We can also delete both with a single step as well.

```bash
tptctl delete helm-workload --config wordpress-helm-workload.yaml
```

## Clean Up

Before we finish, let's clean up the files we downloaded to your file system.

```bash
cd ../
rm -rf threeport-helm-test
```

## Summary

In this guide we demonstrated how to use Helm charts that are hosted on a Helm
repo to install Helm workloads.  We created definitions and instances separately
and also created a definition and instance with one step using the defined
instance abstraction.

