# Namespaces

When deploying workloads, Threeport can manage Kubernetes namespaces for you.
This is the recommended approach.

## Prerequisites

For this guide you will need a Threeport control plane installed.  Follow the
[Install Threeport Locally guide](../install/install-threeport-local.md) to install a local
control plane.

## Unmanaged Namespaces

If you do not want Threeport to manage Kubernetes namespaces for you, you will
need to include the Namespace resource in the workload definition's
`YAMLDocument` that provides the manifest of Kubernetes resources.

To demonstrate, create a work space on your local file system.

```bash
mkdir threeport-test
cd threeport-test
```

Create a very simple Kubernetes manifest to deploy a pod into a namespace.

```bash
cat <<EOF > unmanaged-nginx-manifest.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: test-nginx
---
apiVersion: v1
kind: Pod
metadata:
  name: nginx
  namespace: test-nginx
spec:
  containers:
  - name: nginx
    image: nginx:1.14.2
    ports:
    - containerPort: 80
EOF
```

This Kubernetes manifest includes a Namespace resource and the Pod resource must
have its `metadata.namespace` set to the same namespace.  This is an example of
the user managing the namespace, not Threeport.

Next we'll need workload configs for Threeport.  Let's create the
WorkloadDefinition.

```bash
cat <<EOF > unmanaged-nginx-workload-definition.yaml
WorkloadDefinition:
  Name: unmanaged-nginx
  YAMLDocument: unmanaged-nginx-manifest.yaml
EOF
```

And a WorkloadInstance.

```bash
cat <<EOF > unmanaged-nginx-workload-instance-0.yaml
WorkloadInstance:
  Name: unmanaged-nginx-0
  WorkloadDefinition:
    Name: unmanaged-nginx
EOF
```

Now let's create those workload resources.

```bash
tptctl create workload-definition -c unmanaged-nginx-workload-definition.yaml
tptctl create workload-instance -c unmanaged-nginx-workload-instance-0.yaml
```

We can now see the objects we created in Threeport.

```bash
tptctl get workloads
```

You should see the following output.

```bash
NAME                 WORKLOAD DEFINITION     WORKLOAD INSTANCE      KUBERNETES RUNTIME INSTANCE     STATUS       AGE
unmanaged-nginx      unmanaged-nginx         unmanaged-nginx-0      threeport-dev                   Healthy      42s
```

If you have [kubectl](https://kubernetes.io/docs/tasks/tools/) installed, you
can see the pod resource in Kubernetes as well.

```bash
kubectl get po -n test-nginx
```

Now let's attempt to create a second instance of this workload.  We'll create a
second workload instance that references the same workload definition.

```bash
cat <<EOF > unmanaged-nginx-workload-instance-1.yaml
WorkloadInstance:
  Name: unmanaged-nginx-1
  WorkloadDefinition:
    Name: unmanaged-nginx
EOF
```

When you create the workload instance you will get an error.

```bash
tptctl create workload-instance -c unmanaged-nginx-workload-instance-1.yaml
```

This is because the workload definition for this instance contains a namespace.
Another namespace with the same name cannot be created in Kubernetes so a new,
distinct workload using this manifest is impossible.

In order to create multiple workload instances in a Kubernetes runtime from a
single definition, use managed namespaces in Threeport.

## Managed Namespaces

The recommended approach is to use managed namespaces in Threeport.

To demonstrate, create a very simple Kubernetes manifest to deploy a pod.

```bash
cat <<EOF > managed-nginx-manifest.yaml
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  containers:
  - name: nginx
    image: nginx:1.14.2
    ports:
    - containerPort: 80
EOF
```

This Kubernetes manifest includes only the Pod resource without any reference to
a namespace.  In this case, Threeport will manage the namespace for you so you
don't need to include the Namespace resource.

Next we'll need workload configs for Threeport.  Let's create the
WorkloadDefinition.

```bash
cat <<EOF > managed-nginx-workload-definition.yaml
WorkloadDefinition:
  Name: managed-nginx
  YAMLDocument: managed-nginx-manifest.yaml
EOF
```

And a WorkloadInstance.

```bash
cat <<EOF > managed-nginx-workload-instance-0.yaml
WorkloadInstance:
  Name: managed-nginx-0
  WorkloadDefinition:
    Name: managed-nginx
EOF
```

Now let's create those workload resources.

```bash
tptctl create workload-definition -c managed-nginx-workload-definition.yaml
tptctl create workload-instance -c managed-nginx-workload-instance-0.yaml
```

List the Threeport workloads.

```bash
tptctl get workloads
```

You should see the following output.

```bash
NAME                 WORKLOAD DEFINITION     WORKLOAD INSTANCE      KUBERNETES RUNTIME INSTANCE     STATUS       AGE
unmanaged-nginx      unmanaged-nginx         unmanaged-nginx-0      threeport-dev-0                 Healthy      2h36m34s
managed-nginx        managed-nginx           managed-nginx-0        threeport-dev-0                 Healthy      1m22s
```

If you have [kubectl](https://kubernetes.io/docs/tasks/tools/) installed, you
can query the namespaces and see a new namespace has been created.

```bash
kubectl get ns
```

There will be a new namespace called something like
`managed-nginx-0-4g0i0kshyu`.  Yours will be slightly different because
Threeport puts a random suffix on the namespace name.  This namespace is where
the nginx pod is running.

Now we can create a second instance of this workload.  We'll create a
second workload instance that references the same workload definition.

```bash
cat <<EOF > managed-nginx-workload-instance-1.yaml
WorkloadInstance:
  Name: managed-nginx-1
  WorkloadDefinition:
    Name: managed-nginx
EOF
```

And you can now successfully create a second instance.

```bash
tptctl create workload-instance -c managed-nginx-workload-instance-1.yaml
```

Now you can list Threeport workloads again.

```bash
tptctl get workloads
```

You should see the following:

```bash
NAME                 WORKLOAD DEFINITION     WORKLOAD INSTANCE      KUBERNETES RUNTIME INSTANCE     STATUS       AGE
unmanaged-nginx      unmanaged-nginx         unmanaged-nginx-0      threeport-dev-0                 Healthy      2h45m35s
managed-nginx        managed-nginx           managed-nginx-0        threeport-dev-0                 Healthy      10m22s
managed-nginx        managed-nginx           managed-nginx-1        threeport-dev-0                 Healthy      2m58s
```

Notice there are two instances of the `managed-nginx` workload derived from the
same workload definition.

You can also re-check the namespaces in your cluster.

```bash
kubectl get ns | grep nginx
```

You should see results similar to this:

```bash
managed-nginx-0-4g0i0kshyu   Active   8m5s
managed-nginx-1-5nuxe87le3   Active   42s
test-nginx                   Active   163m
```

In this way, using managed namespaces, you are free to deploy as many workload
instances to a Kubernetes cluster from a common workload definition as you wish.
When using unmanaged namespaces you are limited to one workload instance per
workload definition in a single Kubernetes runtime instance.

## Summary

In this guide you have seen how to use managed namespaces in Threeport and the
utility they provide in allowing you to deploy as many workload instances from a
single workload definition to a single Kubernetes runtime instance as you like.

Clean up.

```bash
cd ../
rm -rf threeport-test
```
