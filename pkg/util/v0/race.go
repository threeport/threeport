// Race tests live in *_race_test.go files tagged //go:build race so mage test:unit skips them.
// mage test:race runs go test -race only on those packages instead of instrumenting the tree.
package v0

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const raceTestSuffix = "_race_test.go"

// RaceTestPackages returns module-relative packages that contain a *_race_test.go file.
// The filename is the finder; Go does not treat it as special, so the build tag still applies.
func RaceTestPackages(root string) ([]string, error) {
	// collect unique package dirs that hold a *_race_test.go file
	seen := map[string]struct{}{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", "vendor", "node_modules", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, raceTestSuffix) {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := "./" + filepath.ToSlash(rel)
		if rel == "." {
			pkg = "."
		}
		seen[pkg] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk %s for race tests: %w", root, err)
	}
	pkgs := make([]string, 0, len(seen))
	for p := range seen {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	return pkgs, nil
}

// RunUnitTests runs go test -count=1 across pkg, internal, and cmd.
func RunUnitTests() error {
	if err := RunCommandStreamOutput(
		"go",
		"test",
		"-count=1",
		"./pkg/...",
		"./internal/...",
		"./cmd/...",
	); err != nil {
		return fmt.Errorf("failed to run unit tests: %w", err)
	}
	return nil
}

// RunRaceTests runs go test -race on packages that contain *_race_test.go files.
func RunRaceTests() error {
	// no-op when the tree has no race tests
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	pkgs, err := RaceTestPackages(root)
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		fmt.Println("no *_race_test.go files")
		return nil
	}
	args := append([]string{"test", "-race", "-count=1"}, pkgs...)
	if err := RunCommandStreamOutput("go", args...); err != nil {
		return fmt.Errorf("failed to run race tests: %w", err)
	}
	return nil
}
