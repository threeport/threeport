/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

const (
	threeportRepoUrl         = "https://api.github.com/repos/threeport/threeport"
	threeportGoModOutputPath = "/tmp/threeport_go.mod"
)

var (
	threeportVersion   string
	threeportGoModPath string
	extensionGoModPath string
)

// syncCmd represents the syncCmd command
var syncCmd = &cobra.Command{
	Use: "sync",
	Example: `  # sync dependency versions with the latest release of Threeport
  threeport-sdk sync

  # sync dependency versions with a specific version of Threeport
  threeport-sdk sync -v v0.5.3

  # sync dependency versions with a Threeport repo go.mod file on local filesystem
  threeport-sdk sync -t /path/to/threeport/repo/go.mod`,
	Short: "Sync the import dependencies with Threeport.",
	Long: `Sync the import dependencies with Threeport.

Plugins to tptctl must share the same import dependency versions with Threeeport.
The sync command sets the direct dependency versions to match the core Threeport
project and runs 'go mod tidy'.

Without the dependency versions sync'd, tptctl will fail to load the plugin with
an error that says 'plugin was built with a different version of package [package].'

Run this command when you have completed the source code for your plugin but before
you build the plugin.

See the Threeport SDK docs for more information: https://threeport.io/sdk/sdk-intro/
`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := validateFlags(threeportVersion, threeportGoModPath); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		// set or remove the replace statement based on whether a local path to
		// the Threeport project go.mod file is provided
		if err := updateThreeportDep(extensionGoModPath, threeportGoModPath); err != nil {
			cli.Error("failed to update replace statement in go.mod file", err)
			os.Exit(1)
		}

		// if path to Threeport project's go.mod file not provided, fetch from
		// GitHub
		if threeportGoModPath == "" {
			if err := fetchGoMod(threeportVersion); err != nil {
				cli.Error("failed to fetch go.mod for Threeport project", err)
				os.Exit(1)
			}

			threeportGoModPath = threeportGoModOutputPath
		}

		// run `go mod tidy` to set initial module requirements
		if err := runGoModTidy(extensionGoModPath); err != nil {
			cli.Error("failed to run go mod tidy", err)
			os.Exit(1)
		}

		// load dependency maps
		threeportDeps, err := loadDirectDependencies(threeportGoModPath)
		if err != nil {
			cli.Error("failed to load Threeport go.mod", err)
			os.Exit(1)
		}

		// update extension go.mod
		if err := updateExtensionGoMod(
			extensionGoModPath,
			threeportDeps,
			//threeportGoModPath,
		); err != nil {
			cli.Error("failed to update extension go.mod", err)
			os.Exit(1)
		}

		// re-run `go mod tidy` with updated dependency versions
		if err := runGoModTidy(extensionGoModPath); err != nil {
			cli.Error("failed to run go mod tidy", err)
			os.Exit(1)
		}

		cli.Complete("Extension go.mod updated successfully!")
	},
}

func init() {
	RootCmd.AddCommand(syncCmd)

	syncCmd.Flags().StringVarP(
		&threeportVersion,
		"threeport-version", "v", "", "The version of Threeport to sync this extension with.",
	)
	syncCmd.Flags().StringVarP(
		&threeportGoModPath,
		"threeport-mod", "t", "", "Path to core Threeport go.mod file.",
	)
	syncCmd.Flags().StringVarP(
		&extensionGoModPath,
		"extension-mod", "e", "./go.mod", "Path to this Threeport extension repo's go.mod file.",
	)
}

// validateFlags validates the flags provided by the user.
func validateFlags(threeportVersion, threeportGoModPath string) error {
	if threeportVersion != "" && threeportGoModPath != "" {
		return errors.New("cannot provide BOTH --threeport-version and --threeport-mod flags; provide only one or none")
	}

	return nil
}

// fetchGoMod retrieves the go.mod file from the Threeport GitHub repository.
func fetchGoMod(version string) error {
	// if no version supplied, use latest version
	if version == "" {
		version = "latest"
	}

	var goModUrl string
	if version == "latest" {
		// fetch the latest release
		latestReleaseURL := fmt.Sprintf("%s/releases/latest", threeportRepoUrl)
		resp, err := http.Get(latestReleaseURL)
		if err != nil {
			return fmt.Errorf("failed to fetch latest release: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to fetch latest release, status: %s", resp.Status)
		}

		var releaseData struct {
			TagName string `json:"tag_name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&releaseData); err != nil {
			return fmt.Errorf("failed to parse release data: %w", err)
		}

		version = releaseData.TagName
	}

	// construct the URL for the go.mod file
	goModUrl = fmt.Sprintf("https://raw.githubusercontent.com/threeport/threeport/%s/go.mod", version)

	// fetch Threeport's go.mod file
	resp, err := http.Get(goModUrl)
	if err != nil {
		return fmt.Errorf("failed to fetch go.mod file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch go.mod file, status: %s", resp.Status)
	}

	// write the go.mod file to the specified output path
	outFile, err := os.Create(threeportGoModOutputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write go.mod file: %w", err)
	}

	cli.Info(fmt.Sprintf("Threeport project's go.mod file saved to %s\n", threeportGoModOutputPath))

	return nil
}

// updateThreeportDep updates or removes the replace statement for Threeport in
// the go.mod file and removes the dependency line for github.com/threeport/threeport
// regardless of its version.
func updateThreeportDep(extensionGoModPath, threeportGoModPath string) error {
	// Read the content of the extension's go.mod file
	content, err := os.ReadFile(extensionGoModPath)
	if err != nil {
		return fmt.Errorf("failed to read go.mod file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var buffer bytes.Buffer
	replaceDirective := fmt.Sprintf("replace github.com/threeport/threeport => %s", strings.TrimSuffix(threeportGoModPath, "/go.mod"))
	replaceFound := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip the line if it declares the dependency on github.com/threeport/threeport
		if strings.HasPrefix(trimmed, "github.com/threeport/threeport v") {
			continue
		}

		// Check for existing replace directive
		if strings.HasPrefix(trimmed, "replace github.com/threeport/threeport =>") {
			replaceFound = true
			// If the Threeport path is empty, skip this line to remove the replace directive
			if threeportGoModPath == "" {
				continue
			}
		}

		buffer.WriteString(line + "\n")
	}

	// If the Threeport path is not empty and no replace directive exists, add it
	if threeportGoModPath != "" && !replaceFound {
		buffer.WriteString(replaceDirective + "\n")
	}

	// Write the updated content back to the go.mod file
	err = os.WriteFile(extensionGoModPath, buffer.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("failed to write updated go.mod file: %w", err)
	}

	fmt.Println("go.mod file updated successfully")
	return nil
}

// loadDirectDependencies parses the go.mod file and extracts direct dependencies.
func loadDirectDependencies(filepath string) (map[string]string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	deps := make(map[string]string)
	scanner := bufio.NewScanner(file)
	inRequire := false

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		if line == "require (" {
			inRequire = true
			continue
		}

		if line == ")" {
			inRequire = false
			continue
		}

		if inRequire && !strings.Contains(line, "// indirect") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				deps[parts[0]] = parts[1]
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return deps, nil
}

// updateExtensionGoMod updates the extension go.mod file with synchronized versions.
func updateExtensionGoMod(filepath string, threeportDeps map[string]string) error {
	file, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(file), "\n")
	var buffer bytes.Buffer
	inRequire := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "require (" {
			inRequire = true
			buffer.WriteString(line + "\n")
			continue
		}

		if trimmed == ")" {
			inRequire = false
			buffer.WriteString(line + "\n")
			continue
		}

		if inRequire && strings.Contains(trimmed, "// indirect") {
			// skip indirect dependencies
			continue
		}

		if inRequire {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				if version, exists := threeportDeps[parts[0]]; exists {
					buffer.WriteString(fmt.Sprintf("\t%s %s\n", parts[0], version))
					continue
				}
			}
		}

		buffer.WriteString(line + "\n")
	}

	// write updated go.mod
	if err := os.WriteFile(filepath, buffer.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write go.mod file: %w", err)
	}
	return nil
}

// runGoModTidy runs `go mod tidy` in the directory of the go.mod file.
func runGoModTidy(goModPath string) error {
	dir := strings.TrimSuffix(goModPath, "/go.mod")
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
