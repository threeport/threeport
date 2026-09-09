package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	mg "github.com/magefile/mage/mg"
)

// Release provides a type for methods that cut tagged releases.
type Release mg.Namespace

// baseVersionPattern matches a bare X.Y.Z release version.
var baseVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// versionFile holds the version string read by imagetag and the release
// targets, relative to the repo root mage runs from.
const versionFile = "internal/version/version.txt"

// Imagetag prints the image tag for the current build: the git tag itself
// on a tag build, otherwise the version file's base joined to the short
// commit sha as v<base>-dev.<sha>.
func Imagetag() error {
	sha, err := gitOutput("rev-parse", "--short=7", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to read short commit sha: %w", err)
	}
	v, err := readVersion()
	if err != nil {
		return err
	}
	fmt.Println(joinImageTag(os.Getenv("GITHUB_REF_TYPE"), os.Getenv("GITHUB_REF_NAME"), v, sha))
	return nil
}

// joinImageTag returns refName on a tag build, otherwise version joined to
// sha with a dot, yielding v<base>-dev.<sha>.
func joinImageTag(refType, refName, version, sha string) string {
	if refType == "tag" {
		return refName
	}
	return version + "." + sha
}

// readVersion returns the trimmed contents of the version file.
func readVersion() (string, error) {
	contents, err := os.ReadFile(versionFile)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", versionFile, err)
	}
	return strings.TrimSpace(string(contents)), nil
}

// baseFromVersion derives the bare X.Y.Z base from a version string by
// stripping a leading v and any prerelease suffix from the first hyphen.
func baseFromVersion(version string) string {
	version = strings.TrimPrefix(version, "v")
	if i := strings.Index(version, "-"); i >= 0 {
		version = version[:i]
	}
	return version
}

// Dev cuts the next dev build under the version file's base, tagging the
// latest dev commit as v<base>-dev.<next N> and pushing the tag.
func (Release) Dev() error {
	return cutRelease("dev", false)
}

// Rc cuts the next release candidate under the version file's base, tagging
// the latest dev commit as v<base>-rc.<next N> and pushing the tag.
func (Release) Rc() error {
	return cutRelease("rc", false)
}

// Ga cuts the general-availability release for the version file's base,
// tagging the latest dev commit as v<base> and pushing the tag.
func (Release) Ga() error {
	return cutRelease("", true)
}

// cutRelease tags the latest pushed dev commit as a release and pushes the
// tag, which triggers the release and image workflows. The base comes from
// the version file. A channel build auto-increments the per-channel counter
// under the base; a ga build tags the bare base. The push remote resolves to
// the RELEASE_REMOTE env override, otherwise to the remote the local dev
// branch tracks, otherwise to origin.
func cutRelease(channel string, ga bool) error {
	fileVersion, err := readVersion()
	if err != nil {
		return err
	}
	base, err := validateBase(baseFromVersion(fileVersion))
	if err != nil {
		return err
	}

	remote := resolveReleaseRemote()

	// fetch first so the counter and the tag target see the latest pushed dev
	if err := git("fetch", remote, "dev", "--tags"); err != nil {
		return fmt.Errorf("failed to fetch %s: %w", remote, err)
	}

	// a channel build bumps its per-channel counter; a ga build needs none
	next := 0
	if !ga {
		next, err = nextCounter(base, channel)
		if err != nil {
			return fmt.Errorf("failed to compute next %s counter: %w", channel, err)
		}
	}
	version := formatVersion(base, channel, ga, next)

	// refuse to reuse an existing tag
	if exec.Command("git", "rev-parse", "-q", "--verify", "refs/tags/"+version).Run() == nil {
		return fmt.Errorf("tag %s already exists", version)
	}

	// tag the latest pushed dev head; the tag push is the release event
	if err := git("tag", "-a", version, remote+"/dev", "-m", "release "+version); err != nil {
		return fmt.Errorf("failed to tag %s: %w", version, err)
	}
	if err := git("push", remote, version); err != nil {
		return fmt.Errorf("failed to push %s: %w", version, err)
	}

	fmt.Printf("pushed %s via %s\n", version, remote)
	return nil
}

// validateBase strips a leading v and reports whether the remainder is a
// bare X.Y.Z release version, returning the cleaned base.
func validateBase(base string) (string, error) {
	base = strings.TrimPrefix(base, "v")
	if !baseVersionPattern.MatchString(base) {
		return "", fmt.Errorf("base must be X.Y.Z, got %q", base)
	}
	return base, nil
}

// formatVersion builds the tag string: the bare v<base> for a ga release,
// or v<base>-<channel>.<next> for a channel build.
func formatVersion(base, channel string, ga bool, next int) string {
	if ga {
		return "v" + base
	}
	return fmt.Sprintf("v%s-%s.%d", base, channel, next)
}

// nextCounter returns one more than the highest counter among the existing
// v<base>-<channel>.N tags, or 1 when none exist.
func nextCounter(base, channel string) (int, error) {
	out, err := gitOutput("tag", "--list", fmt.Sprintf("v%s-%s.*", base, channel))
	if err != nil {
		return 0, err
	}
	return highestCounter(strings.Fields(out), fmt.Sprintf("v%s-%s.", base, channel)) + 1, nil
}

// highestCounter returns the largest numeric N among tags shaped <prefix>N,
// or 0 when none match. It parses N numerically so dev.10 ranks above dev.2.
func highestCounter(tags []string, prefix string) int {
	highest := 0
	for _, tag := range tags {
		n, err := strconv.Atoi(strings.TrimPrefix(tag, prefix))
		if err != nil {
			continue
		}
		if n > highest {
			highest = n
		}
	}
	return highest
}

// resolveReleaseRemote picks the remote to fetch dev from and push the tag
// to. RELEASE_REMOTE overrides everything so a caller can force a specific
// remote; without it, the remote the local dev branch tracks fits any clone
// whose remote is not named origin (a fork, an internal mirror, a rename);
// origin is the final fallback for a fresh clone with no tracking set.
func resolveReleaseRemote() string {
	if r := os.Getenv("RELEASE_REMOTE"); r != "" {
		return r
	}
	if r, err := gitOutput("config", "--get", "branch.dev.remote"); err == nil && r != "" {
		return r
	}
	return "origin"
}

// git runs a git command and surfaces its combined output on failure.
func git(args ...string) error {
	if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return nil
}

// gitOutput runs a git command and returns its trimmed standard output.
func gitOutput(args ...string) (string, error) {
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
