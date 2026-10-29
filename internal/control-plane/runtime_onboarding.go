package controlplane

import (
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// apiTarget is a threeport API that records are read from or written to.
type apiTarget struct {
	client   *http.Client
	endpoint string
}

// runtimeOnboarder copies kubernetes runtime records from the control plane
// that is bootstrapping a child to the child's own API. Every copy is a
// getOrCreate, as the rest of onboarding is, so a reconcile that is retried
// recognizes what an earlier attempt already put there.
type runtimeOnboarder struct {
	// parent is the control plane running the reconciler.
	parent apiTarget

	// child is the control plane being onboarded.
	child apiTarget

	// encryptionKey is the parent's key. Secrets are stored encrypted with it,
	// and the child has a key of its own, so they have to be decrypted before
	// they are submitted: the child's API encrypts on write, and ciphertext
	// handed to it would be encrypted a second time and unreadable to anything
	// holding only the child's key.
	encryptionKey string
}

// runtimeLink identifies the generic kubernetes runtime definition and
// instance that provider-specific records hang off.
type runtimeLink struct {
	definitionID uint
	instanceID   uint
}

// decryptRuntimeInstanceSecrets replaces the stored secrets on a runtime
// instance with their plaintext, for submission to an API that has a different
// encryption key.
func decryptRuntimeInstanceSecrets(instance *v0.KubernetesRuntimeInstance, encryptionKey string) error {
	if err := decryptField(encryptionKey, &instance.CertificateKey, "kubernetes runtime instance certificate key"); err != nil {
		return err
	}

	return decryptField(encryptionKey, &instance.ConnectionToken, "kubernetes runtime instance connection token")
}

// createRuntimeInstance creates a copy of a kubernetes runtime instance on the
// child, under the child's copy of its definition.
//
// Whether the runtime hosts the control plane and whether it is the default are
// decided here rather than carried over, because they are statements about the
// child: the runtime the child's own components run on is its host and its
// default, and the parent's runtime is neither once it is only a registered
// cluster. Carrying the parent's flags over would make the child treat the
// parent's cluster as its own - connecting to it as the local cluster and
// reading its GCP project as the child's.
func (o *runtimeOnboarder) createRuntimeInstance(
	instance *v0.KubernetesRuntimeInstance,
	childDefinitionID *uint,
	controlPlaneHost bool,
	defaultRuntime bool,
) (*v0.KubernetesRuntimeInstance, error) {
	clone := *instance
	clone.Common = v0.Common{}
	clone.KubernetesRuntimeDefinitionID = childDefinitionID
	clone.ThreeportControlPlaneHost = util.Ptr(controlPlaneHost)
	clone.DefaultRuntime = util.Ptr(defaultRuntime)
	if err := decryptRuntimeInstanceSecrets(&clone, o.encryptionKey); err != nil {
		return nil, err
	}

	return getOrCreate(
		"kubernetes runtime instance on the new threeport api",
		func() (*v0.KubernetesRuntimeInstance, error) {
			return client.GetKubernetesRuntimeInstanceByName(o.child.client, o.child.endpoint, *clone.Name)
		},
		func() (*v0.KubernetesRuntimeInstance, error) {
			return client.CreateKubernetesRuntimeInstance(o.child.client, o.child.endpoint, &clone)
		},
	)
}

// copyProviderRecords copies the records specific to a runtime's infra
// provider - the provider account, and the provider's own definition and
// instance - to the child. They are what lets the child's controllers work out
// how to reach the cluster, so a runtime registered without them cannot be
// connected to.
//
// Two runtimes can share a provider account or a definition, and the child's
// records are found by name before they are created, so a second copy finds
// the first.
func (o *runtimeOnboarder) copyProviderRecords(
	infraProvider string,
	from runtimeLink,
	to runtimeLink,
) error {
	switch infraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		return o.copyEksRecords(from, to)
	case v0.KubernetesRuntimeInfraProviderOKE:
		return o.copyOkeRecords(from, to)
	case v0.KubernetesRuntimeInfraProviderGKE:
		return o.copyGkeRecords(from, to)
	}

	return nil
}

// copyEksRecords copies the AWS records behind an EKS runtime to the child.
func (o *runtimeOnboarder) copyEksRecords(from, to runtimeLink) error {
	awsRuntimeDef, err := client.GetAwsEksKubernetesRuntimeDefinitionByK8sRuntimeDef(
		o.parent.client, o.parent.endpoint, from.definitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get AwsEksKubernetesRuntimeDefinition: %w", err)
	}

	awsRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(
		o.parent.client, o.parent.endpoint, from.instanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get AwsEksKubernetesRuntimeInstance: %w", err)
	}

	awsProvider, err := client.GetAwsProviderByID(
		o.parent.client, o.parent.endpoint, *awsRuntimeInstance.AwsProviderID,
	)
	if err != nil {
		return fmt.Errorf("failed to get AwsProvider: %w", err)
	}

	awsProvider.Common = v0.Common{}
	if err := decryptField(o.encryptionKey, &awsProvider.AccessKeyID, "access key id on aws account"); err != nil {
		return err
	}
	if err := decryptField(o.encryptionKey, &awsProvider.SecretAccessKey, "secret access key on aws account"); err != nil {
		return err
	}

	createdAwsProvider, err := getOrCreate(
		"aws provider on the new threeport api",
		func() (*v0.AwsProvider, error) {
			return client.GetAwsProviderByName(o.child.client, o.child.endpoint, *awsProvider.Name)
		},
		func() (*v0.AwsProvider, error) {
			return client.CreateAwsProvider(o.child.client, o.child.endpoint, awsProvider)
		},
	)
	if err != nil {
		return err
	}

	awsRuntimeDef.Common = v0.Common{}
	awsRuntimeDef.KubernetesRuntimeDefinitionID = &to.definitionID
	createdAwsRuntimDef, err := getOrCreate(
		"aws eks runtime definition on the new threeport api",
		func() (*v0.AwsEksKubernetesRuntimeDefinition, error) {
			return client.GetAwsEksKubernetesRuntimeDefinitionByName(o.child.client, o.child.endpoint, *awsRuntimeDef.Name)
		},
		func() (*v0.AwsEksKubernetesRuntimeDefinition, error) {
			return client.CreateAwsEksKubernetesRuntimeDefinition(o.child.client, o.child.endpoint, awsRuntimeDef)
		},
	)
	if err != nil {
		return err
	}

	awsRuntimeInstance.Common = v0.Common{}
	awsRuntimeInstance.AwsProviderID = createdAwsProvider.ID
	awsRuntimeInstance.AwsEksKubernetesRuntimeDefinitionID = createdAwsRuntimDef.ID
	awsRuntimeInstance.KubernetesRuntimeInstanceID = &to.instanceID
	_, err = getOrCreate(
		"aws eks runtime instance on the new threeport api",
		func() (*v0.AwsEksKubernetesRuntimeInstance, error) {
			return client.GetAwsEksKubernetesRuntimeInstanceByName(o.child.client, o.child.endpoint, *awsRuntimeInstance.Name)
		},
		func() (*v0.AwsEksKubernetesRuntimeInstance, error) {
			return client.CreateAwsEksKubernetesRuntimeInstance(o.child.client, o.child.endpoint, awsRuntimeInstance)
		},
	)

	return err
}

// copyOkeRecords copies the OCI records behind an OKE runtime to the child.
func (o *runtimeOnboarder) copyOkeRecords(from, to runtimeLink) error {
	ociRuntimeDef, err := client.GetOciOkeKubernetesRuntimeDefinitionByK8sRuntimeDef(
		o.parent.client, o.parent.endpoint, from.definitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get OciOkeKubernetesRuntimeDefinition: %w", err)
	}

	ociRuntimeInstance, err := client.GetOciOkeKubernetesRuntimeInstanceByK8sRuntimeInst(
		o.parent.client, o.parent.endpoint, from.instanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get OciOkeKubernetesRuntimeInstance: %w", err)
	}

	ociProvider, err := client.GetOciProviderByID(
		o.parent.client, o.parent.endpoint, *ociRuntimeInstance.OciProviderID,
	)
	if err != nil {
		return fmt.Errorf("failed to get OciProvider: %w", err)
	}

	ociProvider.Common = v0.Common{}
	if err := decryptField(o.encryptionKey, &ociProvider.PrivateKey, "OCI provider private key"); err != nil {
		return err
	}

	createdOciProvider, err := getOrCreate(
		"oci provider on the new threeport api",
		func() (*v0.OciProvider, error) {
			return client.GetOciProviderByName(o.child.client, o.child.endpoint, *ociProvider.Name)
		},
		func() (*v0.OciProvider, error) {
			return client.CreateOciProvider(o.child.client, o.child.endpoint, ociProvider)
		},
	)
	if err != nil {
		return err
	}

	ociRuntimeDef.Common = v0.Common{}
	ociRuntimeDef.KubernetesRuntimeDefinitionID = &to.definitionID
	createdOciRuntimeDef, err := getOrCreate(
		"oci oke runtime definition on the new threeport api",
		func() (*v0.OciOkeKubernetesRuntimeDefinition, error) {
			return client.GetOciOkeKubernetesRuntimeDefinitionByName(o.child.client, o.child.endpoint, *ociRuntimeDef.Name)
		},
		func() (*v0.OciOkeKubernetesRuntimeDefinition, error) {
			return client.CreateOciOkeKubernetesRuntimeDefinition(o.child.client, o.child.endpoint, ociRuntimeDef)
		},
	)
	if err != nil {
		return err
	}

	ociRuntimeInstance.Common = v0.Common{}
	ociRuntimeInstance.OciProviderID = createdOciProvider.ID
	ociRuntimeInstance.OciOkeKubernetesRuntimeDefinitionID = createdOciRuntimeDef.ID
	ociRuntimeInstance.KubernetesRuntimeInstanceID = &to.instanceID
	_, err = getOrCreate(
		"oci oke runtime instance on the new threeport api",
		func() (*v0.OciOkeKubernetesRuntimeInstance, error) {
			return client.GetOciOkeKubernetesRuntimeInstanceByName(o.child.client, o.child.endpoint, *ociRuntimeInstance.Name)
		},
		func() (*v0.OciOkeKubernetesRuntimeInstance, error) {
			return client.CreateOciOkeKubernetesRuntimeInstance(o.child.client, o.child.endpoint, ociRuntimeInstance)
		},
	)

	return err
}

// copyGkeRecords copies the GCP records behind a GKE runtime to the child: the
// GcpProvider, GcpGkeKubernetesRuntimeDefinition and
// GcpGkeKubernetesRuntimeInstance.
//
// Without them a controller cannot build a token for the cluster at all, and
// nothing can tell which GCP project the control plane runs in. The instance
// is created Reconciled because the cluster already exists - left to the
// child's gcp-controller, it would try to provision one.
//
// The provider's service account credentials, when it has any, go over as
// plaintext (the child API encrypts them with its own key). When it has none
// they stay unset, which is the ambient Workload Identity case: the child
// reaches the cluster as its own principal and has no key to hold. Which of
// the two applies is the credential precedence policy's to decide when a token
// is built, so it is copied as it is rather than chosen here.
func (o *runtimeOnboarder) copyGkeRecords(from, to runtimeLink) error {
	gcpDefinition, err := client.GetGcpGkeKubernetesRuntimeDefinitionByK8sRuntimeDef(
		o.parent.client, o.parent.endpoint, from.definitionID,
	)
	if err != nil {
		return fmt.Errorf("failed to get GcpGkeKubernetesRuntimeDefinition: %w", err)
	}

	gcpInstance, err := client.GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst(
		o.parent.client, o.parent.endpoint, from.instanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get GcpGkeKubernetesRuntimeInstance: %w", err)
	}
	if gcpInstance.GcpProviderID == nil {
		return errors.New("the GcpGkeKubernetesRuntimeInstance has no GCP provider")
	}

	gcpProvider, err := client.GetGcpProviderByID(
		o.parent.client, o.parent.endpoint, *gcpInstance.GcpProviderID,
	)
	if err != nil {
		return fmt.Errorf("failed to get GcpProvider: %w", err)
	}

	gcpProvider.Common = v0.Common{}
	gcpProvider.GcpGkeKubernetesRuntimeInstances = nil
	if err := decryptField(o.encryptionKey, &gcpProvider.ServiceAccountCredentials, "GCP provider service account credentials"); err != nil {
		return err
	}

	createdGcpProvider, err := getOrCreate(
		"gcp provider on the new threeport api",
		func() (*v0.GcpProvider, error) {
			return client.GetGcpProviderByName(o.child.client, o.child.endpoint, *gcpProvider.Name)
		},
		func() (*v0.GcpProvider, error) {
			return client.CreateGcpProvider(o.child.client, o.child.endpoint, gcpProvider)
		},
	)
	if err != nil {
		return err
	}

	gcpDefinition.Common = v0.Common{}
	gcpDefinition.GcpGkeKubernetesRuntimeInstances = nil
	gcpDefinition.KubernetesRuntimeDefinitionID = &to.definitionID
	createdGcpDefinition, err := getOrCreate(
		"gcp gke runtime definition on the new threeport api",
		func() (*v0.GcpGkeKubernetesRuntimeDefinition, error) {
			return client.GetGcpGkeKubernetesRuntimeDefinitionByName(o.child.client, o.child.endpoint, *gcpDefinition.Name)
		},
		func() (*v0.GcpGkeKubernetesRuntimeDefinition, error) {
			return client.CreateGcpGkeKubernetesRuntimeDefinition(o.child.client, o.child.endpoint, gcpDefinition)
		},
	)
	if err != nil {
		return err
	}

	gcpInstance.Common = v0.Common{}
	gcpInstance.GcpProviderID = createdGcpProvider.ID
	gcpInstance.GcpGkeKubernetesRuntimeDefinitionID = createdGcpDefinition.ID
	gcpInstance.KubernetesRuntimeInstanceID = &to.instanceID
	gcpInstance.Reconciled = util.Ptr(true)
	_, err = getOrCreate(
		"gcp gke runtime instance on the new threeport api",
		func() (*v0.GcpGkeKubernetesRuntimeInstance, error) {
			return client.GetGcpGkeKubernetesRuntimeInstanceByName(o.child.client, o.child.endpoint, *gcpInstance.Name)
		},
		func() (*v0.GcpGkeKubernetesRuntimeInstance, error) {
			return client.CreateGcpGkeKubernetesRuntimeInstance(o.child.client, o.child.endpoint, gcpInstance)
		},
	)

	return err
}

// gkeRuntimeProjectID returns the GCP project of the GKE cluster a kubernetes
// runtime instance is, read from the instance's GCP provider.
func gkeRuntimeProjectID(
	apiClient *http.Client,
	apiEndpoint string,
	kubernetesRuntimeInstanceID uint,
) (string, error) {
	gkeInstance, err := client.GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst(
		apiClient, apiEndpoint, kubernetesRuntimeInstanceID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to get GCP GKE runtime instance: %w", err)
	}
	if gkeInstance.GcpProviderID == nil {
		return "", errors.New("the GCP GKE runtime instance has no GCP provider")
	}

	gcpProvider, err := client.GetGcpProviderByID(apiClient, apiEndpoint, *gkeInstance.GcpProviderID)
	if err != nil {
		return "", fmt.Errorf("failed to get GCP provider: %w", err)
	}
	if gcpProvider.ProjectID == nil || *gcpProvider.ProjectID == "" {
		return "", errors.New("the GCP provider for this runtime has no project ID")
	}

	return *gcpProvider.ProjectID, nil
}
