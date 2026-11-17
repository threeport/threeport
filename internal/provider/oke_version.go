package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	ocicontainerengine "github.com/oracle/oci-go-sdk/v65/containerengine"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// resolveKubernetesVersion sets Version to the OKE release the cluster is
// created on.  OKE takes a full release such as "v1.36.4", while a definition
// records a minor version such as "1.36": the minor is resolved to its newest
// patch that OKE offers for clusters and has a worker node image for the node
// shape.  A full release is kept as it is, and an unset one is the default
// minor version's.
func (i *KubernetesRuntimeInfraOKE) resolveKubernetesVersion() error {
	if strings.HasPrefix(i.Version, "v") {
		return nil
	}
	minor := v0.KubernetesVersionOrDefault(&i.Version)

	containerClient, err := ocicontainerengine.NewContainerEngineClientWithConfigurationProvider(i.ConfigProvider)
	if err != nil {
		return fmt.Errorf("failed to create container engine client: %w", err)
	}
	containerClient.SetRegion(i.Region)

	clusterOptions, err := containerClient.GetClusterOptions(context.Background(), ocicontainerengine.GetClusterOptionsRequest{
		ClusterOptionId: common.String("all"),
		CompartmentId:   common.String(i.CompartmentOCID),
	})
	if err != nil {
		return fmt.Errorf("failed to get the Kubernetes versions OKE offers: %w", err)
	}
	nodePoolOptions, err := containerClient.GetNodePoolOptions(context.Background(), ocicontainerengine.GetNodePoolOptionsRequest{
		CompartmentId:    common.String(i.CompartmentOCID),
		NodePoolOptionId: common.String("all"),
	})
	if err != nil {
		return fmt.Errorf("failed to get node pool options: %w", err)
	}
	var imageNames []string
	for _, source := range nodePoolOptions.Sources {
		if image, ok := source.(ocicontainerengine.NodeSourceViaImageOption); ok && image.SourceName != nil {
			imageNames = append(imageNames, *image.SourceName)
		}
	}

	version, err := newestOkePatch(minor, clusterOptions.KubernetesVersions, imageNames, okeImageArch(i.WorkerNodeShape))
	if err != nil {
		return err
	}
	i.Version = version

	return nil
}

// keepStackVersion sets Version to the release of the cluster the stack's
// deployment holds, and reports whether it holds one.
func (i *KubernetesRuntimeInfraOKE) keepStackVersion(deployment json.RawMessage) bool {
	version, held := clusterInputInStack(deployment, "oci:ContainerEngine/cluster:Cluster", "kubernetesVersion")
	if !held || version == "" {
		return false
	}
	i.Version = version

	return true
}

// newestOkePatch returns the newest release of minor, such as "v1.36.4" for
// "1.36", among the versions OKE offers for clusters, that has a worker node
// image for arch.
func newestOkePatch(minor string, offered, imageNames []string, arch string) (string, error) {
	release := regexp.MustCompile(`^v` + regexp.QuoteMeta(minor) + `\.(\d+)$`)
	type patch struct {
		version string
		number  int
	}
	var patches []patch
	for _, version := range offered {
		if match := release.FindStringSubmatch(version); match != nil {
			number, err := strconv.Atoi(match[1])
			if err == nil {
				patches = append(patches, patch{version: version, number: number})
			}
		}
	}
	slices.SortFunc(patches, func(a, b patch) int { return b.number - a.number })

	for _, p := range patches {
		if okeImageFor(imageNames, p.version, arch) != "" {
			return p.version, nil
		}
	}

	if len(patches) == 0 {
		return "", fmt.Errorf(
			"OKE offers no release of Kubernetes %s for new clusters; it offers %s",
			minor, strings.Join(offered, ", "),
		)
	}
	return "", fmt.Errorf("OKE has no %s worker node image for any release of Kubernetes %s", arch, minor)
}

// okeImageFor returns the name, among imageNames, of the OKE worker node image
// for a release such as "v1.36.4" and arch, or "" if there is none.  The
// release is matched with the hyphen after it, so 1.36.1 doesn't match an
// image of 1.36.10.
func okeImageFor(imageNames []string, version, arch string) string {
	pattern := fmt.Sprintf("OKE-%s-", strings.TrimPrefix(version, "v"))
	for _, name := range imageNames {
		if strings.Contains(name, pattern) && okeImageIsFor(name, arch) {
			return name
		}
	}

	return ""
}

// okeImageIsFor reports whether an OKE worker node image is for arch.  Arm
// images carry "aarch64" in their names, as in
// Oracle-Linux-8.10-aarch64-2026.09.30-0-OKE-1.36.4-1001, while x86 images
// carry no architecture, as in Oracle-Linux-8.10-2026.09.30-0-OKE-1.36.4-1001.
// GPU images are x86 too but need a GPU shape, so they are left out.
func okeImageIsFor(name, arch string) bool {
	if arch == "aarch64" {
		return strings.Contains(name, "aarch64")
	}

	return !strings.Contains(name, "aarch64") && !strings.Contains(name, "GPU")
}

// okeImageArch returns the architecture of the worker node images for a node
// shape: Ampere A1 shapes are Arm.
func okeImageArch(shape string) string {
	if strings.Contains(shape, ".A1.") {
		return "aarch64"
	}

	return "x86_64"
}

// minorVersion returns the minor version of a Kubernetes version such as
// "v1.36.4" or "1.36", "1.36" for both.
func minorVersion(version string) string {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)
	if len(parts) < 2 {
		return ""
	}

	return parts[0] + "." + parts[1]
}

// CreatedKubernetesVersion returns the Kubernetes minor version of the
// cluster Create created or found in the stack, such as "1.36".
func (i *KubernetesRuntimeInfraOKE) CreatedKubernetesVersion() string {
	return minorVersion(i.Version)
}

// clusterInputInStack returns an input of the cluster an exported deployment
// holds, and whether it holds the cluster at all.  An input nested in an
// object is named by its path, such as "releaseChannel.channel".  The input is
// "" when the cluster was created without it.  A create resumed on that stack keeps the
// cluster's version, rather than asking for the one its definition records:
// the stack may predate it, and a provider upgrades a cluster one minor
// version at a time, if it can upgrade it in place at all.
func clusterInputInStack(deployment json.RawMessage, clusterType, input string) (string, bool) {
	var contents struct {
		Resources []struct {
			Type   string         `json:"type"`
			Inputs map[string]any `json:"inputs"`
		} `json:"resources"`
	}
	if len(deployment) == 0 || json.Unmarshal(deployment, &contents) != nil {
		return "", false
	}

	for _, resource := range contents.Resources {
		if resource.Type == clusterType {
			var value any = resource.Inputs
			for _, key := range strings.Split(input, ".") {
				object, _ := value.(map[string]any)
				value = object[key]
			}
			text, _ := value.(string)
			return text, true
		}
	}

	return "", false
}
