package v0

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestResolveImageRepoPrefersExplicitOverride covers ResolveImageRepo returning
// IMAGE_REPO verbatim when set, ahead of any CI or dev derivation.
func TestResolveImageRepoPrefersExplicitOverride(t *testing.T) {
	// an explicit override plus CI signals that would otherwise derive ghcr
	t.Setenv("IMAGE_REPO", "localhost:5001")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "AcmeCorp")
	// the override wins over the CI derivation
	if got := ResolveImageRepo("localhost:5001"); got != "localhost:5001" {
		t.Errorf("ResolveImageRepo = %q, want the IMAGE_REPO override", got)
	}
}

// TestResolveImageRepoDerivesGhcrInCI covers ResolveImageRepo building a
// lowercased ghcr namespace from the repository owner under GitHub Actions.
func TestResolveImageRepoDerivesGhcrInCI(t *testing.T) {
	// no override, in CI, mixed-case owner
	t.Setenv("IMAGE_REPO", "")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "AcmeCorp")
	// the owner lowercases into the ghcr namespace
	if got := ResolveImageRepo("localhost:5001"); got != "ghcr.io/acmecorp" {
		t.Errorf("ResolveImageRepo = %q, want ghcr.io/acmecorp", got)
	}
}

// TestResolveImageRepoFallsBackToDevDefault covers ResolveImageRepo returning
// the dev default outside CI with no override.
func TestResolveImageRepoFallsBackToDevDefault(t *testing.T) {
	// no override, not in CI
	t.Setenv("IMAGE_REPO", "")
	t.Setenv("GITHUB_ACTIONS", "")
	// the local dev registry is the fallback
	if got := ResolveImageRepo("localhost:5001"); got != "localhost:5001" {
		t.Errorf("ResolveImageRepo = %q, want localhost:5001", got)
	}
}

// TestResolveImageTagPrefersExplicitOverride covers ResolveImageTag returning
// IMAGE_TAG verbatim when set.
func TestResolveImageTagPrefersExplicitOverride(t *testing.T) {
	// an explicit tag override under CI
	t.Setenv("IMAGE_TAG", "v9.9.9")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ARCH", "")
	// the override wins
	got, err := ResolveImageTag("", "v0.1.0-dev")
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}
	if got != "v9.9.9" {
		t.Errorf("ResolveImageTag = %q, want the IMAGE_TAG override", got)
	}
}

// TestResolveImageTagEchoesRefNameOnTagBuild covers ResolveImageTag returning
// the pushed ref name on a CI tag build.
func TestResolveImageTagEchoesRefNameOnTagBuild(t *testing.T) {
	// a CI tag build carries the ref name
	t.Setenv("IMAGE_TAG", "")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REF_TYPE", "tag")
	t.Setenv("GITHUB_REF_NAME", "v0.1.0-dev.3")
	t.Setenv("ARCH", "")
	// the tag build echoes the ref name verbatim
	got, err := ResolveImageTag("", "v0.1.0-dev")
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}
	if got != "v0.1.0-dev.3" {
		t.Errorf("ResolveImageTag = %q, want the ref name", got)
	}
}

// gitRedirectVars lists git environment variables that redirect git at
// another repository. git exports them to hook processes, so a suite
// started from a pre-push hook sees the hook's repository until they
// are cleared.
var gitRedirectVars = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"}

// clearGitRedirects unsets git's repository-redirect environment
// variables for the rest of the test. git treats a set-but-empty GIT_DIR
// as a path, so Unsetenv is required after t.Setenv records the original.
func clearGitRedirects(t *testing.T) {
	t.Helper()
	// t.Setenv records each original and restores it at cleanup; Unsetenv
	// is what the later git command actually sees
	for _, name := range gitRedirectVars {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
}

// isolateFromGit moves the test into an empty temp directory with git's
// repository-redirect variables cleared, so git sees no checkout.
func isolateFromGit(t *testing.T) {
	t.Helper()
	// drop git redirects inherited from the test process
	clearGitRedirects(t)
	// leave the enclosing repository so git cannot discover it by walking up
	t.Chdir(t.TempDir())
}

// TestResolveImageTagFallsBackToVersionOutsideCheckout covers ResolveImageTag
// returning the bare version default outside CI and outside a git checkout.
func TestResolveImageTagFallsBackToVersionOutsideCheckout(t *testing.T) {
	// no IMAGE_TAG, outside CI, ARCH unset
	t.Setenv("IMAGE_TAG", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ARCH", "")
	// leave the enclosing checkout so a sha read cannot pass
	isolateFromGit(t)
	// the bare version default passes through when no sha is available
	got, err := ResolveImageTag("", "v0.1.0-dev")
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}
	if got != "v0.1.0-dev" {
		t.Errorf("ResolveImageTag = %q, want v0.1.0-dev", got)
	}
}

// TestResolveImageTagSuffixesShaInCheckout covers ResolveImageTag suffixing
// the version default with a short sha in a git checkout.
func TestResolveImageTagSuffixesShaInCheckout(t *testing.T) {
	// not in CI; the test runs inside the repo checkout, so a short sha is read
	t.Setenv("IMAGE_TAG", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ARCH", "")
	// resolve against the process working directory, this checkout
	got, err := ResolveImageTag("", "v0.1.0-dev")
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}
	// the tag starts with the version default and a trailing dot
	prefix := "v0.1.0-dev."
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("ResolveImageTag = %q, want prefix %q", got, prefix)
	}
	// the sha suffix is seven characters
	if sha := strings.TrimPrefix(got, prefix); len(sha) != 7 {
		t.Errorf("sha suffix = %q, want seven characters", sha)
	}
}

// TestResolveImageTagIgnoresArch covers ResolveImageTag leaving an override
// tag undecorated when ARCH is set.
func TestResolveImageTagIgnoresArch(t *testing.T) {
	// an explicit override plus a single ARCH value
	t.Setenv("IMAGE_TAG", "v9.9.9")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ARCH", "arm64")
	// the canonical tag passes through undecorated
	got, err := ResolveImageTag("", "v0.1.0-dev")
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}
	if got != "v9.9.9" {
		t.Errorf("ResolveImageTag = %q, want the undecorated v9.9.9", got)
	}
}

// TestBuildImageTagDecoratesSingleArch covers buildImageTag decorating the
// canonical tag with -<arch> when ARCH names a single arch.
func TestBuildImageTagDecoratesSingleArch(t *testing.T) {
	// an explicit override plus a single-arch ARCH
	t.Setenv("IMAGE_TAG", "v9.9.9")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ARCH", "arm64")
	// the arch decorates the resolved tag
	got, err := buildImageTag("", "v0.1.0-dev")
	if err != nil {
		t.Fatalf("buildImageTag returned error: %v", err)
	}
	if got != "v9.9.9-arm64" {
		t.Errorf("buildImageTag = %q, want v9.9.9-arm64", got)
	}
}

// TestBuildImageTagSingleArchDecoratesFallbackVersion covers buildImageTag
// decorating the bare version default with -<arch> outside CI and outside a
// git checkout.
func TestBuildImageTagSingleArchDecoratesFallbackVersion(t *testing.T) {
	// no IMAGE_TAG, outside CI, a single ARCH value
	t.Setenv("IMAGE_TAG", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ARCH", "amd64")
	// leave the enclosing checkout so the version default is the tag
	isolateFromGit(t)
	// the arch decorates the bare fallback version
	got, err := buildImageTag("", "v0.1.0-dev")
	if err != nil {
		t.Fatalf("buildImageTag returned error: %v", err)
	}
	if got != "v0.1.0-dev-amd64" {
		t.Errorf("buildImageTag = %q, want v0.1.0-dev-amd64", got)
	}
}

// TestBuildImageTagCommaListArchIsBare covers a comma-list ARCH leaving the
// resolved tag undecorated.
func TestBuildImageTagCommaListArchIsBare(t *testing.T) {
	// an override with a comma-list ARCH, as a one-shot multi-arch build sets
	t.Setenv("IMAGE_TAG", "v9.9.9")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ARCH", "amd64,arm64")
	// the bare tag passes through undecorated
	got, err := buildImageTag("", "v0.1.0-dev")
	if err != nil {
		t.Fatalf("buildImageTag returned error: %v", err)
	}
	if got != "v9.9.9" {
		t.Errorf("buildImageTag = %q, want v9.9.9", got)
	}
}

// initRepo creates a git repository holding one empty commit and
// returns its path and seven-character HEAD sha. Redirect variables
// must already be cleared before this helper runs.
func initRepo(t *testing.T) (dir, sha string) {
	t.Helper()
	// create an empty directory for the fixture repository
	dir = t.TempDir()
	// init main, set identity, and seed one empty commit
	commands := [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"commit", "--allow-empty", "-m", "seed"},
	}
	// run the seed commands against that directory
	for _, args := range commands {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v in fixture repo: %s: %v", args, out, err)
		}
	}
	// read the seven-character HEAD sha
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--short=7", "HEAD").Output()
	if err != nil {
		t.Fatalf("failed to read fixture repo sha: %v", err)
	}
	return dir, strings.TrimSpace(string(out))
}

// TestResolveImageTagReadsShaFromRepoDir covers ResolveImageTag reading the
// short sha from the repository it is handed rather than the process working
// directory.
func TestResolveImageTagReadsShaFromRepoDir(t *testing.T) {
	// no IMAGE_TAG, outside CI, ARCH unset
	t.Setenv("IMAGE_TAG", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ARCH", "")
	// leave the process checkout so a working-directory read cannot pass
	isolateFromGit(t)
	// a fixture repository with a known HEAD sha
	repoDir, want := initRepo(t)
	// the tag carries the fixture repository's sha
	got, err := ResolveImageTag(repoDir, "v0.1.0-dev")
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}
	if got != "v0.1.0-dev."+want {
		t.Errorf("ResolveImageTag = %q, want v0.1.0-dev.%s", got, want)
	}
}

// TestJoinImageTagJoinsVersionAndSha covers joinImageTag joining a version
// and a short sha with a dot.
func TestJoinImageTagJoinsVersionAndSha(t *testing.T) {
	// join a version and a short sha with a dot
	if got := joinImageTag("v0.1.0-dev", "abc1234"); got != "v0.1.0-dev.abc1234" {
		t.Errorf("joinImageTag = %q, want v0.1.0-dev.abc1234", got)
	}
}
