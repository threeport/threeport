package v0

import (
	"fmt"

	aurora "github.com/logrusorgru/aurora"
)

// CliOutputError prints a formatted error message in red.
func CliOutputError(message string, err error) {
	if err != nil {
		fmt.Println(aurora.Red(fmt.Sprintf("Error: %s\n%s", message, err)))
	} else {
		fmt.Println(aurora.Red(fmt.Sprintf("Error: %s", message)))
	}
}

// CliOutputInfo prints a formatted info message.
func CliOutputInfo(message string) {
	fmt.Printf("Info: %s\n", message)
}

// CliOutputNotice prints a formatted notice message in blue.
func CliOutputNotice(message string) {
	fmt.Println(aurora.Blue(fmt.Sprintf("Notice: %s", message)))
}

// CliOutputWarning prints a formatted warning message in yellow.
func CliOutputWarning(message string) {
	fmt.Println(aurora.Yellow(fmt.Sprintf("Warning: %s", message)))
}

// CliOutputComplete prints a formatted message in green. Used when operations are finished.
func CliOutputComplete(message string) {
	fmt.Println(aurora.Green(fmt.Sprintf("Complete: %s", message)))
}
