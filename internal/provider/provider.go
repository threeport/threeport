package provider

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	kube "github.com/threeport/threeport/pkg/kube/v0"
)

const (
	// Max length of runtime names prevents infra provider resource names
	// exceeding maximum lengths imposed by provider.
	RuntimeNameMaxLength = 30

	// The name of the cloud provider account that is used to create a genesis Threeport
	// control plane.  When a genesis control plane is created, the account that was used
	// to create the control plane is stored in the Threeport API with this name.
	DefaultAccountName = "default-account"
)

// KubernetesRuntimeInfra is the interface each provider has to satisfy to manage
// Kubernetes runtime infra.
type KubernetesRuntimeInfra interface {
	Create() (*kube.KubeConnectionInfo, error)
	Delete() error
}

// ThreeportRuntimeName returns the name for a Kubernetes runtime that hosts the
// threeport control plane.
func ThreeportRuntimeName(threeportInstanceName string) string {
	return fmt.Sprintf("threeport-%s", threeportInstanceName)
}

// ThreeportProviderTags returns the standard tags applied to cloud provider
// infrastructure resources to properly identify them.
func ThreeportProviderTags() map[string]string {
	return map[string]string{"ProvisionedBy": "threeport"}
}

const (
	// GcpLabelProvisionedBy is the GCP label key for Threeport ownership.
	GcpLabelProvisionedBy = "provisioned-by"
	// GcpLabelThreeportName is the GCP label key for the owning object name.
	GcpLabelThreeportName = "threeport-name"
	// GcpLabelProvisionedByValue is the GCP label value for Threeport ownership.
	GcpLabelProvisionedByValue = "threeport"
)

// GcpResourceLabels returns Compute-safe labels for a GCP resource owned by
// the named Threeport object. Keys are lowercase; values are at most 63
// characters of [a-z0-9_-].
func GcpResourceLabels(ownerName string) map[string]string {
	return map[string]string{
		GcpLabelProvisionedBy: GcpLabelProvisionedByValue,
		GcpLabelThreeportName: gcpLabelValue(ownerName),
	}
}

// gcpLabelValue maps a Threeport object name onto a GCP label value.
func gcpLabelValue(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			if prevDash {
				continue
			}
			r = '-'
			prevDash = true
		} else {
			prevDash = r == '-'
		}
		b.WriteRune(r)
		if b.Len() >= 63 {
			break
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "unnamed"
	}
	return s
}

// GcpOwnershipDescription is the IAM description suffix that records the same
// ownership pair as GcpResourceLabels. IAM service accounts have no labels.
func GcpOwnershipDescription(ownerName string) string {
	labels := GcpResourceLabels(ownerName)
	return fmt.Sprintf("%s=%s %s=%s",
		GcpLabelProvisionedBy, labels[GcpLabelProvisionedBy],
		GcpLabelThreeportName, labels[GcpLabelThreeportName],
	)
}

// GcpLabelsInput maps GcpResourceLabels onto a Pulumi string map.
func GcpLabelsInput(ownerName string) pulumi.StringMap {
	labels := GcpResourceLabels(ownerName)
	out := make(pulumi.StringMap, len(labels))
	for k, v := range labels {
		out[k] = pulumi.String(v)
	}
	return out
}
