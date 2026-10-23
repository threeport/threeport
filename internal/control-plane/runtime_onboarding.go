package controlplane

import (
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// apiTarget is a threeport API that records are read from or written to.
type apiTarget struct {
	client   *http.Client
	endpoint string
}

// runtimeOnboarder copies kubernetes runtime records from the control plane
// that is bootstrapping a child to the child's own API.
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
	if instance.CertificateKey != nil && *instance.CertificateKey != "" {
		decryptedKey, err := encryption.Decrypt(encryptionKey, *instance.CertificateKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt kubernetes runtime instance certificate key: %w", err)
		}
		instance.CertificateKey = &decryptedKey
	}

	if instance.ConnectionToken != nil && *instance.ConnectionToken != "" {
		decryptedToken, err := encryption.Decrypt(encryptionKey, *instance.ConnectionToken)
		if err != nil {
			return fmt.Errorf("failed to decrypt kubernetes runtime instance connection token: %w", err)
		}
		instance.ConnectionToken = &decryptedToken
	}

	return nil
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

	created, err := client.CreateKubernetesRuntimeInstance(o.child.client, o.child.endpoint, &clone)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes runtime instance on new threeport api: %w", err)
	}

	return created, nil
}

// copyProviderRecords copies the records specific to a runtime's infra
// provider - the provider account, and the provider's own definition and
// instance - to the child. They are what lets the child's controllers work out
// how to reach the cluster, so a runtime registered without them cannot be
// connected to.
//
// definitionCopied is for a runtime that shares its generic definition with one
// already copied: the provider definition is then already on the child and only
// the instance is added. It is honored for GKE only, the one provider this is
// used with for more than one runtime.
func (o *runtimeOnboarder) copyProviderRecords(
	infraProvider string,
	from runtimeLink,
	to runtimeLink,
	definitionCopied bool,
) error {
	switch infraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		return o.copyEksRecords(from, to)
	case v0.KubernetesRuntimeInfraProviderOKE:
		return o.copyOkeRecords(from, to)
	case v0.KubernetesRuntimeInfraProviderGKE:
		return o.copyGkeRecords(from, to, definitionCopied)
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
	if awsProvider.AccessKeyID != nil && *awsProvider.AccessKeyID != "" {
		decryptedKey, err := encryption.Decrypt(o.encryptionKey, *awsProvider.AccessKeyID)
		if err != nil {
			return fmt.Errorf("failed to decrypt access key id on aws account: %w", err)
		}

		awsProvider.AccessKeyID = &decryptedKey
	}

	if awsProvider.SecretAccessKey != nil && *awsProvider.SecretAccessKey != "" {
		decryptedKey, err := encryption.Decrypt(o.encryptionKey, *awsProvider.SecretAccessKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt secret access key on aws account: %w", err)
		}

		awsProvider.SecretAccessKey = &decryptedKey
	}

	createdAwsProvider, err := client.CreateAwsProvider(o.child.client, o.child.endpoint, awsProvider)
	if err != nil {
		return fmt.Errorf("failed to create AwsProvider: %w", err)
	}

	awsRuntimeDef.Common = v0.Common{}
	awsRuntimeDef.KubernetesRuntimeDefinitionID = &to.definitionID
	createdAwsRuntimDef, err := client.CreateAwsEksKubernetesRuntimeDefinition(
		o.child.client, o.child.endpoint, awsRuntimeDef,
	)
	if err != nil {
		return fmt.Errorf("failed to create AwsEksKubernetesRuntimeDefinition: %w", err)
	}

	awsRuntimeInstance.Common = v0.Common{}
	awsRuntimeInstance.AwsProviderID = createdAwsProvider.ID
	awsRuntimeInstance.AwsEksKubernetesRuntimeDefinitionID = createdAwsRuntimDef.ID
	awsRuntimeInstance.KubernetesRuntimeInstanceID = &to.instanceID
	if _, err := client.CreateAwsEksKubernetesRuntimeInstance(
		o.child.client, o.child.endpoint, awsRuntimeInstance,
	); err != nil {
		return fmt.Errorf("failed to create AwsEksKubernetesRuntimeInstance: %w", err)
	}

	return nil
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
	if ociProvider.PrivateKey != nil && *ociProvider.PrivateKey != "" {
		decryptedKey, err := encryption.Decrypt(o.encryptionKey, *ociProvider.PrivateKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt OCI provider private key: %w", err)
		}
		ociProvider.PrivateKey = &decryptedKey
	}

	createdOciProvider, err := client.CreateOciProvider(o.child.client, o.child.endpoint, ociProvider)
	if err != nil {
		return fmt.Errorf("failed to create OciProvider: %w", err)
	}

	ociRuntimeDef.Common = v0.Common{}
	ociRuntimeDef.KubernetesRuntimeDefinitionID = &to.definitionID
	createdOciRuntimeDef, err := client.CreateOciOkeKubernetesRuntimeDefinition(
		o.child.client, o.child.endpoint, ociRuntimeDef,
	)
	if err != nil {
		return fmt.Errorf("failed to create OciOkeKubernetesRuntimeDefinition: %w", err)
	}

	ociRuntimeInstance.Common = v0.Common{}
	ociRuntimeInstance.OciProviderID = createdOciProvider.ID
	ociRuntimeInstance.OciOkeKubernetesRuntimeDefinitionID = createdOciRuntimeDef.ID
	ociRuntimeInstance.KubernetesRuntimeInstanceID = &to.instanceID
	if _, err := client.CreateOciOkeKubernetesRuntimeInstance(
		o.child.client, o.child.endpoint, ociRuntimeInstance,
	); err != nil {
		return fmt.Errorf("failed to create OciOkeKubernetesRuntimeInstance: %w", err)
	}

	return nil
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
func (o *runtimeOnboarder) copyGkeRecords(from, to runtimeLink, definitionCopied bool) error {
	parentInstance, err := client.GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst(
		o.parent.client, o.parent.endpoint, from.instanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to get GcpGkeKubernetesRuntimeInstance: %w", err)
	}
	if parentInstance.GcpProviderID == nil {
		return errors.New("the GcpGkeKubernetesRuntimeInstance has no GCP provider")
	}

	gcpProvider, err := client.GetGcpProviderByID(
		o.parent.client, o.parent.endpoint, *parentInstance.GcpProviderID,
	)
	if err != nil {
		return fmt.Errorf("failed to get GcpProvider: %w", err)
	}

	childProvider, err := o.getOrCreateGcpProvider(gcpProvider)
	if err != nil {
		return err
	}

	var childDefinition *v0.GcpGkeKubernetesRuntimeDefinition
	if definitionCopied {
		childDefinition, err = client.GetGcpGkeKubernetesRuntimeDefinitionByK8sRuntimeDef(
			o.child.client, o.child.endpoint, to.definitionID,
		)
		if err != nil {
			return fmt.Errorf("failed to get GcpGkeKubernetesRuntimeDefinition on new threeport api: %w", err)
		}
	} else {
		gcpDefinition, err := client.GetGcpGkeKubernetesRuntimeDefinitionByK8sRuntimeDef(
			o.parent.client, o.parent.endpoint, from.definitionID,
		)
		if err != nil {
			return fmt.Errorf("failed to get GcpGkeKubernetesRuntimeDefinition: %w", err)
		}

		gcpDefinition.Common = v0.Common{}
		gcpDefinition.GcpGkeKubernetesRuntimeInstances = nil
		gcpDefinition.KubernetesRuntimeDefinitionID = &to.definitionID
		childDefinition, err = client.CreateGcpGkeKubernetesRuntimeDefinition(
			o.child.client, o.child.endpoint, gcpDefinition,
		)
		if err != nil {
			return fmt.Errorf("failed to create GcpGkeKubernetesRuntimeDefinition: %w", err)
		}
	}

	parentInstance.Common = v0.Common{}
	parentInstance.GcpProviderID = childProvider.ID
	parentInstance.GcpGkeKubernetesRuntimeDefinitionID = childDefinition.ID
	parentInstance.KubernetesRuntimeInstanceID = &to.instanceID
	parentInstance.Reconciled = util.Ptr(true)
	if _, err := client.CreateGcpGkeKubernetesRuntimeInstance(
		o.child.client, o.child.endpoint, parentInstance,
	); err != nil {
		return fmt.Errorf("failed to create GcpGkeKubernetesRuntimeInstance: %w", err)
	}

	return nil
}

// getOrCreateGcpProvider returns the child's GCP provider of the same name as
// a parent's, creating it if the child has none. The name is unique, and a
// provider can be shared by more than one runtime that gets copied - the
// child's own and the parent's - so a second copy must find the first.
func (o *runtimeOnboarder) getOrCreateGcpProvider(parentProvider *v0.GcpProvider) (*v0.GcpProvider, error) {
	if parentProvider.Name == nil {
		return nil, errors.New("the GCP provider has no name")
	}

	existing, err := client.GetGcpProviderByName(o.child.client, o.child.endpoint, *parentProvider.Name)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to look up GcpProvider on new threeport api: %w", err)
	}

	clone := *parentProvider
	clone.Common = v0.Common{}
	clone.GcpGkeKubernetesRuntimeInstances = nil
	if clone.ServiceAccountCredentials != nil && *clone.ServiceAccountCredentials != "" {
		decrypted, err := encryption.Decrypt(o.encryptionKey, *clone.ServiceAccountCredentials)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt GCP provider service account credentials: %w", err)
		}
		clone.ServiceAccountCredentials = &decrypted
	}

	created, err := client.CreateGcpProvider(o.child.client, o.child.endpoint, &clone)
	if err != nil {
		return nil, fmt.Errorf("failed to create GcpProvider: %w", err)
	}

	return created, nil
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
