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
	GcpMachineType string
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
			GcpMachineType: "e2-micro",               // 0.25:1
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "XSmall",
			AwsMachineType: "t3.micro",          // 2:1
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
			GcpMachineType: "e2-small",          // 0.5:2
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "Small",
			AwsMachineType: "t3.small",          // 2:2
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
			GcpMachineType: "e2-medium",         // 1:4
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "Medium",
			AwsMachineType: "t3.medium",         // 2:4
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
			GcpMachineType: "e2-medium",         // 1:4
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "Large",
			AwsMachineType: "m8i.large",         // 2:8
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
			GcpMachineType: "e2-standard-4",     // 4:16
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "XLarge",
			AwsMachineType: "m8i.xlarge",        // 4:16
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
			GcpMachineType: "e2-standard-4",     // 4:16
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "2XLarge",
			AwsMachineType: "m8i.2xlarge",       // 8:32
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
			GcpMachineType: "e2-standard-8",     // 8:32
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "3XLarge",
			AwsMachineType: "m8i.4xlarge",       // 16:64
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
			GcpMachineType: "e2-standard-16",    // 16:64
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "4XLarge",
			AwsMachineType: "m8i.8xlarge",       // 32:128
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
			GcpMachineType: "e2-standard-32",    // 32:128
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "5XLarge",
			AwsMachineType: "m8i.12xlarge",        // 48:192
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
			GcpMachineType: "n2-standard-48",      // 48:192
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "6XLarge",
			AwsMachineType: "m8i.16xlarge",        // 64:256
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
			GcpMachineType: "n2-standard-64",      // 64:256
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "7XLarge",
			AwsMachineType: "m8i.24xlarge",        // 96:384
			OciMachineType: "VM.Standard.E5.Flex", // 1-94:1-1049
			GcpMachineType: "n2-standard-96",      // 96:384
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "8XLarge",
			AwsMachineType: "m8i.32xlarge",        // 128:512
			OciMachineType: "VM.Standard.E6.Flex", // 1-126:1-1454
			GcpMachineType: "n2-standard-128",     // 128:512
		},
		{
			NodeProfile:    "Balanced",
			NodeSize:       "9XLarge",
			AwsMachineType: "m8i.48xlarge",        // 192:768
			OciMachineType: "VM.Standard.E6.Flex", // 1-126:1-1454
			GcpMachineType: "m3-ultramem-128",     // 128:1952 (closest available)
		},
		// NodeProfile: ComputeOptimized
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "Small",
			AwsMachineType: "c8g.medium",         // 1:2
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
			GcpMachineType: "c2-standard-4",      // 4:16
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "Medium",
			AwsMachineType: "c8g.large",          // 2:4
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
			GcpMachineType: "c2-standard-8",      // 8:32
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "Large",
			AwsMachineType: "c8g.xlarge",         // 4:8
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
			GcpMachineType: "c2-standard-16",     // 16:64
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "XLarge",
			AwsMachineType: "c8g.2xlarge",        // 8:16
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
			GcpMachineType: "c2-standard-30",     // 30:120
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "2XLarge",
			AwsMachineType: "c8g.4xlarge",        // 16:32
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
			GcpMachineType: "c2-standard-60",     // 60:240
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "3XLarge",
			AwsMachineType: "c8g.8xlarge",       // 32:64
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
			GcpMachineType: "c2d-standard-56",   // 56:224
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "4XLarge",
			AwsMachineType: "c8g.12xlarge",        // 48:96
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
			GcpMachineType: "c2d-standard-112",    // 112:448
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "5XLarge",
			AwsMachineType: "c8g.16xlarge",        // 64:128
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
			GcpMachineType: "c3-standard-88",      // 88:352
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "6XLarge",
			AwsMachineType: "c8g.24xlarge",        // 96:192
			OciMachineType: "VM.Standard.E5.Flex", // 1-94:1-1049
			GcpMachineType: "c3-standard-176",     // 176:704
		},
		{
			NodeProfile:    "ComputeOptimized",
			NodeSize:       "7XLarge",
			AwsMachineType: "c8g.48xlarge",        // 192:384
			OciMachineType: "VM.Standard.E6.Flex", // 1-126:1-1454
			GcpMachineType: "c3-highcpu-176",      // 176:352
		},
		// NodeProfile: MemoryOptimized
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "Medium",
			AwsMachineType: "r8g.large",          // 2:16
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
			GcpMachineType: "n2-highmem-2",       // 2:16
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "Large",
			AwsMachineType: "r8g.xlarge",         // 4:32
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
			GcpMachineType: "n2-highmem-4",       // 4:32
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "XLarge",
			AwsMachineType: "r8g.2xlarge",        // 8:64
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
			GcpMachineType: "n2-highmem-8",       // 8:64
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "2XLarge",
			AwsMachineType: "r8g.4xlarge",        // 16:128
			OciMachineType: "VM.Optimized3.Flex", // 1-18:1-256
			GcpMachineType: "n2-highmem-16",      // 16:128
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "3XLarge",
			AwsMachineType: "r8g.8xlarge",       // 32:256
			OciMachineType: "VM.Standard3.Flex", // 1-32:1-512
			GcpMachineType: "n2-highmem-32",     // 32:256
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "4XLarge",
			AwsMachineType: "r8g.12xlarge",        // 48:384
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
			GcpMachineType: "n2-highmem-48",       // 48:384
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "5XLarge",
			AwsMachineType: "r8g.16xlarge",        // 64:512
			OciMachineType: "VM.Standard.E4.Flex", // 1-64:1-1024
			GcpMachineType: "n2-highmem-64",       // 64:512
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "6XLarge",
			AwsMachineType: "r8g.24xlarge",        // 96:768
			OciMachineType: "VM.Standard.E5.Flex", // 1-94:1-1049
			GcpMachineType: "n2-highmem-96",       // 96:768
		},
		{
			NodeProfile:    "MemoryOptimized",
			NodeSize:       "7XLarge",
			AwsMachineType: "r8g.48xlarge",        // 192:1536
			OciMachineType: "VM.Standard.E6.Flex", // 1-126:1-1454
			GcpMachineType: "n2-highmem-128",      // 128:864
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
				case "oci":
					return m.OciMachineType, nil
				case "gcp":
					return m.GcpMachineType, nil
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
