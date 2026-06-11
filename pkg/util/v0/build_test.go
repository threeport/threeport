package v0

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestPrefixWriter_CompleteLineEmittedDuringWrite(t *testing.T) {
	var out bytes.Buffer
	w := &prefixWriter{prefix: "[svc]", out: &out}

	n, err := w.Write([]byte("done\n"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len("done\n") {
		t.Errorf("Write returned %d, want %d", n, len("done\n"))
	}
	if got, want := out.String(), "[svc] done\n"; got != want {
		t.Errorf("Write output = %q, want %q", got, want)
	}

	if err := w.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got, want := out.String(), "[svc] done\n"; got != want {
		t.Errorf("flush should be no-op, got %q, want %q", got, want)
	}
}

func TestPrefixWriter_PartialLineFlushed(t *testing.T) {
	var out bytes.Buffer
	w := &prefixWriter{prefix: "[svc]", out: &out}

	if _, err := w.Write([]byte("exporting layers")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("partial line should be buffered, got %q", out.String())
	}

	if err := w.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got, want := out.String(), "[svc] exporting layers\n"; got != want {
		t.Errorf("flush output = %q, want %q", got, want)
	}
}

func TestPrefixWriter_FlushEmptyBufferNoop(t *testing.T) {
	var out bytes.Buffer
	w := &prefixWriter{prefix: "[svc]", out: &out}

	if err := w.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("flush on empty buffer should write nothing, got %q", out.String())
	}
}

func TestPrefixWriter_WhitespaceOnlyLineSkipped(t *testing.T) {
	var out bytes.Buffer
	w := &prefixWriter{prefix: "[svc]", out: &out}

	if _, err := w.Write([]byte("   \n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("whitespace-only line should be skipped, got %q", out.String())
	}
}

func TestPrefixWriter_PartialThenCompleteAcrossWrites(t *testing.T) {
	var out bytes.Buffer
	w := &prefixWriter{prefix: "[svc]", out: &out}

	if _, err := w.Write([]byte("expo")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("partial line should be buffered, got %q", out.String())
	}
	if _, err := w.Write([]byte("rting layers\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got, want := out.String(), "[svc] exporting layers\n"; got != want {
		t.Errorf("output across writes = %q, want %q", got, want)
	}
}

func TestPrefixWriter_MultipleCompleteLines(t *testing.T) {
	var out bytes.Buffer
	w := &prefixWriter{prefix: "[svc]", out: &out}

	if _, err := w.Write([]byte("a\nb\nc\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	want := []string{"[svc] a", "[svc] b", "[svc] c"}
	if len(got) != len(want) {
		t.Fatalf("line count = %d, want %d (output %q)", len(got), len(want), out.String())
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPrefixWriter_CompleteLinesThenPartialFlushed(t *testing.T) {
	var out bytes.Buffer
	w := &prefixWriter{prefix: "[svc]", out: &out}

	if _, err := w.Write([]byte("first\nsecond\nthird-no-newline")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got, want := out.String(), "[svc] first\n[svc] second\n"; got != want {
		t.Errorf("complete-line output = %q, want %q", got, want)
	}
	if err := w.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	want := "[svc] first\n[svc] second\n[svc] third-no-newline\n"
	if got := out.String(); got != want {
		t.Errorf("output after flush = %q, want %q", got, want)
	}
}

// buildxArgsFixture returns inputs that produce a valid single-arch
// arg list. Individual tests override the bits they care about.
func buildxArgsFixture() (threeportPath, dockerfilePath, target string, platforms []string, binary, binDir string, extras map[string]string, imageRepo, imageName, imageTag string, pushImage bool) {
	return "/repo", "Dockerfile", "release",
		[]string{"linux/amd64"},
		"rest-api", "bin",
		map[string]string{
			"GIT_REVISION":  "deadbeef",
			"GIT_TAG":       "v0.7.0",
			"BUILD_CREATED": "2026-06-04T00:00:00Z",
		},
		"ghcr.io/threeport", "threeport-rest-api", "v0.7.0",
		true
}

func TestBuildImage_EmptyArchRejected(t *testing.T) {
	err := BuildImage("/repo", "Dockerfile", "release", "", "rest-api", "bin", nil, "ghcr.io/threeport", "threeport-rest-api", "v0.7.0", true, false, "")
	if err == nil || !strings.Contains(err.Error(), "--arch is required") {
		t.Fatalf("expected --arch is required, got %v", err)
	}
}

func TestBuildImage_MultiArchRequiresPush(t *testing.T) {
	err := BuildImage("/repo", "Dockerfile", "release", "amd64,arm64", "rest-api", "bin", nil, "ghcr.io/threeport", "threeport-rest-api", "v0.7.0", false, true, "kind")
	if err == nil || !strings.Contains(err.Error(), "multi-arch builds require --push") {
		t.Fatalf("expected multi-arch-requires-push error, got %v", err)
	}
}

func TestBuildImage_PushAndLoadMutuallyExclusive(t *testing.T) {
	err := BuildImage("/repo", "Dockerfile", "release", "amd64", "rest-api", "bin", nil, "ghcr.io/threeport", "threeport-rest-api", "v0.7.0", true, true, "kind")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected push/load exclusive error, got %v", err)
	}
}

func TestBuildxArgs_SingleArchSelectsLoad(t *testing.T) {
	tp, df, tgt, plats, bin, bd, ex, repo, name, tag, _ := buildxArgsFixture()
	args, _, _ := buildxBuildArgs(tp, df, tgt, plats, bin, bd, ex, repo, name, tag, false)
	if !slices.Contains(args, "--load") {
		t.Errorf("--load missing from args: %v", args)
	}
	if slices.Contains(args, "--push") {
		t.Errorf("--push should be absent: %v", args)
	}
	if slices.Contains(args, "--builder") {
		t.Errorf("--builder should be absent for single-arch: %v", args)
	}
}

func TestBuildxArgs_SingleArchPushOmitsBuilder(t *testing.T) {
	tp, df, tgt, plats, bin, bd, ex, repo, name, tag, _ := buildxArgsFixture()
	args, _, _ := buildxBuildArgs(tp, df, tgt, plats, bin, bd, ex, repo, name, tag, true)
	if !slices.Contains(args, "--push") {
		t.Errorf("--push missing: %v", args)
	}
	if slices.Contains(args, "--builder") {
		t.Errorf("--builder should be absent for single-arch: %v", args)
	}
}

func TestBuildxArgs_MultiArchUsesThreeportMulti(t *testing.T) {
	tp, df, tgt, _, bin, bd, ex, repo, name, tag, _ := buildxArgsFixture()
	args, _, _ := buildxBuildArgs(tp, df, tgt, []string{"linux/amd64", "linux/arm64"}, bin, bd, ex, repo, name, tag, true)
	if !slices.Contains(args, "--builder") {
		t.Errorf("--builder missing: %v", args)
	}
	if !slices.Contains(args, multiArchBuilderName) {
		t.Errorf("threeport-multi builder name missing: %v", args)
	}
	if !slices.Contains(args, "--platform=linux/amd64,linux/arm64") {
		t.Errorf("multi-platform arg missing: %v", args)
	}
}

func TestBuildxArgs_TargetOmittedWhenEmpty(t *testing.T) {
	tp, df, _, plats, bin, bd, ex, repo, name, tag, push := buildxArgsFixture()
	args, _, _ := buildxBuildArgs(tp, df, "", plats, bin, bd, ex, repo, name, tag, push)
	if slices.Contains(args, "--target") {
		t.Errorf("--target should be absent for empty target: %v", args)
	}
}

func TestBuildxArgs_TargetIncludedWhenSet(t *testing.T) {
	tp, df, _, plats, bin, bd, ex, repo, name, tag, push := buildxArgsFixture()
	args, _, _ := buildxBuildArgs(tp, df, "dev", plats, bin, bd, ex, repo, name, tag, push)
	idx := slices.Index(args, "--target")
	if idx < 0 || idx+1 >= len(args) || args[idx+1] != "dev" {
		t.Errorf("expected --target dev pair, got args: %v", args)
	}
}

func TestBuildxArgs_BinaryBuildArgSet(t *testing.T) {
	tp, df, tgt, plats, _, bd, ex, repo, name, tag, push := buildxArgsFixture()
	args, _, _ := buildxBuildArgs(tp, df, tgt, plats, "router-controller", bd, ex, repo, name, tag, push)
	if !slices.Contains(args, "BINARY=router-controller") {
		t.Errorf("BINARY build-arg missing: %v", args)
	}
}

func TestBuildxArgs_ExtraBuildArgsSorted(t *testing.T) {
	tp, df, tgt, plats, bin, bd, _, repo, name, tag, push := buildxArgsFixture()
	ex := map[string]string{
		"GIT_REVISION":  "deadbeef",
		"GIT_TAG":       "v0.7.0",
		"BUILD_CREATED": "2026-06-04T00:00:00Z",
		"ZULU":          "last",
		"ALPHA":         "first",
	}
	args, _, _ := buildxBuildArgs(tp, df, tgt, plats, bin, bd, ex, repo, name, tag, push)
	got := []string{}
	for i, a := range args {
		if a == "--build-arg" && i+1 < len(args) && args[i+1] != "BINARY=rest-api" {
			key := strings.SplitN(args[i+1], "=", 2)[0]
			got = append(got, key)
		}
	}
	want := []string{"ALPHA", "BUILD_CREATED", "GIT_REVISION", "GIT_TAG", "ZULU"}
	if !slices.Equal(got, want) {
		t.Errorf("build-arg key order = %v, want %v", got, want)
	}
}

func TestBuildxArgs_CallerSuppliedRevisionWinsOverEnv(t *testing.T) {
	t.Setenv("GIT_REVISION", "env-value")
	tp, df, tgt, plats, bin, bd, _, repo, name, tag, push := buildxArgsFixture()
	ex := map[string]string{"GIT_REVISION": "caller-value"}
	args, _, _ := buildxBuildArgs(tp, df, tgt, plats, bin, bd, ex, repo, name, tag, push)
	if !slices.Contains(args, "GIT_REVISION=caller-value") {
		t.Errorf("caller-supplied GIT_REVISION should win: %v", args)
	}
	if slices.Contains(args, "GIT_REVISION=env-value") {
		t.Errorf("env GIT_REVISION leaked through: %v", args)
	}
}

func TestBuildxArgs_EnvRevisionUsedWhenCallerSilent(t *testing.T) {
	t.Setenv("GIT_REVISION", "env-value")
	tp, df, tgt, plats, bin, bd, _, repo, name, tag, push := buildxArgsFixture()
	args, _, _ := buildxBuildArgs(tp, df, tgt, plats, bin, bd, nil, repo, name, tag, push)
	if !slices.Contains(args, "GIT_REVISION=env-value") {
		t.Errorf("env GIT_REVISION should be picked up: %v", args)
	}
}

func TestBuildxArgs_ImageRefAndShortName(t *testing.T) {
	tp, df, tgt, plats, bin, bd, ex, _, _, _, push := buildxArgsFixture()
	args, image, shortName := buildxBuildArgs(tp, df, tgt, plats, bin, bd, ex, "ghcr.io/threeport", "threeport-rest-api", "v0.7.0", push)
	if got, want := image, "ghcr.io/threeport/threeport-rest-api:v0.7.0"; got != want {
		t.Errorf("image = %q, want %q", got, want)
	}
	if got, want := shortName, "rest-api"; got != want {
		t.Errorf("shortName = %q, want %q", got, want)
	}
	if !slices.Contains(args, "ghcr.io/threeport/threeport-rest-api:v0.7.0") {
		t.Errorf("image ref missing from args: %v", args)
	}
}

func TestBuildxArgs_ContextAndDockerfilePathsJoined(t *testing.T) {
	_, _, tgt, plats, bin, _, ex, repo, name, tag, push := buildxArgsFixture()
	args, _, _ := buildxBuildArgs("/repo", "subdir/Dockerfile", tgt, plats, bin, "bin", ex, repo, name, tag, push)
	if got := args[len(args)-1]; got != "/repo/bin" {
		t.Errorf("trailing context dir = %q, want /repo/bin", got)
	}
	fIdx := slices.Index(args, "-f")
	if fIdx < 0 || fIdx+1 >= len(args) || args[fIdx+1] != "/repo/subdir/Dockerfile" {
		t.Errorf("expected -f /repo/subdir/Dockerfile, got args: %v", args)
	}
}
