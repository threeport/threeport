package provider

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// EgressCidrsOutput names the stack output in which each Kubernetes runtime
// program records the public addresses its cluster's outbound traffic comes
// from, one /32 per address.
const EgressCidrsOutput = "egressCidrs"

// exportEgressCidrs records addresses as the program's EgressCidrsOutput.  It
// is a variable so that tests can see which addresses a program records,
// which Pulumi's mocks do not show.
var exportEgressCidrs = func(ctx *pulumi.Context, addresses []pulumi.StringOutput) {
	cidrs := pulumi.StringArray{}
	for _, address := range addresses {
		cidrs = append(cidrs, address.ApplyT(func(ip string) string {
			return ip + "/32"
		}).(pulumi.StringOutput))
	}
	ctx.Export(EgressCidrsOutput, cidrs)
}

// EgressCidrsFromState returns the EgressCidrsOutput recorded in a Pulumi
// state, in checkpoint or export format, or nil when the state records none.
func EgressCidrsFromState(state *datatypes.JSON) ([]string, error) {
	if state == nil || len(*state) == 0 {
		return nil, nil
	}

	type resource struct {
		Type    string                     `json:"type"`
		Outputs map[string]json.RawMessage `json:"outputs"`
	}
	var parsed struct {
		Checkpoint struct {
			Latest struct {
				Resources []resource `json:"resources"`
			} `json:"latest"`
		} `json:"checkpoint"`
		Deployment struct {
			Resources []resource `json:"resources"`
		} `json:"deployment"`
	}
	if err := json.Unmarshal(*state, &parsed); err != nil {
		return nil, fmt.Errorf("failed to read Pulumi state: %w", err)
	}

	resources := parsed.Checkpoint.Latest.Resources
	if len(resources) == 0 {
		resources = parsed.Deployment.Resources
	}
	for _, r := range resources {
		if r.Type != "pulumi:pulumi:Stack" {
			continue
		}
		raw, ok := r.Outputs[EgressCidrsOutput]
		if !ok {
			return nil, nil
		}
		var cidrs []string
		if err := json.Unmarshal(raw, &cidrs); err != nil {
			return nil, fmt.Errorf("failed to read the %s output: %w", EgressCidrsOutput, err)
		}
		return cidrs, nil
	}

	return nil, nil
}

// StackEgressCidrs returns the EgressCidrsOutput in the workspace's state
// file, for a caller that has just created the cluster.
func (w *PulumiWorkspace) StackEgressCidrs() ([]string, error) {
	state, err := w.ReadStateFile()
	if err != nil {
		return nil, err
	}

	return EgressCidrsFromState(state)
}

// kubeApiAllowlist returns the blocks the public Kubernetes API endpoint of a
// cluster on infraProvider admits: allowedCidrs, plus the addresses of the
// control plane that manages the cluster when the endpoint is restricted at
// all.  An address already in allowedCidrs is not listed twice.  It fails
// when the combined list is longer than the provider admits, with the number
// of entries left for allowedCidrs.
func kubeApiAllowlist(infraProvider string, allowedCidrs, controlPlaneEgressCidrs []string) ([]string, error) {
	if len(allowedCidrs) == 0 {
		return nil, nil
	}

	allowlist := append([]string{}, allowedCidrs...)
	listed := make(map[string]bool, len(allowedCidrs))
	for _, cidr := range allowedCidrs {
		listed[cidr] = true
	}
	added := 0
	for _, cidr := range controlPlaneEgressCidrs {
		if !listed[cidr] {
			listed[cidr] = true
			allowlist = append(allowlist, cidr)
			added++
		}
	}

	limit, _ := v0.AllowedCidrsLimit(infraProvider, false)
	if len(allowlist) > limit {
		return nil, fmt.Errorf(
			"AllowedCidrs has %d entries and the control plane adds %d of its own addresses, but %s admits at most %d - leave at most %d entries in AllowedCidrs",
			len(allowedCidrs), added, infraProvider, limit, max(limit-added, 0),
		)
	}

	return allowlist, nil
}

// checkKubeApiAllowlist fails when the allowlist kubeApiAllowlist builds is
// longer than infraProvider admits.  A cluster's Create and CreateInfra call
// it before they change anything in the cloud, so that a list that is
// refused leaves nothing behind to clean up.
func checkKubeApiAllowlist(infraProvider string, allowedCidrs, controlPlaneEgressCidrs []string) error {
	_, err := kubeApiAllowlist(infraProvider, allowedCidrs, controlPlaneEgressCidrs)

	return err
}

// EgressCidrsJSON returns cidrs as the JSON array a kubernetes runtime
// instance's EgressCidrs holds, or nil when there are none.
func EgressCidrsJSON(cidrs []string) *datatypes.JSON {
	if len(cidrs) == 0 {
		return nil
	}

	// a slice of strings always marshals
	raw, _ := json.Marshal(cidrs)
	cidrsJSON := datatypes.JSON(raw)
	return &cidrsJSON
}

// ControlPlaneEgressCidrs returns the addresses a cluster restricted with
// AllowedCidrs admits so that the control plane can manage it: the
// EgressCidrs of the runtime that hosts the control plane.  It returns none
// for the runtime that hosts the control plane, which the control plane
// reaches from inside.  A host that has no EgressCidrs, such as a cluster
// Threeport did not provision, is logged, since the control plane can then
// reach the cluster only if AllowedCidrs lists its addresses.
func ControlPlaneEgressCidrs(
	apiClient *http.Client,
	apiServer string,
	kubernetesRuntimeInstanceID uint,
	log *logr.Logger,
) ([]string, error) {
	runtime, err := client.GetKubernetesRuntimeInstanceByID(apiClient, apiServer, kubernetesRuntimeInstanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get the kubernetes runtime instance: %w", err)
	}
	if kube.HostsControlPlane(runtime) {
		return nil, nil
	}

	hosts, err := client.GetKubernetesRuntimeInstancesByQueryString(apiClient, apiServer, "ThreeportControlPlaneHost=true")
	if err != nil {
		return nil, fmt.Errorf("failed to get the kubernetes runtime instance that hosts the control plane: %w", err)
	}
	var cidrs []string
	if len(*hosts) > 0 {
		if cidrs, err = v0.ParseCidrs("EgressCidrs", (*hosts)[0].EgressCidrs); err != nil {
			return nil, fmt.Errorf("failed to read the control plane's EgressCidrs: %w", err)
		}
	}

	if len(cidrs) == 0 && log != nil {
		log.Info(
			"warning: the runtime that hosts the control plane has no EgressCidrs, so its addresses are not admitted with AllowedCidrs - list them in AllowedCidrs, or set EgressCidrs on that runtime instance",
		)
	}

	return cidrs, nil
}
