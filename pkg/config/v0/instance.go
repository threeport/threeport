package v0

import "fmt"

func defaultInstanceName(definitionName string) string {
	return fmt.Sprintf("%s-0", definitionName)
}
