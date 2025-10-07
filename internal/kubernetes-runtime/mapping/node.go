package mapping

import (
	"fmt"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// RegionMap contains a threeport location with the corresponding regions for
// cloud providers.
type MachineTypeMap struct {
	NodeProfile    string
	NodeSize       string
	AwsMachineType string
	OciMachineType string
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

// GetMachineTypeMap returns the map of node sizes and profiles to cloud
// provider machine types.
// Comments on machine type indicate vCPU:GiB memory.
func GetMachineTypeMap() *[]MachineTypeMap {
	return &[]MachineTypeMap{
		// NodeProfile: Balanced
		{
			NodeProfile:    "Balanced",
			NodeSize:       "2XSmall",
			AwsMachineType: "t3.nano",                // 2:0.5
			OciMachineType: "VM.Standard.E2.1.Micro", // 1:1
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "XSmall",
			AwsMachineType: "t3.micro",          // 2:1
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "Small",
			AwsMachineType: "t3.small",          // 2:2
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "Medium",
			AwsMachineType: "t3.medium",         // 2:4
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "Large",
			AwsMachineType: "m8i.large",         // 2:8
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "XLarge",
			AwsMachineType: "m8i.xlarge",        // 4:16
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "2XLarge",
			AwsMachineType: "m8i.2xlarge",       // 8:32
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "3XLarge",
			AwsMachineType: "m8i.4xlarge",       // 16:64
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "4XLarge",
			AwsMachineType: "m8i.8xlarge",       // 32:128
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "5XLarge",
			AwsMachineType: "m8i.12xlarge",        // 48:192
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "6XLarge",
			AwsMachineType: "m8i.16xlarge",        // 64:256
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "7XLarge",
			AwsMachineType: "m8i.24xlarge",        // 96:384
			OciMachineType: "VM.Standard.E5.Flex", // 1-94:1-1049
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "8XLarge",
			AwsMachineType: "m8i.32xlarge",        // 128:512
			OciMachineType: "VM.Standard.E6.Flex", // 1-126:1-1454
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "9XLarge",
			AwsMachineType: "m8i.48xlarge",        // 192:768
			OciMachineType: "VM.Standard.E6.Flex", // 1-126:1-1454
		},
		// NodeProfile: ComputeOptimized
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "Small",
			AwsMachineType: "c8g.medium",         // 1:2
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "Medium",
			AwsMachineType: "c8g.large",          // 2:4
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "Large",
			AwsMachineType: "c8g.xlarge",         // 4:8
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "XLarge",
			AwsMachineType: "c8g.2xlarge",        // 8:16
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "2XLarge",
			AwsMachineType: "c8g.4xlarge",        // 16:32
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "3XLarge",
			AwsMachineType: "c8g.8xlarge",       // 32:64
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "4XLarge",
			AwsMachineType: "c8g.12xlarge",        // 48:96
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "5XLarge",
			AwsMachineType: "c8g.16xlarge",        // 64:128
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "6XLarge",
			AwsMachineType: "c8g.24xlarge",        // 96:192
			OciMachineType: "VM.Standard.E5.Flex", // 1-94:1-1049
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "7XLarge",
			AwsMachineType: "c8g.48xlarge",        // 192:384
			OciMachineType: "VM.Standard.E6.Flex", // 1-126:1-1454
		},
		// NodeProfile: MemoryOptimized
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "Medium",
			AwsMachineType: "r8g.large",          // 2:16
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "Large",
			AwsMachineType: "r8g.xlarge",         // 4:32
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "XLarge",
			AwsMachineType: "r8g.2xlarge",        // 8:64
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "2XLarge",
			AwsMachineType: "r8g.4xlarge",        // 16:128
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "3XLarge",
			AwsMachineType: "r8g.8xlarge",       // 32:256
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "4XLarge",
			AwsMachineType: "r8g.12xlarge",        // 48:384
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "5XLarge",
			AwsMachineType: "r8g.16xlarge",        // 64:512
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "6XLarge",
			AwsMachineType: "r8g.24xlarge",        // 96:768
			OciMachineType: "VM.Standard.E5.Flex", // 1-94:1-1049
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "7XLarge",
			AwsMachineType: "r8g.48xlarge",        // 192:1536
			OciMachineType: "VM.Standard.E6.Flex", // 1-126:1-1454
		},
	}
}

// GetMachineType returns a cloud provider machine type for a given provider,
// node profile and node size.
func GetMachineType(provider, nodeProfile, nodeSize string) (string, error) {
	for _, m := range *GetMachineTypeMap() {
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
	for _, m := range *GetMachineTypeMap() {
		if m.NodeProfile == nodeProfile {
			nodeSizes = append(nodeSizes, m.NodeSize)
		}
	}

	return nodeSizes, nil
}

// GetNodProfiles returns all unique node profiles supported.
func GetNodeProfiles() []string {
	var nodeProfiles []string
	for _, m := range *GetMachineTypeMap() {
		if !util.StringSliceContains(nodeProfiles, m.NodeProfile, true) {
			nodeProfiles = append(nodeProfiles, m.NodeProfile)
		}
	}

	return nodeProfiles
}
