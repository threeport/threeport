package v0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A Head build is an untagged build of a commit on a release branch. Its
// version is <contents of version.txt>.<first 7 characters of the commit sha>
// and that one string is the tptctl version, the image tag, the REST API
// /version response and the image version label. See docs/dev/head-builds.md.

// HeadCommitEnv is the environment variable that stamps a Head build. When it
// is set to a 7-character short sha, binaries are linked with it as
// internal/version.Commit. When it is unset, builds are unchanged.
const HeadCommitEnv = "HEAD_COMMIT"

// versionFilePath is the version file relative to the repo root.
const versionFilePath = "internal/version/version.txt"

// versionPackageDir is the version package relative to the repo root.
const versionPackageDir = "./internal/version"

const (
	// labelRevision is the OCI label that names the commit an image was built from.
	labelRevision = "org.opencontainers.image.revision"

	// dispatchClockSkew is how far before the local dispatch time a run may
	// have been created and still count as ours, so skew between this machine
	// and GitHub cannot hide the run.
	dispatchClockSkew = 30 * time.Second
)

var (
	// shortShaPattern matches the short sha stamped into a Head build.
	shortShaPattern = regexp.MustCompile(`^[0-9a-f]{7}$`)

	// fullShaPattern matches a full commit sha.
	fullShaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

	// releaseBranchPattern matches a release branch name such as 0.7.
	releaseBranchPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
)

// ShortSha returns the first 7 characters of a full commit sha. It truncates
// the string and does not ask git to abbreviate: git rev-parse --short=7
// returns at least 7 characters, and more when a prefix is ambiguous, so the
// same commit could get different tags at different times.
func ShortSha(fullSha string) (string, error) {
	// reject anything that is not a full lowercase hex sha
	if !fullShaPattern.MatchString(fullSha) {
		return "", fmt.Errorf("commit %q is not a full 40-character lowercase hex sha", fullSha)
	}
	return fullSha[:7], nil
}

// HeadVersion returns the Head version for a commit: the version with its
// surrounding whitespace trimmed, a dot, and the short sha of fullSha.
func HeadVersion(version, fullSha string) (string, error) {
	// require a version to append to
	version = strings.TrimSpace(version)
	if version == "" {
		return "", fmt.Errorf("version is empty")
	}

	// truncate the sha
	short, err := ShortSha(fullSha)
	if err != nil {
		return "", err
	}
	return version + "." + short, nil
}

// ReadVersionFile returns the contents of internal/version/version.txt in
// repoDir with whitespace trimmed. Head versions are built from the file on
// disk rather than from the version package compiled into the caller, because
// mage caches its compiled magefile binary and does not rebuild it when a
// package it imports changes.
func ReadVersionFile(repoDir string) (string, error) {
	path := filepath.Join(repoDir, versionFilePath)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// VersionLdflags returns the go build flags that stamp the Head commit into
// the version package of the module in repoDir. It returns nil when
// HEAD_COMMIT is unset or empty, so RC and OSR builds are unchanged. It
// returns an error when HEAD_COMMIT is set to anything but 7 lowercase hex
// characters, so a bad value never silently produces an unstamped build.
// The package path comes from go list so that it is right for any module and
// in workspace mode.
func VersionLdflags(repoDir string) ([]string, error) {
	// leave builds alone when no commit is set
	commit := os.Getenv(HeadCommitEnv)
	if commit == "" {
		return nil, nil
	}
	if !shortShaPattern.MatchString(commit) {
		return nil, fmt.Errorf(
			"%s=%q is invalid: it must be exactly 7 lowercase hex characters",
			HeadCommitEnv, commit,
		)
	}

	// resolve the import path of the version package
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}", versionPackageDir)
	cmd.Dir = repoDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to find the version package in %q (%s is set): %w: %s",
			repoDir, HeadCommitEnv, err, strings.TrimSpace(stderr.String()),
		)
	}
	pkg := strings.TrimSpace(string(out))
	if pkg == "" || strings.ContainsAny(pkg, " \t\r\n") {
		return nil, fmt.Errorf("go list returned an unexpected version package path %q", pkg)
	}

	return []string{fmt.Sprintf("-ldflags=-X=%s.Commit=%s", pkg, commit)}, nil
}

// WithEnv runs fn with the environment variable key set to value, then
// restores the previous value, or unsets the variable if it was not set.
func WithEnv(key, value string, fn func() error) error {
	// remember the current value
	previous, wasSet := os.LookupEnv(key)

	if err := os.Setenv(key, value); err != nil {
		return fmt.Errorf("failed to set %s: %w", key, err)
	}

	// put the environment back however fn returns
	defer func() {
		if wasSet {
			_ = os.Setenv(key, previous)
		} else {
			_ = os.Unsetenv(key)
		}
	}()

	return fn()
}

// IsReleaseBranch reports whether name looks like a release branch, e.g. 0.7.
func IsReleaseBranch(name string) bool {
	return releaseBranchPattern.MatchString(name)
}

// RequireTools returns an error naming every tool that is not on PATH.
func RequireTools(tools ...string) error {
	// collect each missing tool
	var missing []string
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required tools not found on PATH: %s", strings.Join(missing, ", "))
	}
	return nil
}

// RequireDockerBuildx returns an error when docker has no buildx plugin.
func RequireDockerBuildx() error {
	if out, err := exec.Command("docker", "buildx", "version").CombinedOutput(); err != nil {
		return fmt.Errorf("docker buildx is required: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RequireGhAuth returns an error when gh is not logged in.
func RequireGhAuth() error {
	if out, err := exec.Command("gh", "auth", "status").CombinedOutput(); err != nil {
		return fmt.Errorf(
			"gh is not authenticated, run 'gh auth login': %w: %s",
			err, strings.TrimSpace(string(out)),
		)
	}
	return nil
}

// GitFullSha returns the full sha of HEAD in repoDir.
func GitFullSha(repoDir string) (string, error) {
	out, err := gitCommand(repoDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to read the HEAD commit: %w", err)
	}
	sha := strings.TrimSpace(string(out))
	if !fullShaPattern.MatchString(sha) {
		return "", fmt.Errorf("git returned %q for HEAD, expected a full 40-character sha", sha)
	}
	return sha, nil
}

// GitCurrentBranch returns the checked out branch of repoDir, or an empty
// string when HEAD is detached.
func GitCurrentBranch(repoDir string) (string, error) {
	out, err := gitCommand(repoDir, "branch", "--show-current").Output()
	if err != nil {
		return "", fmt.Errorf("failed to read the current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RequireReleaseBranch returns an error unless repoDir has a release branch
// checked out, and returns the branch name when it does.
func RequireReleaseBranch(repoDir string) (string, error) {
	branch, err := GitCurrentBranch(repoDir)
	if err != nil {
		return "", err
	}
	if !IsReleaseBranch(branch) {
		shown := branch
		if shown == "" {
			shown = "a detached HEAD"
		}
		return "", fmt.Errorf(
			"%s is not a release branch (expected a name like 0.7): check out the release branch",
			shown,
		)
	}
	return branch, nil
}

// RequireCleanTree returns an error when repoDir has any uncommitted or
// untracked change. There are no exceptions, so that a tptctl built locally is
// built from exactly the commit CI builds.
func RequireCleanTree(repoDir string) error {
	out, err := gitCommand(repoDir, "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("failed to read the git status: %w", err)
	}
	if status := strings.TrimRight(string(out), "\n"); status != "" {
		return fmt.Errorf(
			"the working tree is not clean, commit or remove these changes first: a Head build must be exactly the commit CI builds\n%s",
			status,
		)
	}
	return nil
}

// GitUpstreamHead returns the sha at the head of branch in the repository at
// upstreamURL. It reads the URL directly and never uses a configured remote,
// because on a fork origin is the fork and upstream may have any name or none.
func GitUpstreamHead(upstreamURL, branch string) (string, error) {
	ref := "refs/heads/" + branch
	out, err := exec.Command("git", "ls-remote", upstreamURL, ref).Output()
	if err != nil {
		return "", fmt.Errorf("failed to read %s from %s: %w", ref, upstreamURL, err)
	}
	sha, ok := parseLsRemote(string(out), ref)
	if !ok {
		return "", fmt.Errorf("branch %s was not found at %s", branch, upstreamURL)
	}
	return sha, nil
}

// parseLsRemote returns the sha listed for exactly ref in git ls-remote output.
func parseLsRemote(output, ref string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == ref && fullShaPattern.MatchString(fields[0]) {
			return fields[0], true
		}
	}
	return "", false
}

// RequireAtUpstreamHead returns the full sha of HEAD in repoDir, or an error
// unless it equals the head of branch at upstreamURL.
func RequireAtUpstreamHead(repoDir, upstreamURL, branch string) (string, error) {
	local, err := GitFullSha(repoDir)
	if err != nil {
		return "", err
	}
	remote, err := GitUpstreamHead(upstreamURL, branch)
	if err != nil {
		return "", err
	}
	if local != remote {
		return "", fmt.Errorf(
			"local HEAD %s is not at the head of %s on %s (%s):\n"+
				"  - if your checkout is behind, pull from upstream and run this again\n"+
				"  - if it is ahead or has diverged, Head builds only come from commits merged upstream",
			local[:7], branch, upstreamURL, remote[:7],
		)
	}
	return local, nil
}

// imageConfig is the part of an OCI image config that Head checks read.
type imageConfig struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Config       struct {
		Labels map[string]string `json:"Labels"`
	} `json:"config"`
}

// ParseImageLabels parses the output of
// docker buildx imagetools inspect --format '{{json .Image}}' into the image
// labels of each platform. A multi-arch image prints a map from platform
// (linux/amd64) to image config, and a single-platform image prints one config.
func ParseImageLabels(data []byte) (map[string]map[string]string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse image config: %w", err)
	}

	labels := map[string]map[string]string{}

	// a single-platform image prints its config directly
	if _, ok := raw["config"]; ok {
		var cfg imageConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse image config: %w", err)
		}
		labels[cfg.OS+"/"+cfg.Architecture] = cfg.Config.Labels
		return labels, nil
	}

	// a multi-arch image prints one config per platform
	for platform, rawCfg := range raw {
		var cfg imageConfig
		if err := json.Unmarshal(rawCfg, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse image config for %s: %w", platform, err)
		}
		labels[platform] = cfg.Config.Labels
	}
	if len(labels) == 0 {
		return nil, fmt.Errorf("image has no config")
	}
	return labels, nil
}

// CheckImageRevision returns an error unless labels has an image for linux on
// every arch, each with a revision label equal to fullSha. Platforms beyond
// arches, such as attestations, are ignored. image names the image in errors.
func CheckImageRevision(
	image string,
	labels map[string]map[string]string,
	arches []string,
	fullSha string,
) error {
	var problems MultiError
	for _, arch := range arches {
		platform := "linux/" + arch

		// the platform must be in the image
		platformLabels, ok := labels[platform]
		if !ok {
			problems.AppendError(fmt.Errorf("%s has no %s image", image, platform))
			continue
		}

		// and built from exactly this commit
		if revision := platformLabels[labelRevision]; revision != fullSha {
			problems.AppendError(fmt.Errorf(
				"%s (%s) was built from revision %q, not %s",
				image, platform, revision, fullSha,
			))
		}
	}
	return problems.Error()
}

// InspectImageLabels reads the image labels of every platform of image from
// its registry.
func InspectImageLabels(image string) (map[string]map[string]string, error) {
	cmd := exec.Command("docker", "buildx", "imagetools", "inspect", image, "--format", "{{json .Image}}")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("could not inspect %s: %w: %s", image, err, strings.TrimSpace(stderr.String()))
	}
	labels, err := ParseImageLabels(out)
	if err != nil {
		return nil, fmt.Errorf("could not read the labels of %s: %w", image, err)
	}
	return labels, nil
}

// CheckHeadImages returns an error unless every image in imageNames exists at
// <imageRepo>/<name>:<tag> for each of arches, built from the commit fullSha.
// Images are checked in parallel and every problem is reported.
func CheckHeadImages(
	imageRepo string,
	imageNames []string,
	tag string,
	fullSha string,
	arches []string,
	parallel int,
) error {
	tasks := make([]func() error, 0, len(imageNames))
	for _, name := range imageNames {
		image := fmt.Sprintf("%s/%s:%s", imageRepo, name, tag)
		tasks = append(tasks, func() error {
			labels, err := InspectImageLabels(image)
			if err != nil {
				return err
			}
			return CheckImageRevision(image, labels, arches, fullSha)
		})
	}
	return RunParallel(parallel, tasks)
}

// WorkflowRun is a GitHub Actions workflow run as listed by gh run list.
type WorkflowRun struct {
	DatabaseID   int64     `json:"databaseId"`
	DisplayTitle string    `json:"displayTitle"`
	URL          string    `json:"url"`
	CreatedAt    time.Time `json:"createdAt"`
}

// ParseWorkflowRuns parses gh run list --json
// databaseId,displayTitle,url,createdAt output.
func ParseWorkflowRuns(data []byte) ([]WorkflowRun, error) {
	var runs []WorkflowRun
	if err := json.Unmarshal(data, &runs); err != nil {
		return nil, fmt.Errorf("failed to parse workflow runs: %w", err)
	}
	return runs, nil
}

// SelectDispatchedRun returns the newest run whose title contains fullSha and
// that was created at or after notBefore.
func SelectDispatchedRun(runs []WorkflowRun, fullSha string, notBefore time.Time) (WorkflowRun, bool) {
	var found WorkflowRun
	ok := false
	for _, run := range runs {
		// skip other commits and older runs
		if !strings.Contains(run.DisplayTitle, fullSha) || run.CreatedAt.Before(notBefore) {
			continue
		}
		if !ok || run.CreatedAt.After(found.CreatedAt) {
			found, ok = run, true
		}
	}
	return found, ok
}

// DispatchWorkflow starts a workflow_dispatch run of workflowFile on ref in
// the GitHub repository ghRepo, and returns the local time of the dispatch.
func DispatchWorkflow(ghRepo, workflowFile, ref string, inputs map[string]string) (time.Time, error) {
	args := []string{"workflow", "run", workflowFile, "--repo", ghRepo, "--ref", ref}

	// pass inputs in a stable order
	keys := make([]string, 0, len(inputs))
	for k := range inputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-f", k+"="+inputs[k])
	}

	dispatchedAt := time.Now()
	if out, err := exec.Command("gh", args...).CombinedOutput(); err != nil {
		return time.Time{}, fmt.Errorf(
			"failed to dispatch %s (write access to %s is required): %w: %s",
			workflowFile, ghRepo, err, strings.TrimSpace(string(out)),
		)
	}
	return dispatchedAt, nil
}

// FindDispatchedRun polls gh run list until a workflow_dispatch run of
// workflowFile for fullSha appears that was created after dispatchedAt, or
// timeout passes.
func FindDispatchedRun(
	ghRepo, workflowFile, fullSha string,
	dispatchedAt time.Time,
	timeout, interval time.Duration,
) (WorkflowRun, error) {
	notBefore := dispatchedAt.Add(-dispatchClockSkew)
	deadline := time.Now().Add(timeout)

	for {
		// list recent dispatched runs of the workflow
		out, err := exec.Command(
			"gh", "run", "list",
			"--repo", ghRepo,
			"--workflow", workflowFile,
			"--event", "workflow_dispatch",
			"--limit", "20",
			"--json", "databaseId,displayTitle,url,createdAt",
		).Output()
		if err != nil {
			return WorkflowRun{}, fmt.Errorf("failed to list runs of %s: %w", workflowFile, err)
		}
		runs, err := ParseWorkflowRuns(out)
		if err != nil {
			return WorkflowRun{}, err
		}
		if run, ok := SelectDispatchedRun(runs, fullSha, notBefore); ok {
			return run, nil
		}

		if time.Now().After(deadline) {
			return WorkflowRun{}, fmt.Errorf(
				"no %s run for %s appeared within %s: see https://github.com/%s/actions/workflows/%s",
				workflowFile, fullSha[:7], timeout, ghRepo, workflowFile,
			)
		}
		time.Sleep(interval)
	}
}

// WatchRun streams a workflow run to stdout until it finishes and returns an
// error if the run did not succeed.
func WatchRun(ghRepo string, runID int64) error {
	return RunCommandStreamOutput(
		"gh", "run", "watch", strconv.FormatInt(runID, 10),
		"--repo", ghRepo,
		"--exit-status",
	)
}
