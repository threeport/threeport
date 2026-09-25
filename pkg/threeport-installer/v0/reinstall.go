package v0

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/pkg/api-server/v0/database"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// A reinstall deletes installer-managed objects that are not marked persistent,
// then installs the control plane again in the same namespace. Emptying the
// database or the message broker is issued against the running service so its
// volume survives. A drop is refused unless the namespace tier is development.

// deploymentGVR is the apps/v1 deployments resource used to scale and to read status.
var deploymentGVR = schema.GroupVersionResource{
	Group:    "apps",
	Version:  "v1",
	Resource: "deployments",
}

// deleteTarget is one Kubernetes kind removed on reinstall.
// A cluster-scoped kind is listed without a namespace.
type deleteTarget struct {
	// The group, version, and resource to list and delete
	gvr schema.GroupVersionResource
	// Whether the resource is listed inside the control plane namespace
	namespaced bool
}

// deleteTargets are the stateless kinds removed on reinstall. CRDs, the
// namespace, volume claims, and stateful sets are omitted so custom resources,
// the namespace, and stored data survive.
var deleteTargets = []deleteTarget{
	{gvr: deploymentGVR, namespaced: true},
	{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, namespaced: true},
	{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, namespaced: true},
	{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, namespaced: true},
	{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}, namespaced: true},
	{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}, namespaced: true},
	{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}, namespaced: true},
	{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}, namespaced: false},
	{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}, namespaced: false},
}

// restApiDeploymentReadyTimeout is how long to wait for one ready REST API replica.
const restApiDeploymentReadyTimeout = 5 * time.Minute

// dropDatabaseJobName is the Job that runs the database drop statement.
const dropDatabaseJobName = "threeport-drop-database"

// dropDatabaseJobBackoffLimit is how many failed pods end the database drop wait.
const dropDatabaseJobBackoffLimit = 3

// dropMessageBrokerJobName is the Job that removes message broker streams.
const dropMessageBrokerJobName = "threeport-drop-message-broker"

// dropMessageBrokerJobBackoffLimit is how many failed pods end the stream drop wait.
const dropMessageBrokerJobBackoffLimit = 3

// dbRootCertsMountPath is where the database client mounts its certificates.
// The secret keys must use the filenames that client expects.
const dbRootCertsMountPath = "/cockroach/cockroach-certs"

// dbRootCertsFileMode is an owner-only mode the cockroach client accepts on key files.
const dbRootCertsFileMode = 0600

// jobGVR is the batch/v1 jobs resource used to run a drop.
var jobGVR = schema.GroupVersionResource{
	Group:    "batch",
	Version:  "v1",
	Resource: "jobs",
}

// ErrControlPlaneNotDevelopment is returned when a drop is asked for on a
// control plane that is not labeled as a development installation.
var ErrControlPlaneNotDevelopment = errors.New("control plane is not installed at the development tier")

// DropDatabase empties the control plane database on a development installation.
// The drop is SQL against the running database so its volume and certificates survive.
func (cpi *ControlPlaneInstaller) DropDatabase(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	namespace := cpi.Opts.Namespace

	// refuse anything that is not the development tier before the drop
	if err := cpi.RequireDevelopmentTier(kubeClient, mapper); err != nil {
		return err
	}

	// scale control plane deployments to zero
	if err := cpi.scaleDownDeployments(kubeClient, namespace); err != nil {
		return fmt.Errorf("failed to scale control plane down before the drop: %w", err)
	}

	// run the database drop job
	if err := cpi.runDropDatabaseJob(kubeClient, namespace); err != nil {
		return err
	}

	// report that the following install recreates an empty database
	fmt.Println("Info: database dropped, it will be recreated empty by the install that follows")

	return nil
}

// DropMessageBrokerState removes every message broker stream on a development
// installation. Key-value buckets are streams, so the pass also clears reconciliation locks.
func (cpi *ControlPlaneInstaller) DropMessageBrokerState(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	namespace := cpi.Opts.Namespace

	// refuse anything that is not the development tier before the drop
	if err := cpi.RequireDevelopmentTier(kubeClient, mapper); err != nil {
		return err
	}

	// run the stream removal job
	if err := cpi.runDropMessageBrokerJob(kubeClient, namespace); err != nil {
		return err
	}

	// report that the following install recreates the streams
	fmt.Println("Info: message broker streams dropped, they will be recreated by the install that follows")

	return nil
}

// runDropMessageBrokerJob runs a one-shot Job that deletes every stream.
func (cpi *ControlPlaneInstaller) runDropMessageBrokerJob(
	kubeClient dynamic.Interface,
	namespace string,
) error {
	// delete a leftover job because its pod template is immutable
	if err := cpi.deleteDropJob(
		kubeClient, namespace, dropMessageBrokerJobName, "message broker",
	); err != nil {
		return err
	}

	// remove every stream, which also removes key-value buckets stored as streams
	script := "set -e; for stream in $(nats stream ls --names); do nats stream rm \"$stream\" --force; done"
	fmt.Println("Info: removing every message broker stream and key-value bucket")

	// create the message broker drop job
	job := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name":      dropMessageBrokerJobName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"backoffLimit": int64(dropMessageBrokerJobBackoffLimit),
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"restartPolicy": "Never",
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "drop-message-broker",
								"image": natsBoxImage,
								"env": []interface{}{
									map[string]interface{}{
										"name":  "NATS_URL",
										"value": natsServiceName,
									},
								},
								"command": []interface{}{"/bin/sh", "-c", script},
							},
						},
					},
				},
			},
		},
	}

	// submit the job
	if _, err := kubeClient.Resource(jobGVR).Namespace(namespace).Create(
		context.Background(), job, metav1.CreateOptions{},
	); err != nil {
		return fmt.Errorf("failed to create message broker drop job %s: %w", dropMessageBrokerJobName, err)
	}

	// wait for the job to finish
	if err := cpi.waitForDropJob(
		kubeClient, namespace, dropMessageBrokerJobName, dropMessageBrokerJobBackoffLimit, "message broker",
	); err != nil {
		return err
	}

	// delete the finished job
	return cpi.deleteDropJob(kubeClient, namespace, dropMessageBrokerJobName, "message broker")
}

// runDropDatabaseJob runs a one-shot Job that drops the control plane database.
func (cpi *ControlPlaneInstaller) runDropDatabaseJob(
	kubeClient dynamic.Interface,
	namespace string,
) error {
	// delete a leftover job because its pod template is immutable
	if err := cpi.deleteDropJob(kubeClient, namespace, dropDatabaseJobName, "database"); err != nil {
		return err
	}

	// cancel paused new-schema-change jobs, then drop the database
	statement := fmt.Sprintf(
		"CANCEL JOBS (SELECT job_id FROM [SHOW JOBS] WHERE status = 'paused' AND job_type = 'NEW SCHEMA CHANGE'); DROP DATABASE IF EXISTS %s CASCADE",
		database.ThreeportDatabaseName,
	)
	fmt.Printf("Info: running %q against the database\n", statement)

	// create the database drop job
	job := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name":      dropDatabaseJobName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"backoffLimit": int64(dropDatabaseJobBackoffLimit),
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"restartPolicy": "Never",
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "drop-database",
								"image": fmt.Sprintf("cockroachdb/cockroach:%s", DatabaseImageTag),
								"command": []interface{}{
									"/cockroach/cockroach",
									"sql",
									fmt.Sprintf("--certs-dir=%s", dbRootCertsMountPath),
									fmt.Sprintf("--host=%s", database.ThreeportDatabaseHost),
									fmt.Sprintf("--port=%s", database.ThreeportDatabasePort),
									"--execute",
									statement,
								},
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      dbRootCertSecretName,
										"mountPath": dbRootCertsMountPath,
									},
								},
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": dbRootCertSecretName,
								"secret": map[string]interface{}{
									"secretName":  dbRootCertSecretName,
									"defaultMode": int64(dbRootCertsFileMode),
								},
							},
						},
					},
				},
			},
		},
	}

	// submit the job
	if _, err := kubeClient.Resource(jobGVR).Namespace(namespace).Create(
		context.Background(), job, metav1.CreateOptions{},
	); err != nil {
		return fmt.Errorf("failed to create database drop job %s: %w", dropDatabaseJobName, err)
	}

	// wait for the job to finish
	if err := cpi.waitForDropJob(
		kubeClient, namespace, dropDatabaseJobName, dropDatabaseJobBackoffLimit, "database",
	); err != nil {
		return err
	}

	// delete the finished job
	return cpi.deleteDropJob(kubeClient, namespace, dropDatabaseJobName, "database")
}

// waitForDropJob polls until the Job succeeds or its failed-pod count passes
// the backoff limit. Exhausted retries are returned at once because the job will not recover.
func (cpi *ControlPlaneInstaller) waitForDropJob(
	kubeClient dynamic.Interface,
	namespace string,
	jobName string,
	backoffLimit int64,
	subject string,
) error {
	var jobFailed error

	// poll until the job succeeds or 60 reads fail
	if err := util.Retry(60, 3, func() error {
		// read the job status
		job, err := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
			context.Background(), jobName, metav1.GetOptions{},
		)
		if err != nil {
			return fmt.Errorf("failed to read %s drop job status: %w", subject, err)
		}

		// stop polling once failed pods pass the backoff limit
		failed, _, _ := util.NestedInt64OrFloat64(job.Object, "status", "failed")
		if failed > backoffLimit {
			jobFailed = fmt.Errorf(
				"%s drop job %s failed after %d pod attempt(s): inspect its pod logs in namespace %s",
				subject, jobName, failed, namespace,
			)
			return nil
		}

		// stop polling once a pod has succeeded
		succeeded, _, _ := util.NestedInt64OrFloat64(job.Object, "status", "succeeded")
		if succeeded > 0 {
			return nil
		}

		// keep polling while the job is still running
		return fmt.Errorf("%s drop job %s has not completed", subject, jobName)
	}); err != nil {
		return fmt.Errorf("%s drop did not complete: %w", subject, err)
	}

	// return a recorded job failure instead of a retry timeout
	return jobFailed
}

// deleteDropJob removes a drop Job and waits until the object is gone so the
// same name can be created again.
func (cpi *ControlPlaneInstaller) deleteDropJob(
	kubeClient dynamic.Interface,
	namespace string,
	jobName string,
	subject string,
) error {
	// delete the job and its pods in the foreground
	deletePolicy := metav1.DeletePropagationForeground
	deleteOpts := metav1.DeleteOptions{PropagationPolicy: &deletePolicy}

	if err := kubeClient.Resource(jobGVR).Namespace(namespace).Delete(
		context.Background(), jobName, deleteOpts,
	); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete %s drop job %s: %w", subject, jobName, err)
	}

	// wait until the job object is gone
	if err := util.Retry(20, 3, func() error {
		_, err := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
			context.Background(), jobName, metav1.GetOptions{},
		)
		if err == nil {
			return fmt.Errorf("%s drop job %s still terminating", subject, jobName)
		}
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to check %s drop job %s: %w", subject, jobName, err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("%s drop job did not finish terminating: %w", subject, err)
	}

	return nil
}

// RequireDevelopmentTier reads threeport.io/tier on the control plane namespace.
// A drop proceeds only when that label is development. Any other value, including
// a missing label, is refused before stored state is changed.
func (cpi *ControlPlaneInstaller) RequireDevelopmentTier(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	// read the installed tier
	tier, err := cpi.getInstalledTier(kubeClient, mapper)
	if err != nil {
		return err
	}

	// refuse a control plane that is not development
	if tier != ControlPlaneTierDev {
		return fmt.Errorf(
			"%w: namespace %s reports tier %q",
			ErrControlPlaneNotDevelopment, cpi.Opts.Namespace, tier,
		)
	}

	return nil
}

// getInstalledTier reads the tier label on the control plane namespace.
// An absent tier is an error so a drop never proceeds on a missing value.
func (cpi *ControlPlaneInstaller) getInstalledTier(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) (ControlPlaneTier, error) {
	// read the control plane namespace
	namespace, err := kube.GetResource(
		"", "v1", "Namespace",
		"", cpi.Opts.Namespace,
		kubeClient, *mapper,
	)
	if err != nil {
		return "", fmt.Errorf("failed to read control plane namespace %s: %w", cpi.Opts.Namespace, err)
	}

	// refuse a namespace that records no tier
	tier := namespace.GetLabels()[LabelTier]
	if tier == "" {
		return "", fmt.Errorf(
			"%w: namespace %s records no tier, so it cannot be confirmed as a development installation",
			ErrControlPlaneNotDevelopment, cpi.Opts.Namespace,
		)
	}

	// return the recorded tier
	return ControlPlaneTier(tier), nil
}

// Reinstall deletes installer-managed stateless resources and installs the
// control plane again. Persistent resources keep their data and the API endpoint.
func (cpi *ControlPlaneInstaller) Reinstall(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	authConfig *auth.AuthConfig,
) error {
	ns := cpi.Opts.Namespace

	// delete installer-managed stateless resources
	if err := cpi.deleteForReinstall(kubeClient, ns); err != nil {
		return fmt.Errorf("failed to delete control plane resources: %w", err)
	}

	// set create-or-update so install can reapply resources that still exist
	cpi.Opts.CreateOrUpdateKubeResources = true

	// install control plane dependencies
	fmt.Println("Info: installing threeport control plane dependencies (nats, crdb, encryption-key, api load balancer)")
	if err := cpi.InstallThreeportControlPlaneDependencies(kubeClient, mapper, "", nil); err != nil {
		return fmt.Errorf("failed to install control plane dependencies: %w", err)
	}

	// install the api deployment
	fmt.Println("Info: installing threeport api deployment")
	if err := cpi.UpdateThreeportAPIDeployment(kubeClient, mapper, nil); err != nil {
		return fmt.Errorf("failed to install threeport api deployment: %w", err)
	}

	// install the controllers
	fmt.Printf("Info: installing %d threeport controller(s)\n", len(cpi.Opts.ControllerList))
	if err := cpi.InstallThreeportControllers(kubeClient, mapper, authConfig); err != nil {
		return fmt.Errorf("failed to install threeport controllers: %w", err)
	}

	// install the agent
	fmt.Println("Info: installing threeport agent")
	if err := cpi.InstallThreeportAgent(kubeClient, mapper, authConfig); err != nil {
		return fmt.Errorf("failed to install threeport agent: %w", err)
	}

	// wait for the rest api deployment to become ready
	fmt.Printf("Info: waiting for rest-api deployment to become ready (timeout %s)\n", restApiDeploymentReadyTimeout)
	if err := cpi.waitForRestAPIReady(kubeClient, ns, restApiDeploymentReadyTimeout); err != nil {
		return fmt.Errorf("rest-api did not become ready after reinstall: %w", err)
	}

	return nil
}

// deleteForReinstall removes installer-managed objects that are not marked
// persistent. Recreating them covers spec fields that cannot be patched.
func (cpi *ControlPlaneInstaller) deleteForReinstall(
	kubeClient dynamic.Interface,
	namespace string,
) error {
	// select installer-managed resources that are not persistent
	selector := fmt.Sprintf(
		"%s=%s,%s!=%s",
		LabelManagedBy, LabelManagedByValue,
		LabelPersistent, LabelPersistentValue,
	)

	// scale deployments to zero before deleting them
	if err := cpi.scaleDownDeployments(kubeClient, namespace); err != nil {
		return err
	}

	// delete matching resources in the foreground so dependents are gone before install
	fmt.Println("Info: deleting installer-managed stateless resources across deployments, configmaps, secrets, services, serviceaccounts, roles, rolebindings, clusterroles, clusterrolebindings")
	deletePolicy := metav1.DeletePropagationForeground
	deleteOpts := metav1.DeleteOptions{PropagationPolicy: &deletePolicy}

	controlPlaneNamespaces, err := controlPlaneNamespaceNames(kubeClient)
	if err != nil {
		return err
	}

	count := 0
	for _, target := range deleteTargets {
		var ri dynamic.ResourceInterface
		if target.namespaced {
			ri = kubeClient.Resource(target.gvr).Namespace(namespace)
		} else {
			ri = kubeClient.Resource(target.gvr)
		}

		list, err := ri.List(context.Background(), metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return fmt.Errorf(
				"failed to list %s matching %q: %w",
				target.gvr.Resource, selector, err,
			)
		}

		for _, obj := range list.Items {
			name := obj.GetName()
			// a longer sibling namespace owns the binding when its name is the longer prefix
			if !target.namespaced && !clusterBindingFor(name, namespace, controlPlaneNamespaces) {
				continue
			}
			if err := ri.Delete(context.Background(), name, deleteOpts); err != nil && !k8serrors.IsNotFound(err) {
				return fmt.Errorf(
					"failed to delete %s/%s: %w",
					target.gvr.Resource, name, err,
				)
			}
			count++
		}
	}
	fmt.Printf("Info: deleted %d stateless resource(s)\n", count)

	// wait until every selected resource is gone
	var sample string
	if err := util.Retry(60, 3, func() error {
		pending := 0
		sample = ""
		for _, target := range deleteTargets {
			var ri dynamic.ResourceInterface
			if target.namespaced {
				ri = kubeClient.Resource(target.gvr).Namespace(namespace)
			} else {
				ri = kubeClient.Resource(target.gvr)
			}
			list, err := ri.List(context.Background(), metav1.ListOptions{LabelSelector: selector})
			if err != nil {
				return fmt.Errorf("failed to list %s while waiting for delete: %w", target.gvr.Resource, err)
			}
			owned := 0
			for _, obj := range list.Items {
				if !target.namespaced && !clusterBindingFor(obj.GetName(), namespace, controlPlaneNamespaces) {
					continue
				}
				owned++
				if sample == "" {
					sample = fmt.Sprintf("%s/%s", target.gvr.Resource, obj.GetName())
				}
			}
			pending += owned
		}
		if pending > 0 {
			return fmt.Errorf("%d resource(s) still terminating (e.g. %s)", pending, sample)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("control plane resources did not finish terminating: %w", err)
	}

	return nil
}

// clusterBindingFor reports whether a cluster role or binding belongs to this namespace.
// A longer sibling namespace wins when the name starts with that sibling too.
func clusterBindingFor(name, namespace string, namespaces []string) bool {
	if !strings.HasPrefix(name, namespace+"-") {
		return false
	}
	if !strings.HasSuffix(name, "-threeportworkloads") && !strings.HasSuffix(name, "-cluster-admin") {
		return false
	}
	for _, other := range namespaces {
		if other != namespace && len(other) > len(namespace) && strings.HasPrefix(name, other+"-") {
			return false
		}
	}
	return true
}

// controlPlaneNamespaceNames lists namespaces the installer manages.
func controlPlaneNamespaceNames(kubeClient dynamic.Interface) ([]string, error) {
	list, err := kubeClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).List(
		context.Background(),
		metav1.ListOptions{LabelSelector: LabelManagedBy + "=" + LabelManagedByValue},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list control plane namespaces: %w", err)
	}
	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}
	return names, nil
}

// scaleDownDeployments sets non-persistent installer-managed deployments to
// zero replicas. The control plane must stop driving state before a delete or a schema drop.
func (cpi *ControlPlaneInstaller) scaleDownDeployments(
	kubeClient dynamic.Interface,
	namespace string,
) error {
	// select installer-managed deployments that are not persistent
	selector := fmt.Sprintf(
		"%s=%s,%s!=%s",
		LabelManagedBy, LabelManagedByValue,
		LabelPersistent, LabelPersistentValue,
	)

	// list those deployments
	fmt.Println("Info: scaling all control plane deployments to 0 and waiting for pods to terminate")
	deployList, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: selector},
	)
	if err != nil {
		return fmt.Errorf("failed to list control plane deployments: %w", err)
	}
	// patch each deployment to zero replicas
	patch := []byte(`{"spec":{"replicas":0}}`)
	for _, dep := range deployList.Items {
		name := dep.GetName()
		_, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Patch(
			context.Background(),
			name,
			"application/strategic-merge-patch+json",
			patch,
			metav1.PatchOptions{},
		)
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to scale deployment %s to zero: %w", name, err)
		}
	}

	// wait until no ready replicas remain
	if err := util.Retry(60, 3, func() error {
		current, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).List(
			context.Background(), metav1.ListOptions{LabelSelector: selector},
		)
		if err != nil {
			return fmt.Errorf("failed to list deployments while waiting for scale-down: %w", err)
		}
		pending := 0
		for _, dep := range current.Items {
			ready, _, _ := util.NestedInt64OrFloat64(dep.Object, "status", "readyReplicas")
			if ready > 0 {
				pending += int(ready)
			}
		}
		if pending > 0 {
			return fmt.Errorf("%d ready replica(s) still present", pending)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("control plane deployments did not scale to zero: %w", err)
	}

	return nil
}

// waitForRestAPIReady polls until the REST API deployment has one ready
// replica. A deployment that is not there yet keeps the wait going.
func (cpi *ControlPlaneInstaller) waitForRestAPIReady(
	kubeClient dynamic.Interface,
	namespace string,
	timeout time.Duration,
) error {
	name := cpi.Opts.RestApiInfo.ServiceResourceName
	attemptsMax := int(timeout / (3 * time.Second))

	// poll until one replica is ready
	return util.Retry(attemptsMax, 3, func() error {
		dep, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Get(
			context.Background(), name, metav1.GetOptions{},
		)
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to read rest-api deployment status: %w", err)
		}
		if err == nil {
			ready, _, _ := util.NestedInt64OrFloat64(dep.Object, "status", "readyReplicas")
			if ready >= 1 {
				return nil
			}
		}
		return errors.New("rest-api deployment must be ready")
	})
}

// LoadAuthConfigFromCluster rebuilds the API certificate authority from the
// api-ca secret. The returned config signs new certificates without rotating that CA.
func (cpi *ControlPlaneInstaller) LoadAuthConfigFromCluster(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) (*auth.AuthConfig, error) {
	// read the api ca secret
	secret, err := kube.GetResource(
		"", "v1", "Secret",
		cpi.Opts.Namespace, ThreeportApiCaSecret,
		kubeClient, *mapper,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load api-ca secret: %w", err)
	}

	// read the cert and key fields
	caB64, _, err := unstructured.NestedString(secret.Object, "data", "tls.crt")
	if err != nil || caB64 == "" {
		return nil, errors.New("tls.crt not found in api-ca secret")
	}
	keyB64, _, err := unstructured.NestedString(secret.Object, "data", "tls.key")
	if err != nil || keyB64 == "" {
		return nil, errors.New("tls.key not found in api-ca secret")
	}

	// decode the cert and key
	caPem, err := base64.StdEncoding.DecodeString(caB64)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode ca cert: %w", err)
	}
	keyPem, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode ca key: %w", err)
	}

	// parse the certificate
	caBlock, _ := pem.Decode(caPem)
	if caBlock == nil {
		return nil, errors.New("ca cert pem block not found")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ca cert: %w", err)
	}

	// parse the private key
	keyBlock, _ := pem.Decode(keyPem)
	if keyBlock == nil {
		return nil, errors.New("ca key pem block not found")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ca key: %w", err)
	}

	// return the auth config with raw and base64 forms
	return &auth.AuthConfig{
		CAConfig:                  caCert,
		CAPrivateKey:              *caKey,
		CA:                        caBlock.Bytes,
		CAPemEncoded:              string(caPem),
		CABase64Encoded:           util.Base64Encode(string(caPem)),
		CAPrivateKeyPemEncoded:    string(keyPem),
		CAPrivateKeyBase64Encoded: util.Base64Encode(string(keyPem)),
	}, nil
}

// DetectAuthEnabled reports whether the installed API was started with auth.
// A missing deployment or an unreadable arg list is reported as enabled.
func DetectAuthEnabled(kubeClient dynamic.Interface, namespace string) bool {
	// read the api deployment and treat a lookup failure as auth enabled
	deploy, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).Get(
		context.Background(),
		ThreeportAPIServiceResourceName,
		metav1.GetOptions{},
	)
	if err != nil {
		return true
	}

	// read the first container and treat a missing spec as auth enabled
	containers, found, err := unstructured.NestedSlice(deploy.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		return true
	}

	container, ok := containers[0].(map[string]interface{})
	if !ok {
		return true
	}
	args, found, err := unstructured.NestedStringSlice(container, "args")
	if err != nil || !found {
		return true
	}

	// return false only for an explicit -auth-enabled=false argument
	for _, arg := range args {
		if arg == "-auth-enabled=false" {
			return false
		}
	}

	return true
}
