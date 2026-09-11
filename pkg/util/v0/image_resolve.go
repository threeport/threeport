package v0

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Image repository and tag derive from the same env vars and git state
// so a build and an install name the same image. IMAGE_REPO and
// IMAGE_TAG win over derivation. Under GitHub Actions the repository
// is ghcr.io plus the lowercased owner; ghcr requires lowercase. A
// tag-triggered run uses the ref name as the tag. Locally the
// repository is the caller default and the tag is version.sha, naming
// the commit instead of a mutable base. The canonical tag does not
// read ARCH, so an exported ARCH cannot redirect a pull.

// ResolveImageRepo returns the image repository. IMAGE_REPO wins when
// set, so a caller can target any registry. Under GitHub Actions it
// is ghcr.io plus the lowercased owner; otherwise it is devDefault.
func ResolveImageRepo(devDefault string) string {
	// prefer IMAGE_REPO when set
	if repo := strings.TrimSpace(os.Getenv("IMAGE_REPO")); repo != "" {
		return repo
	}
	// derive the ghcr namespace from the Actions owner
	if os.Getenv("GITHUB_ACTIONS") != "" {
		return "ghcr.io/" + strings.ToLower(os.Getenv("GITHUB_REPOSITORY_OWNER"))
	}
	// fall back to the supplied default
	return devDefault
}

// ResolveImageTag returns the canonical image tag with no architecture
// suffix. IMAGE_TAG wins when set. A GitHub Actions tag build returns
// the ref name. Otherwise the tag is versionDefault.sha, an empty
// repoDir reads the process working directory, and outside Actions a
// failed sha read returns versionDefault with no error.
func ResolveImageTag(repoDir, versionDefault string) (string, error) {
	// prefer IMAGE_TAG when set
	if tag := strings.TrimSpace(os.Getenv("IMAGE_TAG")); tag != "" {
		return tag, nil
	}
	// derive a local tag from HEAD
	if os.Getenv("GITHUB_ACTIONS") == "" {
		// warn that a dirty tree's sha tag does not name this code
		warnIfDirtyOnce(repoDir)
		// read the short sha
		sha, err := gitShortSha(repoDir)
		// use the bare version when the sha cannot be read
		if err != nil {
			return versionDefault, nil
		}
		return joinImageTag(versionDefault, sha), nil
	}
	// echo the pushed ref name on a tag build
	if os.Getenv("GITHUB_REF_TYPE") == "tag" {
		return os.Getenv("GITHUB_REF_NAME"), nil
	}
	// suffix the version with the short sha
	sha, err := gitShortSha(repoDir)
	if err != nil {
		return "", err
	}
	return joinImageTag(versionDefault, sha), nil
}

// buildImageTag returns the tag used to push an image. A single-arch
// ARCH value appends -<arch> so each arch is a distinct tag. A
// comma-list names a multi-arch push under the canonical tag, and an
// empty ARCH is left bare too.
func buildImageTag(repoDir, versionDefault string) (string, error) {
	// start from the canonical tag
	tag, err := ResolveImageTag(repoDir, versionDefault)
	if err != nil {
		return "", err
	}
	// append -<arch> when ARCH names one architecture
	if arch := strings.TrimSpace(os.Getenv("ARCH")); arch != "" && !strings.Contains(arch, ",") {
		return tag + "-" + arch, nil
	}
	return tag, nil
}

// ResolveImageCoordinates returns the image repository and the tag
// used to push, including a single-arch suffix when ARCH names one.
func ResolveImageCoordinates(repoDir, devRepo, versionDefault string) (repo, tag string, err error) {
	// resolve the push tag
	tag, err = buildImageTag(repoDir, versionDefault)
	if err != nil {
		return "", "", err
	}
	// resolve the repository against the supplied default
	return ResolveImageRepo(devRepo), tag, nil
}

// joinImageTag joins a version and a short sha into version.sha.
func joinImageTag(version, sha string) string {
	return version + "." + sha
}

// gitShortSha returns the seven-character HEAD sha of repoDir.
func gitShortSha(repoDir string) (string, error) {
	// read HEAD as a seven-character sha
	out, err := gitCommand(repoDir, "rev-parse", "--short=7", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to read short commit sha: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitCommand returns a git command that runs in repoDir when set.
// An empty repoDir omits -C and reads the process working directory.
func gitCommand(repoDir string, args ...string) *exec.Cmd {
	// prefix -C so git runs in repoDir rather than the process directory
	if repoDir != "" {
		args = append([]string{"-C", repoDir}, args...)
	}
	return exec.Command("git", args...)
}

// dirtyWarnOnce limits the dirty-tree warning to one print per process
// so a multi-component build does not repeat it.
var dirtyWarnOnce sync.Once

// warnIfDirtyOnce prints a warning on stderr the first time repoDir
// has uncommitted source. go.mod and go.sum do not count; a local
// replace directive keeps them modified. Any other dirty file means
// the sha tag names HEAD, not the code compiled into the image.
func warnIfDirtyOnce(repoDir string) {
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
