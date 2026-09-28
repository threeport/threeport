package v0

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// IMAGE_TAG wins over derivation. A tag-triggered GitHub Actions run
// uses the ref name as the tag. Locally the tag is version.sha, naming
// the commit instead of a mutable base. The tag does not read ARCH, so
// an exported ARCH cannot redirect a pull.

// ResolveImageTag returns the image tag and does not append ARCH.
// A non-blank IMAGE_TAG wins, a GitHub Actions tag build returns the ref name, and otherwise the tag is versionDefault.sha.
// Outside Actions a failed sha read returns versionDefault and a nil error; inside Actions the error is returned.
func ResolveImageTag(repoDir, versionDefault string) (string, error) {
	// prefer IMAGE_TAG when non-blank
	if tag := strings.TrimSpace(os.Getenv("IMAGE_TAG")); tag != "" {
		return tag, nil
	}

	// derive a local tag when not in GitHub Actions
	if os.Getenv("GITHUB_ACTIONS") == "" {
		// warn when uncommitted source is not go.mod or go.sum
		warnIfDirtyOnce(repoDir)

		// read the short sha
		sha, err := gitShortSha(repoDir)

		// use the bare version when the sha cannot be read
		if err != nil {
			return versionDefault, nil
		}
		return joinImageTag(versionDefault, sha), nil
	}

	// echo the ref name on a tag build
	if os.Getenv("GITHUB_REF_TYPE") == "tag" {
		return os.Getenv("GITHUB_REF_NAME"), nil
	}

	// suffix the version with the short sha, or return the git error
	sha, err := gitShortSha(repoDir)
	if err != nil {
		return "", err
	}
	return joinImageTag(versionDefault, sha), nil
}

// joinImageTag joins a version and a short sha into version.sha.
func joinImageTag(version, sha string) string {
	return version + "." + sha
}

// gitShortSha returns the abbreviated HEAD sha of repoDir, at least seven characters.
func gitShortSha(repoDir string) (string, error) {
	// read the abbreviated HEAD sha
	out, err := gitCommand(repoDir, "rev-parse", "--short=7", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to read short commit sha: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitCommand returns a git command that runs in repoDir when set.
// An empty repoDir omits -C.
func gitCommand(repoDir string, args ...string) *exec.Cmd {
	// prefix -C so git runs in repoDir rather than the process directory
	if repoDir != "" {
		args = append([]string{"-C", repoDir}, args...)
	}
	return exec.Command("git", args...)
}

// dirtyWarnOnce limits the dirty-tree warning to one print per process
// so a later resolve does not print it again.
var dirtyWarnOnce sync.Once

// warnIfDirtyOnce prints at most one stderr warning per process, on the first call.
// go.mod and go.sum do not count. A git status failure skips the warning.
func warnIfDirtyOnce(repoDir string) {
	// warn at most once per process
	dirtyWarnOnce.Do(func() {
		// list uncommitted paths
		out, err := gitCommand(repoDir, "status", "--porcelain").Output()

		// skip the warning when status cannot be read
		if err != nil {
			return
		}

		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}

			// take the last field, the path or rename destination
			path := fields[len(fields)-1]

			// skip go.mod and go.sum
			if path == "go.mod" || path == "go.sum" {
				continue
			}

			// print the warning and stop scanning
			fmt.Fprintln(os.Stderr, "warning: building a dev image from a tree with uncommitted code; commit first so the .<sha> tag names the exact code (go.mod / go.sum excluded)")
			return
		}
	})
}

// ImageWithoutTag returns the image reference with a :tag stripped.
// A digest is left alone.
func ImageWithoutTag(image string) string {
	// leave a digest reference unchanged
	if strings.Contains(image, "@") {
		return image
	}

	// a colon after the last slash is a tag, not a registry port
	colon := strings.LastIndex(image, ":")
	if colon < 0 || colon < strings.LastIndex(image, "/") {
		return image
	}

	// drop the tag
	return image[:colon]
}
