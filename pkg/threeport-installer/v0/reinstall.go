package v0

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
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

// A reinstall deletes installer-managed resources that are not labeled
// persistent, then re-runs the install path. Emptying the database or
// message broker is issued against the running services so their volumes
// survive, and is refused unless the namespace records a development tier.

var deploymentGVR = schema.GroupVersionResource{
	Group:    "apps",
	Version:  "v1",
	Resource: "deployments",
}

// deleteTarget is a Kubernetes resource kind to list and delete during reinstall.
// Cluster-scoped kinds are listed without a namespace.
type deleteTarget struct {
	gvr        schema.GroupVersionResource
	namespaced bool
}

// deleteTargets is the installer-managed resource kinds reinstall deletes.
// CRDs, namespaces, volume claims, and stateful sets are omitted so
// custom resources, the control plane namespace, and stored data survive.
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

// restApiDeploymentReadyTimeout is the time allowed for the rest-api
// deployment to report a ready replica after reinstall.
const restApiDeploymentReadyTimeout = 5 * time.Minute

const dropDatabaseJobName = "threeport-drop-database"

// dropDatabaseJobBackoffLimit is the number of pod retries before the
// database drop job is treated as failed.
const dropDatabaseJobBackoffLimit = 3

const dropMessageBrokerJobName = "threeport-drop-message-broker"

// dropMessageBrokerJobBackoffLimit is the number of pod retries before the
// message broker drop job is treated as failed.
const dropMessageBrokerJobBackoffLimit = 3

// dbRootCertsMountPath is the directory the cockroach client reads for certificates.
// The secret's keys must use the filenames that client expects.
const dbRootCertsMountPath = "/cockroach/cockroach-certs"

// dbRootCertsFileMode is the owner-only permission the cockroach client requires on key files.
const dbRootCertsFileMode = 0600

var jobGVR = schema.GroupVersionResource{
	Group:    "batch",
	Version:  "v1",
	Resource: "jobs",
}

// ErrControlPlaneNotDevelopment is returned when a drop is refused because the
// control plane is not installed at the development tier, or its tier is unknown.
var ErrControlPlaneNotDevelopment = errors.New("control plane is not installed at the development tier")

// DropDatabase empties the control plane database on a development installation.
// The drop is issued as SQL against the running database so its volume and certificates survive.
func (cpi *ControlPlaneInstaller) DropDatabase(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	namespace := cpi.Opts.Namespace

	// read the installed control plane tier
	tier, err := cpi.getInstalledTier(kubeClient, mapper)
	if err != nil {
		return err
	}
	// refuse a non-development control plane
	if tier != ControlPlaneTierDev {
		return fmt.Errorf(
			"%w: namespace %s reports tier %q, refusing to drop its database",
			ErrControlPlaneNotDevelopment, namespace, tier,
		)
	}

	// scale the control plane to zero so nothing is mid-write during the drop
	if err := cpi.scaleDownDeployments(kubeClient, namespace); err != nil {
		return fmt.Errorf("failed to scale control plane down before the drop: %w", err)
	}

	// run the database drop job
	if err := cpi.runDropDatabaseJob(kubeClient, namespace); err != nil {
		return err
	}

	fmt.Println("Info: database dropped, it will be recreated empty by the install that follows")

	return nil
}

// DropMessageBrokerState removes every stream on a development installation's message broker.
// Key-value buckets are streams too, so one pass also clears reconciliation locks.
func (cpi *ControlPlaneInstaller) DropMessageBrokerState(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	namespace := cpi.Opts.Namespace

	// read the installed control plane tier
	tier, err := cpi.getInstalledTier(kubeClient, mapper)
	if err != nil {
		return err
	}
	// refuse a non-development control plane
	if tier != ControlPlaneTierDev {
		return fmt.Errorf(
			"%w: namespace %s reports tier %q, refusing to drop its message broker state",
			ErrControlPlaneNotDevelopment, namespace, tier,
		)
	}

	// run the message broker drop job
	if err := cpi.runDropMessageBrokerJob(kubeClient, namespace); err != nil {
		return err
	}

	fmt.Println("Info: message broker streams dropped, they will be recreated by the install that follows")

	return nil
}

// runDropMessageBrokerJob removes every nats stream from a one-off job, waits, then deletes it.
func (cpi *ControlPlaneInstaller) runDropMessageBrokerJob(
	kubeClient dynamic.Interface,
	namespace string,
) error {
	// clear a leftover drop job whose pod template is immutable
	if err := cpi.deleteDropJob(
		kubeClient, namespace, dropMessageBrokerJobName, "message broker",
	); err != nil {
		return err
	}

	// remove every nats stream including key-value buckets
	script := "set -e; for stream in $(nats stream ls --names); do nats stream rm \"$stream\" --force; done"
	fmt.Println("Info: removing every message broker stream and key-value bucket")

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

	// create the drop job
	if _, err := kubeClient.Resource(jobGVR).Namespace(namespace).Create(
		context.Background(), job, metav1.CreateOptions{},
	); err != nil {
		return fmt.Errorf("failed to create message broker drop job %s: %w", dropMessageBrokerJobName, err)
	}

	// wait for the drop job to complete
	if err := cpi.waitForDropJob(
		kubeClient, namespace, dropMessageBrokerJobName, dropMessageBrokerJobBackoffLimit, "message broker",
	); err != nil {
		return err
	}

	// delete the job after it succeeds
	return cpi.deleteDropJob(kubeClient, namespace, dropMessageBrokerJobName, "message broker")
}

// runDropDatabaseJob issues the drop from a one-off job, waits, then deletes it.
func (cpi *ControlPlaneInstaller) runDropDatabaseJob(
	kubeClient dynamic.Interface,
	namespace string,
) error {
	// clear a leftover drop job whose pod template is immutable
	if err := cpi.deleteDropJob(kubeClient, namespace, dropDatabaseJobName, "database"); err != nil {
		return err
	}

	// cancel paused schema changes before the drop
	statement := fmt.Sprintf(
		"CANCEL JOBS (SELECT job_id FROM [SHOW JOBS] WHERE status = 'paused' AND job_type = 'NEW SCHEMA CHANGE'); DROP DATABASE IF EXISTS %s CASCADE",
		database.ThreeportDatabaseName,
	)
	fmt.Printf("Info: running %q against the database\n", statement)

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

	// create the drop job
	if _, err := kubeClient.Resource(jobGVR).Namespace(namespace).Create(
		context.Background(), job, metav1.CreateOptions{},
	); err != nil {
		return fmt.Errorf("failed to create database drop job %s: %w", dropDatabaseJobName, err)
	}

	// wait for the drop job to complete
	if err := cpi.waitForDropJob(
		kubeClient, namespace, dropDatabaseJobName, dropDatabaseJobBackoffLimit, "database",
	); err != nil {
		return err
	}

	// delete the job after it succeeds
	return cpi.deleteDropJob(kubeClient, namespace, dropDatabaseJobName, "database")
}

// waitForDropJob polls a drop job until a pod succeeds or the job exceeds its backoff.
// Exhausted retries are reported immediately because the job will not recover.
func (cpi *ControlPlaneInstaller) waitForDropJob(
	kubeClient dynamic.Interface,
	namespace string,
	jobName string,
	backoffLimit int64,
	subject string,
) error {
	var jobFailed error

	// poll until the job succeeds or exhausts its retries
	if err := util.Retry(60, 3, func() error {
		job, err := kubeClient.Resource(jobGVR).Namespace(namespace).Get(
			context.Background(), jobName, metav1.GetOptions{},
		)
		if err != nil {
			return fmt.Errorf("failed to read %s drop job status: %w", subject, err)
		}

		// stop polling a job that has exhausted its retries
		failed, _, _ := util.NestedInt64OrFloat64(job.Object, "status", "failed")
		if failed > backoffLimit {
			jobFailed = fmt.Errorf(
				"%s drop job %s failed after %d pod attempt(s): inspect its pod logs in namespace %s",
				subject, jobName, failed, namespace,
			)
			return nil
		}

		// succeed once a pod has completed
		succeeded, _, _ := util.NestedInt64OrFloat64(job.Object, "status", "succeeded")
		if succeeded > 0 {
			return nil
		}

		return fmt.Errorf("%s drop job %s has not completed", subject, jobName)
	}); err != nil {
		return fmt.Errorf("%s drop did not complete: %w", subject, err)
	}

	return jobFailed
}

// deleteDropJob deletes a drop job in the foreground and waits until its name is free.
func (cpi *ControlPlaneInstaller) deleteDropJob(
	kubeClient dynamic.Interface,
	namespace string,
	jobName string,
	subject string,
) error {
	deletePolicy := metav1.DeletePropagationForeground
	deleteOpts := metav1.DeleteOptions{PropagationPolicy: &deletePolicy}

	// delete the job and its pods in the foreground
	if err := kubeClient.Resource(jobGVR).Namespace(namespace).Delete(
		context.Background(), jobName, deleteOpts,
	); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete %s drop job %s: %w", subject, jobName, err)
	}

	// wait until the job name is gone so it can be reused
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

// getInstalledTier returns the control plane tier recorded on the namespace.
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

	return ControlPlaneTier(tier), nil
}

// Reinstall deletes installer-managed stateless resources and re-runs the install path.
// Resources labeled persistent keep their data and the api endpoint across the reinstall.
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

	fmt.Println("Info: installing threeport control plane dependencies (nats, crdb, encryption-key, api load balancer)")
	// install control plane dependencies
	if err := cpi.InstallThreeportControlPlaneDependencies(kubeClient, mapper, "", nil); err != nil {
		return fmt.Errorf("failed to install control plane dependencies: %w", err)
	}

	fmt.Println("Info: installing threeport api deployment")
	// install the api deployment
	if err := cpi.UpdateThreeportAPIDeployment(kubeClient, mapper, nil); err != nil {
		return fmt.Errorf("failed to install threeport api deployment: %w", err)
	}

	fmt.Printf("Info: installing %d threeport controller(s)\n", len(cpi.Opts.ControllerList))
	// install controllers
	if err := cpi.InstallThreeportControllers(kubeClient, mapper, authConfig); err != nil {
		return fmt.Errorf("failed to install threeport controllers: %w", err)
	}

	fmt.Println("Info: installing threeport agent")
	// install the agent
	if err := cpi.InstallThreeportAgent(kubeClient, mapper, authConfig); err != nil {
		return fmt.Errorf("failed to install threeport agent: %w", err)
	}

	fmt.Printf("Info: waiting for rest-api deployment to become ready (timeout %s)\n", restApiDeploymentReadyTimeout)
	// wait for the rest-api to become ready
	if err := cpi.waitForRestAPIReady(kubeClient, ns, restApiDeploymentReadyTimeout); err != nil {
		return fmt.Errorf("rest-api did not become ready after reinstall: %w", err)
	}

	return nil
}

// deleteForReinstall deletes installer-managed resources that are not labeled persistent.
// Deleting and recreating covers spec fields that cannot be patched on a running deployment.
func (cpi *ControlPlaneInstaller) deleteForReinstall(
	kubeClient dynamic.Interface,
	namespace string,
) error {
	// select installer-managed resources that are not marked persistent
	selector := fmt.Sprintf(
		"%s=%s,%s!=%s",
		LabelManagedBy, LabelManagedByValue,
		LabelPersistent, LabelPersistentValue,
	)

	// scale deployments to zero so the control plane stops driving state
	if err := cpi.scaleDownDeployments(kubeClient, namespace); err != nil {
		return err
	}

	fmt.Println("Info: deleting installer-managed stateless resources across deployments, configmaps, secrets, services, serviceaccounts, roles, rolebindings, clusterroles, clusterrolebindings")
	deletePolicy := metav1.DeletePropagationForeground
	deleteOpts := metav1.DeleteOptions{PropagationPolicy: &deletePolicy}

	// delete matching resources in the foreground so dependents are gone before install
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

	// wait until every matching resource has left the api
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
			if n := len(list.Items); n > 0 {
				pending += n
				if sample == "" {
					sample = fmt.Sprintf("%s/%s", target.gvr.Resource, list.Items[0].GetName())
				}
			}
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

// scaleDownDeployments sets installer-managed non-persistent deployments to zero replicas.
// The control plane must stop driving state before resources are deleted or the schema is dropped.
func (cpi *ControlPlaneInstaller) scaleDownDeployments(
	kubeClient dynamic.Interface,
	namespace string,
) error {
	// select installer-managed deployments that are not marked persistent
	selector := fmt.Sprintf(
		"%s=%s,%s!=%s",
		LabelManagedBy, LabelManagedByValue,
		LabelPersistent, LabelPersistentValue,
	)

	fmt.Println("Info: scaling all control plane deployments to 0 and waiting for pods to terminate")
	// list installer-managed non-persistent deployments
	deployList, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: selector},
	)
	if err != nil {
		return fmt.Errorf("failed to list control plane deployments: %w", err)
	}
	// scale each matching deployment to zero
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

// waitForRestAPIReady polls the rest-api deployment until it reports a ready replica.
func (cpi *ControlPlaneInstaller) waitForRestAPIReady(
	kubeClient dynamic.Interface,
	namespace string,
	timeout time.Duration,
) error {
	name := cpi.Opts.RestApiInfo.ServiceResourceName
	attemptsMax := int(timeout / (3 * time.Second))
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
		return fmt.Errorf("rest-api deployment not yet ready")
	})
}

// LoadAuthConfigFromCluster reconstructs the control plane CA from the api-ca secret.
// The returned config signs new certificates without rotating that CA.
func (cpi *ControlPlaneInstaller) LoadAuthConfigFromCluster(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) (*auth.AuthConfig, error) {
	// load the api-ca secret
	secret, err := kube.GetResource(
		"", "v1", "Secret",
		cpi.Opts.Namespace, ThreeportApiCaSecret,
		kubeClient, *mapper,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load api-ca secret: %w", err)
	}

	caB64, _, err := unstructured.NestedString(secret.Object, "data", "tls.crt")
	if err != nil || caB64 == "" {
		return nil, fmt.Errorf("api-ca secret missing data.tls.crt")
	}
	keyB64, _, err := unstructured.NestedString(secret.Object, "data", "tls.key")
	if err != nil || keyB64 == "" {
		return nil, fmt.Errorf("api-ca secret missing data.tls.key")
	}

	// decode the base64-encoded kubernetes secret data
	caPem, err := base64.StdEncoding.DecodeString(caB64)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode ca cert: %w", err)
	}
	keyPem, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode ca key: %w", err)
	}

	// parse the ca certificate
	caBlock, _ := pem.Decode(caPem)
	if caBlock == nil {
		return nil, fmt.Errorf("ca cert pem block missing")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ca cert: %w", err)
	}

	// parse the pkcs1 ca private key
	keyBlock, _ := pem.Decode(keyPem)
	if keyBlock == nil {
		return nil, fmt.Errorf("ca key pem block missing")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ca key: %w", err)
	}

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
