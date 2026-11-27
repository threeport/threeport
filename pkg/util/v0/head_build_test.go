package v0

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	testFullSha  = "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"
	testShortSha = "a1b2c3d"
)

// TestShortShaTruncatesFullSha covers the first seven characters of a full
// sha, and rejects anything that is not a full lowercase hex sha.
func TestShortShaTruncatesFullSha(t *testing.T) {
	// assert the truncation
	got, err := ShortSha(testFullSha)
	if err != nil {
		t.Fatalf("ShortSha returned error: %v", err)
	}
	if got != testShortSha {
		t.Errorf("ShortSha = %q, want %q", got, testShortSha)
	}

	// assert each malformed sha is rejected
	for name, sha := range map[string]string{
		"empty":     "",
		"short":     testShortSha,
		"39 chars":  testFullSha[:39],
		"41 chars":  testFullSha + "0",
		"uppercase": strings.ToUpper(testFullSha),
		"non-hex":   strings.Repeat("g", 40),
	} {
		if _, err := ShortSha(sha); err == nil {
			t.Errorf("ShortSha accepted %s sha %q", name, sha)
		}
	}
}

// TestHeadVersion covers the Head version string and its input checks.
func TestHeadVersion(t *testing.T) {
	// assert the version, a dot, and the seven-character sha
	got, err := HeadVersion("v0.7.0-rc.6", testFullSha)
	if err != nil {
		t.Fatalf("HeadVersion returned error: %v", err)
	}
	if got != "v0.7.0-rc.6.a1b2c3d" {
		t.Errorf("HeadVersion = %q, want v0.7.0-rc.6.a1b2c3d", got)
	}

	// assert whitespace around the version is trimmed
	for _, version := range []string{"v0.7.0-rc.6\n", "v0.7.0-rc.6\r\n", "  v0.7.0-rc.6 \t\n"} {
		got, err := HeadVersion(version, testFullSha)
		if err != nil {
			t.Fatalf("HeadVersion(%q) returned error: %v", version, err)
		}
		if got != "v0.7.0-rc.6.a1b2c3d" {
			t.Errorf("HeadVersion(%q) = %q, want v0.7.0-rc.6.a1b2c3d", version, got)
		}
	}

	// assert a short, non-hex or empty sha is an error
	for _, sha := range []string{"", testShortSha, strings.Repeat("z", 40)} {
		if _, err := HeadVersion("v0.7.0-rc.6", sha); err == nil {
			t.Errorf("HeadVersion accepted sha %q", sha)
		}
	}

	// assert an empty version is an error
	if _, err := HeadVersion(" \n", testFullSha); err == nil {
		t.Error("HeadVersion accepted an empty version")
	}
}

// writeVersionModule writes a module with a version package to a temp dir and
// returns the directory.
func writeVersionModule(t *testing.T, modulePath string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "version"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":                      "module " + modulePath + "\n\ngo 1.21\n",
		"internal/version/version.go": "package version\n\nvar Commit string\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestVersionLdflagsReturnsNilWhenUnset covers an unset or empty HEAD_COMMIT
// leaving builds unchanged.
func TestVersionLdflagsReturnsNilWhenUnset(t *testing.T) {
	// the repo dir is never consulted when no commit is set
	missingDir := filepath.Join(t.TempDir(), "missing")

	// assert an empty value, as CI passes for a non-Head build, returns nil
	t.Setenv(HeadCommitEnv, "")
	if got, err := VersionLdflags(missingDir); err != nil || got != nil {
		t.Errorf("VersionLdflags with an empty value = %v, %v, want nil, nil", got, err)
	}

	// assert a variable that is not set at all returns nil
	os.Unsetenv(HeadCommitEnv)
	if got, err := VersionLdflags(missingDir); err != nil || got != nil {
		t.Errorf("VersionLdflags with no variable = %v, %v, want nil, nil", got, err)
	}
}

// TestVersionLdflagsStampsModulePath covers a valid commit producing the -X
// flag for the module's own version package.
func TestVersionLdflagsStampsModulePath(t *testing.T) {
	t.Setenv("GOWORK", "off")
	t.Setenv(HeadCommitEnv, testShortSha)

	// a module other than threeport gets its own path
	dir := writeVersionModule(t, "example.com/qtm")
	got, err := VersionLdflags(dir)
	if err != nil {
		t.Fatalf("VersionLdflags returned error: %v", err)
	}
	want := []string{"-ldflags=-X=example.com/qtm/internal/version.Commit=a1b2c3d"}
	if !slices.Equal(got, want) {
		t.Errorf("VersionLdflags = %v, want %v", got, want)
	}
}

// TestVersionLdflagsStampsThreeportModule covers the flag for this repo.
func TestVersionLdflagsStampsThreeportModule(t *testing.T) {
	t.Setenv(HeadCommitEnv, testShortSha)

	// this package is three directories below the repo root
	got, err := VersionLdflags(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("VersionLdflags returned error: %v", err)
	}
	want := []string{"-ldflags=-X=github.com/threeport/threeport/internal/version.Commit=a1b2c3d"}
	if !slices.Equal(got, want) {
		t.Errorf("VersionLdflags = %v, want %v", got, want)
	}
}

// TestVersionLdflagsRejectsInvalidCommit covers a bad HEAD_COMMIT being an
// error rather than an unstamped build.
func TestVersionLdflagsRejectsInvalidCommit(t *testing.T) {
	for _, value := range []string{
		"a1b2c3",    // too short
		"a1b2c3d4",  // too long
		"A1B2C3D",   // uppercase
		"a1b2c3g",   // not hex
		" a1b2c3d",  // padded
		testFullSha, // a full sha is not the short form
	} {
		t.Setenv(HeadCommitEnv, value)

		got, err := VersionLdflags(t.TempDir())
		if err == nil {
			t.Errorf("VersionLdflags accepted %q, returned %v", value, got)
			continue
		}
		if !strings.Contains(err.Error(), HeadCommitEnv) {
			t.Errorf("error for %q does not name %s: %v", value, HeadCommitEnv, err)
		}
	}
}

// TestVersionLdflagsErrorsWithoutVersionPackage covers a module with no
// version package being an error when a commit is set.
func TestVersionLdflagsErrorsWithoutVersionPackage(t *testing.T) {
	t.Setenv("GOWORK", "off")
	t.Setenv(HeadCommitEnv, testShortSha)

	// a module with no internal/version
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := VersionLdflags(dir); err == nil {
		t.Error("VersionLdflags succeeded for a module without a version package")
	}
}

// TestGoBuildArgsStampsOnlyWithLdflags covers the go build arguments
// carrying the stamp when given and being unchanged when not.
func TestGoBuildArgsStampsOnlyWithLdflags(t *testing.T) {
	t.Setenv("PARALLEL_GO_BUILD", "")

	// assert an unstamped build has no ldflags
	plain := goBuildArgs("amd64", []string{"cmd/rest-api"}, false, nil)
	for _, arg := range plain {
		if strings.HasPrefix(arg, "-ldflags") {
			t.Errorf("unstamped build has %q: %v", arg, plain)
		}
	}
	wantPlain := []string{"build", "-buildvcs=false", "-o", "bin/amd64/", "./cmd/rest-api"}
	if !slices.Equal(plain, wantPlain) {
		t.Errorf("goBuildArgs = %v, want %v", plain, wantPlain)
	}

	// assert a stamped build has the flag before the output and packages
	stamp := "-ldflags=-X=github.com/threeport/threeport/internal/version.Commit=a1b2c3d"
	stamped := goBuildArgs("arm64", []string{"cmd/rest-api"}, true, []string{stamp})
	wantStamped := []string{"build", "-buildvcs=false", "-a", stamp, "-o", "bin/arm64/", "./cmd/rest-api"}
	if !slices.Equal(stamped, wantStamped) {
		t.Errorf("goBuildArgs = %v, want %v", stamped, wantStamped)
	}
}

// TestBuildBinariesFailsOnInvalidHeadCommit covers a bad HEAD_COMMIT failing
// the build before any compile starts.
func TestBuildBinariesFailsOnInvalidHeadCommit(t *testing.T) {
	t.Setenv(HeadCommitEnv, "not-a-sha")

	// a missing repo dir proves nothing was built
	err := BuildBinaries(filepath.Join(t.TempDir(), "missing"), []string{"amd64", "arm64"}, []string{"cmd/rest-api"}, false)
	if err == nil || !strings.Contains(err.Error(), HeadCommitEnv) {
		t.Fatalf("expected an error naming %s, got %v", HeadCommitEnv, err)
	}
}

// TestWithEnvRestoresPreviousState covers the variable returning to its
// prior value, or to unset, however fn returns.
func TestWithEnvRestoresPreviousState(t *testing.T) {
	const key = "THREEPORT_TEST_WITH_ENV"

	// assert a variable that was set is restored
	t.Setenv(key, "before")
	err := WithEnv(key, "during", func() error {
		if got := os.Getenv(key); got != "during" {
			t.Errorf("inside WithEnv %s = %q, want during", key, got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithEnv returned error: %v", err)
	}
	if got := os.Getenv(key); got != "before" {
		t.Errorf("after WithEnv %s = %q, want before", key, got)
	}

	// assert a variable that was unset is unset again, and fn's error returns
	os.Unsetenv(key)
	err = WithEnv(key, "during", func() error { return os.ErrNotExist })
	if err != os.ErrNotExist {
		t.Errorf("WithEnv returned %v, want fn's error", err)
	}
	if _, ok := os.LookupEnv(key); ok {
		t.Errorf("%s is still set after WithEnv", key)
	}
}

// TestIsReleaseBranch covers release branch names.
func TestIsReleaseBranch(t *testing.T) {
	for name, want := range map[string]bool{
		"0.7":         true,
		"0.12":        true,
		"10.2":        true,
		"main":        false,
		"head-builds": false,
		"0.7.1":       false,
		"v0.7":        false,
		"0.7-fix":     false,
		"":            false,
	} {
		if got := IsReleaseBranch(name); got != want {
			t.Errorf("IsReleaseBranch(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestRequireToolsNamesEveryMissingTool covers one error listing each tool
// that is not on PATH.
func TestRequireToolsNamesEveryMissingTool(t *testing.T) {
	// assert present tools pass
	if err := RequireTools("go"); err != nil {
		t.Fatalf("RequireTools(go) returned error: %v", err)
	}

	// assert both missing tools are named
	err := RequireTools("go", "no-such-tool-one", "no-such-tool-two")
	if err == nil {
		t.Fatal("RequireTools succeeded with missing tools")
	}
	for _, name := range []string{"no-such-tool-one", "no-such-tool-two"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not name %s: %v", name, err)
		}
	}
}

// multiArchInspectJSON is the shape of docker buildx imagetools inspect
// --format '{{json .Image}}' for a multi-arch image.
const multiArchInspectJSON = `{
  "linux/amd64": {
    "architecture": "amd64", "os": "linux",
    "config": {"Labels": {
      "org.opencontainers.image.revision": "` + testFullSha + `",
      "org.opencontainers.image.version": "v0.7.0-rc.6.a1b2c3d"}}
  },
  "linux/arm64": {
    "architecture": "arm64", "os": "linux",
    "config": {"Labels": {
      "org.opencontainers.image.revision": "` + testFullSha + `"}}
  }
}`

// TestParseImageLabelsReadsEachPlatform covers a multi-arch image giving
// labels per platform.
func TestParseImageLabelsReadsEachPlatform(t *testing.T) {
	labels, err := ParseImageLabels([]byte(multiArchInspectJSON))
	if err != nil {
		t.Fatalf("ParseImageLabels returned error: %v", err)
	}

	// assert both platforms and the revision label of each
	for _, platform := range []string{"linux/amd64", "linux/arm64"} {
		if got := labels[platform][labelRevision]; got != testFullSha {
			t.Errorf("%s revision = %q, want %s", platform, got, testFullSha)
		}
	}
}

// TestParseImageLabelsReadsSinglePlatformImage covers an image that is not a
// manifest list printing one config.
func TestParseImageLabelsReadsSinglePlatformImage(t *testing.T) {
	data := `{"architecture": "amd64", "os": "linux", "config": {"Labels": {"` + labelRevision + `": "` + testFullSha + `"}}}`

	labels, err := ParseImageLabels([]byte(data))
	if err != nil {
		t.Fatalf("ParseImageLabels returned error: %v", err)
	}
	if got := labels["linux/amd64"][labelRevision]; got != testFullSha {
		t.Errorf("linux/amd64 revision = %q, want %s", got, testFullSha)
	}
}

// TestParseImageLabelsRejectsUnusableOutput covers output with no image config.
func TestParseImageLabelsRejectsUnusableOutput(t *testing.T) {
	for name, data := range map[string]string{
		"null":    "null",
		"empty":   "{}",
		"garbage": "not json",
	} {
		if _, err := ParseImageLabels([]byte(data)); err == nil {
			t.Errorf("ParseImageLabels accepted %s output", name)
		}
	}
}

// TestCheckImageRevision covers a Head image passing only when every arch is
// present and built from the commit.
func TestCheckImageRevision(t *testing.T) {
	arches := []string{"amd64", "arm64"}
	labels, err := ParseImageLabels([]byte(multiArchInspectJSON))
	if err != nil {
		t.Fatal(err)
	}

	// assert a matching image passes
	if err := CheckImageRevision("img:tag", labels, arches, testFullSha); err != nil {
		t.Errorf("CheckImageRevision returned error for a matching image: %v", err)
	}

	// assert a different commit fails and names both revisions
	other := strings.Repeat("f", 40)
	err = CheckImageRevision("img:tag", labels, arches, other)
	if err == nil || !strings.Contains(err.Error(), testFullSha) || !strings.Contains(err.Error(), other) {
		t.Errorf("expected a revision mismatch naming both shas, got %v", err)
	}

	// assert a missing arch fails
	err = CheckImageRevision("img:tag", labels, []string{"amd64", "arm64", "s390x"}, testFullSha)
	if err == nil || !strings.Contains(err.Error(), "linux/s390x") {
		t.Errorf("expected a missing linux/s390x error, got %v", err)
	}

	// assert a missing revision label fails
	delete(labels["linux/arm64"], labelRevision)
	err = CheckImageRevision("img:tag", labels, arches, testFullSha)
	if err == nil || !strings.Contains(err.Error(), "linux/arm64") {
		t.Errorf("expected an arm64 revision error, got %v", err)
	}

	// assert extra platforms, such as attestations, are ignored
	labels["unknown/unknown"] = map[string]string{}
	labels["linux/arm64"][labelRevision] = testFullSha
	if err := CheckImageRevision("img:tag", labels, arches, testFullSha); err != nil {
		t.Errorf("CheckImageRevision rejected an extra platform: %v", err)
	}
}

// TestSelectDispatchedRun covers picking the run for this commit that this
// dispatch created.
func TestSelectDispatchedRun(t *testing.T) {
	dispatched := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	title := "Head images for " + testFullSha
	runs, err := ParseWorkflowRuns([]byte(`[
	  {"databaseId": 1, "displayTitle": "Head images for ` + strings.Repeat("f", 40) + `", "url": "u1", "createdAt": "2026-11-27T12:00:05Z"},
	  {"databaseId": 2, "displayTitle": "` + title + `", "url": "u2", "createdAt": "2026-11-27T11:00:00Z"},
	  {"databaseId": 3, "displayTitle": "` + title + `", "url": "u3", "createdAt": "2026-11-27T12:00:10Z"},
	  {"databaseId": 4, "displayTitle": "` + title + `", "url": "u4", "createdAt": "2026-11-27T12:00:20Z"}
	]`))
	if err != nil {
		t.Fatalf("ParseWorkflowRuns returned error: %v", err)
	}

	// assert the newest run for the commit after the dispatch is chosen
	run, ok := SelectDispatchedRun(runs, testFullSha, dispatched)
	if !ok || run.DatabaseID != 4 || run.URL != "u4" {
		t.Errorf("SelectDispatchedRun = %+v, %v, want run 4", run, ok)
	}

	// assert an older run alone is not chosen
	if run, ok := SelectDispatchedRun(runs[1:2], testFullSha, dispatched); ok {
		t.Errorf("SelectDispatchedRun chose the earlier run %+v", run)
	}

	// assert a run within the clock skew allowance is chosen
	early := []WorkflowRun{{DatabaseID: 5, DisplayTitle: title, CreatedAt: dispatched.Add(-10 * time.Second)}}
	if _, ok := SelectDispatchedRun(early, testFullSha, dispatched.Add(-dispatchClockSkew)); !ok {
		t.Error("SelectDispatchedRun missed a run inside the clock skew allowance")
	}

	// assert another commit finds nothing
	if _, ok := SelectDispatchedRun(runs, strings.Repeat("e", 40), dispatched); ok {
		t.Error("SelectDispatchedRun chose a run for another commit")
	}
}

// TestParseLsRemoteMatchesExactRef covers reading only the exact ref.
func TestParseLsRemoteMatchesExactRef(t *testing.T) {
	other := strings.Repeat("e", 40)
	output := other + "\trefs/heads/fix/refs/heads/0.7\n" + testFullSha + "\trefs/heads/0.7\n"

	// assert the exact ref wins
	if got, ok := parseLsRemote(output, "refs/heads/0.7"); !ok || got != testFullSha {
		t.Errorf("parseLsRemote = %q, %v, want %s", got, ok, testFullSha)
	}

	// assert a missing ref is not found
	if _, ok := parseLsRemote(output, "refs/heads/0.8"); ok {
		t.Error("parseLsRemote found a ref that is not listed")
	}
	if _, ok := parseLsRemote("", "refs/heads/0.7"); ok {
		t.Error("parseLsRemote found a ref in empty output")
	}
}

// gitIn runs git in dir for a test and returns trimmed stdout.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()

	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %s: %v", args, dir, out, err)
	}
	return strings.TrimSpace(string(out))
}

// newReleaseRepo creates a repo on the 0.7 branch with one commit, and a bare
// repo at the same commit to stand in for upstream. No remote is configured.
func newReleaseRepo(t *testing.T) (repoDir, upstreamURL string) {
	t.Helper()

	clearGitRedirects(t)
	repoDir, _ = initRepo(t)
	gitIn(t, repoDir, "branch", "-m", "0.7")

	upstreamURL = t.TempDir()
	gitIn(t, upstreamURL, "init", "--bare", "--initial-branch=0.7")
	gitIn(t, repoDir, "push", upstreamURL, "0.7")
	return repoDir, upstreamURL
}

// TestRequireCleanTree covers any uncommitted or untracked file failing.
func TestRequireCleanTree(t *testing.T) {
	repoDir, _ := newReleaseRepo(t)

	// assert a clean tree passes
	if err := RequireCleanTree(repoDir); err != nil {
		t.Fatalf("RequireCleanTree returned error for a clean tree: %v", err)
	}

	// assert an untracked file fails and is named
	if err := os.WriteFile(filepath.Join(repoDir, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RequireCleanTree(repoDir); err == nil || !strings.Contains(err.Error(), "scratch.txt") {
		t.Errorf("expected an error naming scratch.txt, got %v", err)
	}

	// assert a modified go.mod fails too, because there are no exceptions
	gitIn(t, repoDir, "add", "scratch.txt")
	gitIn(t, repoDir, "commit", "-m", "add scratch")
	if err := os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RequireCleanTree(repoDir); err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("expected an error naming go.mod, got %v", err)
	}
}

// TestRequireReleaseBranch covers the branch name check.
func TestRequireReleaseBranch(t *testing.T) {
	repoDir, _ := newReleaseRepo(t)

	// assert a release branch passes and is returned
	if branch, err := RequireReleaseBranch(repoDir); err != nil || branch != "0.7" {
		t.Errorf("RequireReleaseBranch = %q, %v, want 0.7", branch, err)
	}

	// assert another branch fails and says to check out the release branch
	gitIn(t, repoDir, "checkout", "-b", "head-builds")
	if _, err := RequireReleaseBranch(repoDir); err == nil ||
		!strings.Contains(err.Error(), "head-builds") ||
		!strings.Contains(err.Error(), "release branch") {
		t.Errorf("expected a release branch error naming head-builds, got %v", err)
	}

	// assert a detached HEAD fails
	gitIn(t, repoDir, "checkout", "--detach")
	if _, err := RequireReleaseBranch(repoDir); err == nil || !strings.Contains(err.Error(), "detached") {
		t.Errorf("expected a detached HEAD error, got %v", err)
	}
}

// TestRequireAtUpstreamHead covers local HEAD being equal to upstream, and
// the behind, ahead and diverged cases failing with guidance. The upstream is
// a URL only, so no remote is needed.
func TestRequireAtUpstreamHead(t *testing.T) {
	repoDir, upstreamURL := newReleaseRepo(t)
	atHead := gitIn(t, repoDir, "rev-parse", "HEAD")

	// assert equal heads pass and return the full sha
	got, err := RequireAtUpstreamHead(repoDir, upstreamURL, "0.7")
	if err != nil || got != atHead {
		t.Fatalf("RequireAtUpstreamHead = %q, %v, want %s", got, err, atHead)
	}

	// assert a missing upstream branch is an error
	if _, err := RequireAtUpstreamHead(repoDir, upstreamURL, "0.8"); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Errorf("expected a branch not found error, got %v", err)
	}

	// assert an unreachable upstream is an error
	if _, err := RequireAtUpstreamHead(repoDir, filepath.Join(t.TempDir(), "nope"), "0.7"); err == nil {
		t.Error("expected an error for an unreachable upstream")
	}

	// assert a local commit that is not upstream fails, saying why
	gitIn(t, repoDir, "commit", "--allow-empty", "-m", "local only")
	_, err = RequireAtUpstreamHead(repoDir, upstreamURL, "0.7")
	if err == nil || !strings.Contains(err.Error(), "merged upstream") {
		t.Errorf("expected an ahead error mentioning merged upstream, got %v", err)
	}

	// assert a local checkout behind upstream fails, saying to pull
	gitIn(t, repoDir, "push", upstreamURL, "0.7")
	gitIn(t, repoDir, "reset", "--hard", atHead)
	_, err = RequireAtUpstreamHead(repoDir, upstreamURL, "0.7")
	if err == nil || !strings.Contains(err.Error(), "pull from upstream") {
		t.Errorf("expected a behind error saying to pull, got %v", err)
	}

	// assert a diverged checkout fails with the same two-part guidance
	gitIn(t, repoDir, "commit", "--allow-empty", "-m", "diverged")
	_, err = RequireAtUpstreamHead(repoDir, upstreamURL, "0.7")
	if err == nil ||
		!strings.Contains(err.Error(), "pull from upstream") ||
		!strings.Contains(err.Error(), "merged upstream") {
		t.Errorf("expected diverged guidance, got %v", err)
	}
}

// TestReadVersionFileTrimsWhitespace covers the version coming from the file
// on disk with its newline removed.
func TestReadVersionFileTrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "version"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, versionFilePath), []byte("v0.7.0-rc.8\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// assert the trimmed version
	if got, err := ReadVersionFile(dir); err != nil || got != "v0.7.0-rc.8" {
		t.Errorf("ReadVersionFile = %q, %v, want v0.7.0-rc.8", got, err)
	}

	// assert a missing file is an error
	if _, err := ReadVersionFile(t.TempDir()); err == nil {
		t.Error("ReadVersionFile succeeded without a version file")
	}
}

// TestHeadImagesStampingCheck covers the check in head-images.yml that refuses
// a commit from before Head builds existed, because the build runs the code of
// the commit being built and that code could not stamp the Head version. The
// workflow finds support with a grep of the commit's source, so the patterns
// must keep matching the current source, and the workflow must keep using
// them. Change both together.
func TestHeadImagesStampingCheck(t *testing.T) {
	root := filepath.Join("..", "..", "..")

	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("failed to read %s: %v", path, err)
		}
		return string(data)
	}

	// assert the version package still declares Commit on a line of its own
	if !slices.Contains(strings.Split(read("internal/version/version_gen.go"), "\n"), "var Commit string") {
		t.Error("internal/version/version_gen.go has no line 'var Commit string', which head-images.yml looks for")
	}

	// assert the build code still applies the stamp
	if !strings.Contains(read("pkg/util/v0/build.go"), "VersionLdflags") {
		t.Error("pkg/util/v0/build.go does not use VersionLdflags, which head-images.yml looks for")
	}

	// assert the workflow still looks for exactly these
	workflow := read(".github/workflows/head-images.yml")
	for _, want := range []string{
		`git grep -q -e '^var Commit string$' "$COMMIT" -- internal/version/version_gen.go`,
		`git grep -q -e 'VersionLdflags' "$COMMIT" -- pkg/util/v0/build.go`,
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("head-images.yml no longer contains the check %q", want)
		}
	}
}
