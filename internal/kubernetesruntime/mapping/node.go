package mapping

import (
	"fmt"

	"github.com/threeport/threeport/pkg/util/v0"
)

// RegionMap contains a threeport location with the corresponding regions for
// cloud providers.
type MachineTypeMap struct {
	NodeProfile    string
	NodeSize       string
	AwsMachineType string
	//GcpMachineType string  // future use
}

// MachineTypeError is an error returned when a machine type cannot be provided
// for a provider, node profile, node size combination.
type MachineTypeError struct {
	Message string
}

// Error returns a customized message for the MachineTypeError.
func (e *MachineTypeError) Error() string {
	return e.Message
}

// getMachineTypeMap returns the map of node sizes and profiles to cloud
// provider machine types.
// Comments on machine type indicate vCPU:GiB memory.
func getMachineTypeMap() *[]MachineTypeMap {
	return &[]MachineTypeMap{
		// NodeProfile: Balanced
		{
			NodeProfile:    "Balanced",
			NodeSize:       "2XSmall",
			AwsMachineType: "t3.nano", // 2:0.5
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "XSmall",
			AwsMachineType: "t3.micro", // 2:1
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "Small",
			AwsMachineType: "t3.small", // 2:2
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "Medium",
			AwsMachineType: "t3.medium", // 2:4
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "Large",
			AwsMachineType: "m7i.large", // 2:8
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "XLarge",
			AwsMachineType: "m7i.xlarge", // 4:16
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "2XLarge",
			AwsMachineType: "m7i.2xlarge", // 8:32
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "3XLarge",
			AwsMachineType: "m7i.4xlarge", // 16:64
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "4XLarge",
			AwsMachineType: "m7i.8xlarge", // 32:128
		},
		// NodeProfile: ComputeOptimized
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "Small",
			AwsMachineType: "c7g.medium", // 1:2
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "Medium",
			AwsMachineType: "c7g.large", // 2:4
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "Large",
			AwsMachineType: "c7g.xlarge", // 4:8
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "XLarge",
			AwsMachineType: "c7g.2xlarge", // 8:16
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "2XLarge",
			AwsMachineType: "c7g.4xlarge", // 16:32
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "3XLarge",
			AwsMachineType: "c7g.8xlarge", // 32:64
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "4XLarge",
			AwsMachineType: "c7g.12xlarge", // 48:96
		},
		// NodeProfile: MemoryOptimized
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "Medium",
			AwsMachineType: "r6i.large", // 2:16
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "Large",
			AwsMachineType: "r6i.xlarge", // 4:32
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "XLarge",
			AwsMachineType: "r6i.2xlarge", // 8:64
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "2XLarge",
			AwsMachineType: "r6i.4xlarge", // 16:128
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "3XLarge",
			AwsMachineType: "r6i.8xlarge", // 32:256
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "4XLarge",
			AwsMachineType: "r6i.12xlarge", // 48:384
		},
	}
}

// GetMachineType returns a cloud provider machine type for a given provider,
// node profile and node size.
func GetMachineType(provider, nodeProfile, nodeSize string) (string, error) {
	for _, m := range *getMachineTypeMap() {
		if m.NodeProfile == nodeProfile {
			if m.NodeSize == nodeSize {
				switch provider {
				case "aws":
					return m.AwsMachineType, nil
				default:
					msg := fmt.Sprintf("provider %s not supported", provider)
					return "", &ProviderError{Message: msg}
				}
			}
		}
	}

	availableNodeSizes, err := GetNodeSizeForProfile(nodeProfile)
	if err != nil {
		return "", err
	}

	msg := fmt.Sprintf(
		"node size %s not supported for node profile %s - supported node sizes for that profile: %s",
		nodeSize,
		nodeProfile,
		availableNodeSizes,
	)
	return "", &MachineTypeError{Message: msg}
}

// GetNodeSizeForProfile returns all available node sizes for a given node
// profile.
func GetNodeSizeForProfile(nodeProfile string) ([]string, error) {
	allNodeProfiles := GetNodeProfiles()
	if !util.StringSliceContains(allNodeProfiles, nodeProfile, true) {
		msg := fmt.Sprintf(
			"node profile %s not supported - supported node profiles: %s",
			nodeProfile,
			allNodeProfiles,
		)
		return []string{}, &MachineTypeError{Message: msg}
	}

	var nodeSizes []string
	for _, m := range *getMachineTypeMap() {
		if m.NodeProfile == nodeProfile {
			nodeSizes = append(nodeSizes, m.NodeSize)
		}
	}

	return nodeSizes, nil
}

// GetNodProfiles returns all unique node profiles supported.
func GetNodeProfiles() []string {
	var nodeProfiles []string
	for _, m := range *getMachineTypeMap() {
		if !util.StringSliceContains(nodeProfiles, m.NodeProfile, true) {
			nodeProfiles = append(nodeProfiles, m.NodeProfile)
		}
	}

	return nodeProfiles
}
