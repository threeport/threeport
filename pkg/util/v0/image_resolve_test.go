package v0

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestResolveImageRepoPrefersExplicitOverride covers a set IMAGE_REPO winning
// over the GitHub Actions derivation.
func TestResolveImageRepoPrefersExplicitOverride(t *testing.T) {
	// set IMAGE_REPO while Actions is on
	t.Setenv("IMAGE_REPO", "localhost:5001")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "AcmeCorp")

	// assert the explicit repo is returned
	if got := ResolveImageRepo("localhost:5001"); got != "localhost:5001" {
		t.Errorf("ResolveImageRepo = %q, want the IMAGE_REPO override", got)
	}
}

// TestResolveImageRepoDerivesGhcrInCI covers the ghcr.io namespace built from
// the lowercased Actions owner when IMAGE_REPO is blank.
func TestResolveImageRepoDerivesGhcrInCI(t *testing.T) {
	// clear IMAGE_REPO and set the Actions owner
	t.Setenv("IMAGE_REPO", "")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "AcmeCorp")

	// assert the lowercased ghcr namespace
	if got := ResolveImageRepo("localhost:5001"); got != "ghcr.io/acmecorp" {
		t.Errorf("ResolveImageRepo = %q, want ghcr.io/acmecorp", got)
	}
}

// TestResolveImageRepoFallsBackToDevDefault covers the caller default when
// IMAGE_REPO is blank and the process is not under Actions.
func TestResolveImageRepoFallsBackToDevDefault(t *testing.T) {
	// clear IMAGE_REPO and Actions
	t.Setenv("IMAGE_REPO", "")
	t.Setenv("GITHUB_ACTIONS", "")

	// assert the supplied default
	if got := ResolveImageRepo("localhost:5001"); got != "localhost:5001" {
		t.Errorf("ResolveImageRepo = %q, want localhost:5001", got)
	}
}

// TestResolveImageTagPrefersExplicitOverride covers a set IMAGE_TAG winning
// over Actions and ARCH.
func TestResolveImageTagPrefersExplicitOverride(t *testing.T) {
	// set IMAGE_TAG under Actions with ARCH empty
	t.Setenv("IMAGE_TAG", "v9.9.9")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ARCH", "")

	// resolve the tag
	got, err := ResolveImageTag("", "v0.1.0-dev")

	// assert a nil error
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}

	// assert the explicit tag
	if got != "v9.9.9" {
		t.Errorf("ResolveImageTag = %q, want the IMAGE_TAG override", got)
	}
}

// TestResolveImageTagEchoesRefNameOnTagBuild covers a tag-triggered Actions
// run returning GITHUB_REF_NAME when IMAGE_TAG is blank.
func TestResolveImageTagEchoesRefNameOnTagBuild(t *testing.T) {
	// clear IMAGE_TAG and mark the run as a tag build
	t.Setenv("IMAGE_TAG", "")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REF_TYPE", "tag")
	t.Setenv("GITHUB_REF_NAME", "v0.1.0-dev.3")
	t.Setenv("ARCH", "")

	// resolve the tag
	got, err := ResolveImageTag("", "v0.1.0-dev")

	// assert a nil error
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}

	// assert the ref name
	if got != "v0.1.0-dev.3" {
		t.Errorf("ResolveImageTag = %q, want the ref name", got)
	}
}

// gitRedirectVars are env vars git exports into hook processes. A suite
// started from a pre-push hook still sees that repository until they are cleared.
var gitRedirectVars = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"}

// clearGitRedirects unsets git directory redirects for the rest of the test.
// A set-but-empty GIT_DIR is still a path, so Unsetenv follows t.Setenv.
func clearGitRedirects(t *testing.T) {
	t.Helper()

	// record the prior value, then unset the redirect
	for _, name := range gitRedirectVars {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
}

// isolateFromGit clears git redirects and chdirs to a temp dir so git
// cannot discover a repository by walking up from cwd.
func isolateFromGit(t *testing.T) {
	t.Helper()

	// drop git directory redirects
	clearGitRedirects(t)

	// leave the checkout so walk-up discovery fails
	t.Chdir(t.TempDir())
}

// TestResolveImageTagFallsBackToVersionOutsideCheckout covers a missing sha
// outside Actions returning the version and a nil error.
func TestResolveImageTagFallsBackToVersionOutsideCheckout(t *testing.T) {
	// clear tag and Actions env
	t.Setenv("IMAGE_TAG", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ARCH", "")

	// leave any checkout
	isolateFromGit(t)

	// resolve the tag
	got, err := ResolveImageTag("", "v0.1.0-dev")

	// assert a nil error
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}

	// assert the bare version
	if got != "v0.1.0-dev" {
		t.Errorf("ResolveImageTag = %q, want v0.1.0-dev", got)
	}
}

// TestResolveImageTagSuffixesShaInCheckout covers a local resolve appending
// a seven-character sha when repoDir is empty.
func TestResolveImageTagSuffixesShaInCheckout(t *testing.T) {
	// clear tag and Actions env
	t.Setenv("IMAGE_TAG", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ARCH", "")

	// resolve with an empty repoDir
	got, err := ResolveImageTag("", "v0.1.0-dev")

	// assert a nil error
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}

	// assert the version.sha prefix
	prefix := "v0.1.0-dev."
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("ResolveImageTag = %q, want prefix %q", got, prefix)
	}

	// assert a seven-character sha
	if sha := strings.TrimPrefix(got, prefix); len(sha) != 7 {
		t.Errorf("sha suffix = %q, want seven characters", sha)
	}
}

// TestResolveImageTagIgnoresArch covers ResolveImageTag leaving a set ARCH
// off the tag.
func TestResolveImageTagIgnoresArch(t *testing.T) {
	// set IMAGE_TAG and a single ARCH under Actions
	t.Setenv("IMAGE_TAG", "v9.9.9")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ARCH", "arm64")

	// resolve the tag
	got, err := ResolveImageTag("", "v0.1.0-dev")

	// assert a nil error
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}

	// assert the tag has no arch suffix
	if got != "v9.9.9" {
		t.Errorf("ResolveImageTag = %q, want the undecorated v9.9.9", got)
	}
}

// TestBuildImageTagDecoratesSingleArch covers a one-value ARCH appending
// -<arch> to the canonical tag.
func TestBuildImageTagDecoratesSingleArch(t *testing.T) {
	// set IMAGE_TAG and a single ARCH
	t.Setenv("IMAGE_TAG", "v9.9.9")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ARCH", "arm64")

	// build the push tag
	got, err := buildImageTag("", "v0.1.0-dev")

	// assert a nil error
	if err != nil {
		t.Fatalf("buildImageTag returned error: %v", err)
	}

	// assert the arch suffix
	if got != "v9.9.9-arm64" {
		t.Errorf("buildImageTag = %q, want v9.9.9-arm64", got)
	}
}

// TestBuildImageTagSingleArchDecoratesFallbackVersion covers a single ARCH
// appended to the bare version when no checkout supplies a sha.
func TestBuildImageTagSingleArchDecoratesFallbackVersion(t *testing.T) {
	// set a single ARCH outside Actions
	t.Setenv("IMAGE_TAG", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ARCH", "amd64")

	// leave any checkout
	isolateFromGit(t)

	// build the push tag
	got, err := buildImageTag("", "v0.1.0-dev")

	// assert a nil error
	if err != nil {
		t.Fatalf("buildImageTag returned error: %v", err)
	}

	// assert version plus arch and no sha
	if got != "v0.1.0-dev-amd64" {
		t.Errorf("buildImageTag = %q, want v0.1.0-dev-amd64", got)
	}
}

// TestBuildImageTagCommaListArchIsBare covers a comma-separated ARCH, the
// multi-arch value, leaving the canonical tag undecorated.
func TestBuildImageTagCommaListArchIsBare(t *testing.T) {
	// set a comma-separated multi-arch ARCH
	t.Setenv("IMAGE_TAG", "v9.9.9")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ARCH", "amd64,arm64")

	// build the push tag
	got, err := buildImageTag("", "v0.1.0-dev")

	// assert a nil error
	if err != nil {
		t.Fatalf("buildImageTag returned error: %v", err)
	}

	// assert the bare canonical tag
	if got != "v9.9.9" {
		t.Errorf("buildImageTag = %q, want v9.9.9", got)
	}
}

// initRepo creates a temp git repo with one empty commit and returns its
// directory and --short=7 HEAD sha. Callers must already have cleared git redirects.
func initRepo(t *testing.T) (dir, sha string) {
	t.Helper()

	// create an empty directory
	dir = t.TempDir()
	commands := [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"commit", "--allow-empty", "-m", "seed"},
	}

	// init main, set identity, and seed one empty commit
	for _, args := range commands {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v in fixture repo: %s: %v", args, out, err)
		}
	}

	// read the --short=7 HEAD sha
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--short=7", "HEAD").Output()
	if err != nil {
		t.Fatalf("failed to read fixture repo sha: %v", err)
	}
	return dir, strings.TrimSpace(string(out))
}

// TestResolveImageTagReadsShaFromRepoDir covers the short sha coming from the
// handed repository rather than the process working directory.
func TestResolveImageTagReadsShaFromRepoDir(t *testing.T) {
	// clear tag and Actions env
	t.Setenv("IMAGE_TAG", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ARCH", "")

	// leave any checkout
	isolateFromGit(t)

	// seed a fixture repo
	repoDir, want := initRepo(t)

	// resolve against that directory
	got, err := ResolveImageTag(repoDir, "v0.1.0-dev")

	// assert a nil error
	if err != nil {
		t.Fatalf("ResolveImageTag returned error: %v", err)
	}

	// assert version plus the fixture sha
	if got != "v0.1.0-dev."+want {
		t.Errorf("ResolveImageTag = %q, want v0.1.0-dev.%s", got, want)
	}
}

// TestJoinImageTagJoinsVersionAndSha covers version and sha joined by a dot.
func TestJoinImageTagJoinsVersionAndSha(t *testing.T) {
	// assert version.sha
	if got := joinImageTag("v0.1.0-dev", "abc1234"); got != "v0.1.0-dev.abc1234" {
		t.Errorf("joinImageTag = %q, want v0.1.0-dev.abc1234", got)
	}
}

// TestImageWithoutTag covers stripping a :tag while leaving a digest and a registry port.
func TestImageWithoutTag(t *testing.T) {
	// cover a ported registry, a host, a bare name, and a digest
	tests := []struct {
		name  string
		image string
		want  string
	}{
		{
			name:  "ported registry with a tag",
			image: "localhost:5001/threeport-rest-api:v0.7.0-dev.23",
			want:  "localhost:5001/threeport-rest-api",
		},
		{
			name:  "ported registry with no tag",
			image: "localhost:5001/threeport-rest-api",
			want:  "localhost:5001/threeport-rest-api",
		},
		{
			name:  "hosted registry with a tag",
			image: "ghcr.io/randalljohnson/threeport-rest-api:v0.7.0-dev.23",
			want:  "ghcr.io/randalljohnson/threeport-rest-api",
		},
		{
			name:  "bare name with a tag",
			image: "threeport-rest-api:v0.7.0-dev.23",
			want:  "threeport-rest-api",
		},
		{
			name:  "bare name with no tag",
			image: "threeport-rest-api",
			want:  "threeport-rest-api",
		},
		{
			name:  "digest reference is left alone",
			image: "ghcr.io/randalljohnson/threeport-rest-api@sha256:abc123",
			want:  "ghcr.io/randalljohnson/threeport-rest-api@sha256:abc123",
		},
	}

	// assert each image
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ImageWithoutTag(test.image); got != test.want {
				t.Errorf("ImageWithoutTag(%q) = %q, want %q", test.image, got, test.want)
			}
		})
	}
}
