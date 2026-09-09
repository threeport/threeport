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

// versionFile is the version path relative to the repo root mage runs from.
const versionFile = "internal/version/version.txt"

// Imagetag prints GITHUB_REF_NAME when GITHUB_REF_TYPE is tag, otherwise
// the version and short sha.
func Imagetag() error {
	// read the short commit sha
	sha, err := gitOutput("rev-parse", "--short=7", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to read short commit sha: %w", err)
	}
	// read the version file
	v, err := readVersion()
	if err != nil {
		return err
	}
	// print the joined image tag
	fmt.Println(joinImageTag(os.Getenv("GITHUB_REF_TYPE"), os.Getenv("GITHUB_REF_NAME"), v, sha))
	return nil
}

// joinImageTag returns the git tag name when refType is tag, otherwise
// version and sha joined with a dot.
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

// baseFromVersion returns the X.Y.Z core of a version, dropping a leading
// v and any prerelease suffix from the first hyphen.
func baseFromVersion(version string) string {
	// drop a leading v
	version = strings.TrimPrefix(version, "v")
	// drop a prerelease suffix from the first hyphen
	if i := strings.Index(version, "-"); i >= 0 {
		version = version[:i]
	}
	return version
}

// Dev cuts the next vX.Y.Z-dev.N tag on the latest pushed dev commit
// and pushes it.
func (Release) Dev() error {
	return cutRelease("dev", false)
}

// Rc cuts the next vX.Y.Z-rc.N tag on the latest pushed dev commit
// and pushes it.
func (Release) Rc() error {
	return cutRelease("rc", false)
}

// Ga cuts a vX.Y.Z tag on the latest pushed dev commit and pushes it.
func (Release) Ga() error {
	return cutRelease("", true)
}

// cutRelease tags the latest pushed dev commit and pushes the tag. A
// channel cut increments that channel's counter; a ga cut uses the bare base.
func cutRelease(channel string, ga bool) error {
	// read the version file
	fileVersion, err := readVersion()
	if err != nil {
		return err
	}
	// validate the X.Y.Z base
	base, err := validateBase(baseFromVersion(fileVersion))
	if err != nil {
		return err
	}

	// resolve the git remote to fetch from and push to
	remote := resolveReleaseRemote()

	// fetch remote dev and tags so the counter and tag target are current
	if err := git("fetch", remote, "dev", "--tags"); err != nil {
		return fmt.Errorf("failed to fetch %s: %w", remote, err)
	}

	// set the next channel counter, or 0 for ga
	next := 0
	if !ga {
		next, err = nextCounter(base, channel)
		if err != nil {
			return fmt.Errorf("failed to compute next %s counter: %w", channel, err)
		}
	}
	// format the release tag
	version := formatVersion(base, channel, ga, next)

	// reject a tag that already exists
	if exec.Command("git", "rev-parse", "-q", "--verify", "refs/tags/"+version).Run() == nil {
		return fmt.Errorf("tag %s already exists", version)
	}

	// tag the latest pushed dev head
	if err := git("tag", "-a", version, remote+"/dev", "-m", "release "+version); err != nil {
		return fmt.Errorf("failed to tag %s: %w", version, err)
	}
	// push the tag
	if err := git("push", remote, version); err != nil {
		return fmt.Errorf("failed to push %s: %w", version, err)
	}

	// report the pushed tag
	fmt.Printf("pushed %s via %s\n", version, remote)
	return nil
}

// validateBase returns a bare X.Y.Z base, stripping a leading v and
// rejecting any other form.
func validateBase(base string) (string, error) {
	// drop a leading v
	base = strings.TrimPrefix(base, "v")
	// reject anything that is not X.Y.Z
	if !baseVersionPattern.MatchString(base) {
		return "", fmt.Errorf("base must be X.Y.Z, got %q", base)
	}
	return base, nil
}

// formatVersion returns vX.Y.Z for a ga cut and vX.Y.Z-channel.N otherwise.
func formatVersion(base, channel string, ga bool, next int) string {
	if ga {
		return "v" + base
	}
	return fmt.Sprintf("v%s-%s.%d", base, channel, next)
}

// nextCounter returns one more than the highest existing tag counter for
// the base and channel, or 1 when none exist.
func nextCounter(base, channel string) (int, error) {
	// list existing tags for this base and channel
	out, err := gitOutput("tag", "--list", fmt.Sprintf("v%s-%s.*", base, channel))
	if err != nil {
		return 0, err
	}
	// add one to the highest existing counter
	return highestCounter(strings.Fields(out), fmt.Sprintf("v%s-%s.", base, channel)) + 1, nil
}

// highestCounter returns the largest integer N among tags shaped prefixN,
// or 0 if none parse.
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

// resolveReleaseRemote returns the remote used to fetch dev and push the
// tag: RELEASE_REMOTE, the remote tracking local dev, or origin.
func resolveReleaseRemote() string {
	// prefer RELEASE_REMOTE when set
	if r := os.Getenv("RELEASE_REMOTE"); r != "" {
		return r
	}
	// use the remote tracking local dev
	if r, err := gitOutput("config", "--get", "branch.dev.remote"); err == nil && r != "" {
		return r
	}
	// fall back to origin
	return "origin"
}

// git runs git with args and returns a combined-output error on failure.
func git(args ...string) error {
	if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to run git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(output)), err)
	}
	return nil
}

// gitOutput runs git with args and returns trimmed stdout.
func gitOutput(args ...string) (string, error) {
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
