package output

import (
	"fmt"

	. "github.com/logrusorgru/aurora"
)

// Error returns a formatted error message in red.
func Error(message string, err error) {
	if err != nil {
		fmt.Println(Red(fmt.Sprintf("Error: %s\n%s", message, err)))
	} else {
		fmt.Println(Red(fmt.Sprintf("Error: %s\n", message)))
	}
}

// Info returns a formatted info message.
func Info(message string) {
	fmt.Printf("Info: %s\n", message)
}

// Warning returns a formatted warning message in yellow.
func Warning(message string) {
	fmt.Println(Yellow(fmt.Sprintf("Warning: %s\n", message)))
}

// Complete returns a formatted message in green.  Used when operations are
// finished.
func Complete(message string) {
	fmt.Println(Green(fmt.Sprintf("Complete: %s\n", message)))
}
