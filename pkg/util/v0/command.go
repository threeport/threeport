package v0

import (
	"bufio"
	"fmt"
	"os/exec"
)

// RunCommandStreamOutput runs a command and streams the output back to the user.
func RunCommandStreamOutput(command string, args ...string) error {
	cmd := exec.Command(command, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error getting stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error getting stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting command: %w", err)
	}

	done := make(chan struct{})

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			fmt.Println("error reading stdout: ", err)
		}
		done <- struct{}{}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Println(scanner.Text())

		}
		if err := scanner.Err(); err != nil {
			fmt.Println("error reading stderr: ", err)
		}
		done <- struct{}{}
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("error waiting for command: %w", err)
	}

	<-done
	<-done

	return nil
}
