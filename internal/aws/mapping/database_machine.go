package mapping

import "fmt"

// MachineClassMap contains a threeport machine size with the corresponding
// machine class used by the cloud provider.
type MachineClassMap struct {
	MachineSize     string
	AwsMachineClass string
}

// ProviderError is an error returned when an unsupported provider is used.
type ProviderError struct {
	Message string
}

// Error returns a customized message for the ProviderError.
func (e *ProviderError) Error() string {
	return e.Message
}

// MachineClassError is an error returned when an unsupported machine class is
// used.
type MachineClassError struct {
	Message string
}

// Error returns a customized message for the MachineClassError.
func (e *MachineClassError) Error() string {
	return e.Message
}

// MachineSizeError is an error returned when an unsupported threeport machine
// size is used.
type MachineSizeError struct {
	Message string
}

// Error returns a customized message for the MachineSizeError.
func (e *MachineSizeError) Error() string {
	return e.Message
}

// getMachineClassMap returns the map of threeport database machine sizes to cloud
// provider machine classes.
// Comments on machine class indicate vCPU:GiB memory.
func getMachineClassMap() *[]MachineClassMap {
	return &[]MachineClassMap{
		{
			MachineSize:     "XSmall",
			AwsMachineClass: "db.t3.micro", // 2:1
		},
		{
			MachineSize:     "Small",
			AwsMachineClass: "db.t3.small", // 2:2
		},
		{
			MachineSize:     "Medium",
			AwsMachineClass: "db.t3.medium", // 2:4
		},
		{
			MachineSize:     "Large",
			AwsMachineClass: "db.m5.large", // 2:8
		},
		{
			MachineSize:     "XLarge",
			AwsMachineClass: "db.m5.xlarge", // 4:16
		},
		{
			MachineSize:     "2XLarge",
			AwsMachineClass: "db.m5.2xlarge", // 8:32
		},
		{
			MachineSize:     "3XLarge",
			AwsMachineClass: "db.m5.4xlarge", // 16:64
		},
		{
			MachineSize:     "4XLarge",
			AwsMachineClass: "db.m5.8xlarge", // 32:128
		},
	}
}

// ValidMachineSize returns true if the machine size provided is a supported machine size.
func ValidMachineSize(machineSize string) bool {
	// validate machine size
	machineSizeFound := false
	for _, mapping := range *getMachineClassMap() {
		if machineSize == mapping.MachineSize {
			machineSizeFound = true
			break
		}
	}

	return machineSizeFound
}

// GetProviderMachineClassForMachineSize returns a cloud provider machine class for a given
// threeport machine size and provider.
func GetProviderMachineClassForMachineSize(provider, machineSize string) (string, error) {
	for _, r := range *getMachineClassMap() {
		if r.MachineSize == machineSize {
			switch provider {
			case "aws":
				return r.AwsMachineClass, nil
			default:
				msg := fmt.Sprintf("provider %s not supported", provider)
				return "", &ProviderError{Message: msg}
			}
		}
	}

	msg := fmt.Sprintf("machine size %s not supported", machineSize)
	return "", &MachineSizeError{Message: msg}
}

// GetMachineSizeForAwsMachineClass returns the threeport machine size for a given AWS
// machine class.
func GetMachineSizeForAwsMachineClass(awsMachineClass string) (string, error) {
	for _, r := range *getMachineClassMap() {
		if r.AwsMachineClass == awsMachineClass {
			return r.MachineSize, nil
		}
	}

	msg := fmt.Sprintf("AWS machine class %s not supported", awsMachineClass)
	return "", &MachineClassError{Message: msg}
}
