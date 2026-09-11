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

// TestBuildxArgs_LeavesTheCallerMapAlone asserts the resolved label defaults
// do not land in the caller's extra build-arg map.
func TestBuildxArgs_LeavesTheCallerMapAlone(t *testing.T) {
	// GIT_REVISION in the environment
	t.Setenv("GIT_REVISION", "env-value")
	tp, df, tgt, plats, bin, bd, _, repo, name, _, push := buildxArgsFixture()
	// a caller map holding one extra and none of the labels
	ex := map[string]string{"TERRAFORM_VERSION": "1.9.0"}
	// resolve labels against that extras map
	buildxBuildArgs(tp, df, tgt, plats, bin, bd, ex, repo, name, "v0.7.0", push)
	// the map still holds only what the caller put in it
	if len(ex) != 1 || ex["TERRAFORM_VERSION"] != "1.9.0" {
		t.Errorf("caller map = %v, want only TERRAFORM_VERSION=1.9.0", ex)
	}
}

// TestBuildxArgs_ImageRefAndShortName covers buildxBuildArgs returning the
// full image ref and the short component name, and putting the ref in argv.
func TestBuildxArgs_ImageRefAndShortName(t *testing.T) {
	tp, df, tgt, plats, bin, bd, ex, _, _, _, push := buildxArgsFixture()
	// assemble args for threeport-rest-api at v0.7.0
	args, image, shortName := buildxBuildArgs(tp, df, tgt, plats, bin, bd, ex, "ghcr.io/threeport", "threeport-rest-api", "v0.7.0", push)
	// the full registry/name:tag
	if got, want := image, "ghcr.io/threeport/threeport-rest-api:v0.7.0"; got != want {
		t.Errorf("image = %q, want %q", got, want)
	}
	// the short component name
	if got, want := shortName, "rest-api"; got != want {
		t.Errorf("shortName = %q, want %q", got, want)
	}
	// the same ref in the buildx argv
	if !slices.Contains(args, "ghcr.io/threeport/threeport-rest-api:v0.7.0") {
		t.Errorf("image ref missing from args: %v", args)
	}
}

// TestBuildxArgs_ContextAndDockerfilePathsJoined covers buildxBuildArgs
// joining the repo path onto the dockerfile and the build context.
func TestBuildxArgs_ContextAndDockerfilePathsJoined(t *testing.T) {
	_, _, tgt, plats, bin, _, ex, repo, name, tag, push := buildxArgsFixture()
	// a dockerfile under a subdir and a bin context dir
	args, _, _ := buildxBuildArgs("/repo", "subdir/Dockerfile", tgt, plats, bin, "bin", ex, repo, name, tag, push)
	// trailing context dir
	if got := args[len(args)-1]; got != "/repo/bin" {
		t.Errorf("trailing context dir = %q, want /repo/bin", got)
	}
	// -f dockerfile path
	fIdx := slices.Index(args, "-f")
	if fIdx < 0 || fIdx+1 >= len(args) || args[fIdx+1] != "/repo/subdir/Dockerfile" {
		t.Errorf("expected -f /repo/subdir/Dockerfile, got args: %v", args)
	}
}

// TestBuildParallelismIsPositive covers BuildParallelism returning at least one
// worker.
func TestBuildParallelismIsPositive(t *testing.T) {
	// derive a worker count for the current runner
	if got := BuildParallelism(); got < 1 {
		t.Errorf("BuildParallelism() = %d, want >= 1", got)
	}
}

// TestParseMemAvailableReadsKilobytesToBytes covers parseMemAvailable
// converting a MemAvailable kilobyte value to bytes.
func TestParseMemAvailableReadsKilobytesToBytes(t *testing.T) {
	// a /proc/meminfo snippet carrying a well-formed MemAvailable line
	contents := "MemTotal:       32660320 kB\nMemAvailable:   16331640 kB\nBuffers:          123456 kB\n"
	// parse the available-memory line
	got, ok := parseMemAvailable(contents)
	// parse succeeds
	if !ok {
		t.Fatalf("parseMemAvailable reported failure on a valid line")
	}
	// the kilobyte value converts to bytes
	if want := int64(16331640) * 1024; got != want {
		t.Errorf("parseMemAvailable = %d, want %d", got, want)
	}
}

// TestParseMemAvailableRejectsMissingAndMalformed covers parseMemAvailable
// rejecting a missing line, too few fields, and a non-integer value.
func TestParseMemAvailableRejectsMissingAndMalformed(t *testing.T) {
	// each case is a /proc/meminfo snippet that yields no usable value
	cases := []struct {
		name     string
		contents string
	}{
		// no MemAvailable line at all
		{"no MemAvailable line", "MemTotal:       32660320 kB\nBuffers: 123456 kB\n"},
		// the line has fewer than two whitespace-separated fields
		{"too few fields", "MemAvailable:\n"},
		// the value column is not an integer
		{"non-integer value", "MemAvailable:   sixteen kB\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// parse the unusable snippet
			got, ok := parseMemAvailable(c.contents)
			// a missing or malformed field reports (0, false)
			if ok || got != 0 {
				t.Errorf("parseMemAvailable(%q) = (%d, %v), want (0, false)", c.contents, got, ok)
			}
		})
	}
}

// TestParseCgroupV2MaxReadsBytesAndRejectsMax covers parseCgroupV2Max
// accepting a numeric byte limit and rejecting max, zero, and malformed values.
func TestParseCgroupV2MaxReadsBytesAndRejectsMax(t *testing.T) {
	// each case pairs a memory.max snippet with its expected (value, ok)
	cases := []struct {
		name     string
		contents string
		want     int64
		wantOK   bool
	}{
		// a numeric limit comes back as a byte count
		{"numeric limit", "8589934592\n", 8589934592, true},
		// surrounding whitespace and a missing trailing newline are tolerated
		{"trimmed whitespace", "  8589934592  ", 8589934592, true},
		// the max token means no limit
		{"max means unlimited", "max\n", 0, false},
		// non-numeric content reports false
		{"non-numeric", "eight gigs\n", 0, false},
		// a non-positive value is treated as no limit
		{"zero rejected", "0\n", 0, false},
		// empty contents report false
		{"empty", "", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// parse the memory.max contents
			got, ok := parseCgroupV2Max(c.contents)
			if got != c.want || ok != c.wantOK {
				t.Errorf("parseCgroupV2Max(%q) = (%d, %v), want (%d, %v)", c.contents, got, ok, c.want, c.wantOK)
			}
		})
	}
}

// TestParseCgroupV1LimitReadsBytesAndRejectsUnlimited covers parseCgroupV1Limit
// accepting a numeric byte limit and rejecting unlimited and malformed values.
func TestParseCgroupV1LimitReadsBytesAndRejectsUnlimited(t *testing.T) {
	// each case pairs a memory.limit_in_bytes snippet with its expected (value, ok)
	cases := []struct {
		name     string
		contents string
		want     int64
		wantOK   bool
	}{
		// a numeric limit comes back as a byte count
		{"numeric limit", "8589934592\n", 8589934592, true},
		// surrounding whitespace is tolerated
		{"trimmed whitespace", "  8589934592  ", 8589934592, true},
		// a value near int64 max means no limit
		{"near int64 max means unlimited", "9223372036854771712\n", 0, false},
		// any value at or above 1<<62 is treated as unlimited
		{"at one-shifted-62 rejected", "4611686018427387904\n", 0, false},
		// non-numeric content reports false
		{"non-numeric", "eight gigs\n", 0, false},
		// a non-positive value is treated as no limit
		{"zero rejected", "0\n", 0, false},
		// empty contents report false
		{"empty", "", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// parse the memory.limit_in_bytes contents
			got, ok := parseCgroupV1Limit(c.contents)
			if got != c.want || ok != c.wantOK {
				t.Errorf("parseCgroupV1Limit(%q) = (%d, %v), want (%d, %v)", c.contents, got, ok, c.want, c.wantOK)
			}
		})
	}
}

// TestClampWorkersClampsToRange covers clampWorkers flooring at one, capping
// at the CPU count, and falling back on a non-positive memory reading.
func TestClampWorkersClampsToRange(t *testing.T) {
	// each case names the clamp behavior the memory and CPU inputs exercise
	cases := []struct {
		name     string
		memBytes int64
		cpus     int
		want     int
	}{
		// less than one worker's worth of memory floors at one worker
		{"below one floors to one", memBytesPerWorker - 1, 8, 1},
		// more memory than CPUs would allow caps at the CPU count
		{"above cpus caps at cpus", memBytesPerWorker * 100, 4, 4},
		// a non-positive reading falls back to the CPU count
		{"zero memory falls back to cpus", 0, 6, 6},
		{"negative memory falls back to cpus", -1, 6, 6},
		// exactly one worker's worth of memory yields one worker
		{"exact one-worker boundary", memBytesPerWorker, 8, 1},
		// one byte under two workers' worth still yields one worker
		{"just under two workers", memBytesPerWorker*2 - 1, 8, 1},
		// one byte over two workers' worth yields two workers
		{"just over two workers", memBytesPerWorker*2 + 1, 8, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// clamp the memory-derived worker count
			if got := clampWorkers(c.memBytes, c.cpus); got != c.want {
				t.Errorf("clampWorkers(%d, %d) = %d, want %d", c.memBytes, c.cpus, got, c.want)
			}
		})
	}
}

// TestReleaseParallelismScalesAndFloors covers ReleaseParallelism returning
// a quarter of BuildParallelism, floored at 1.
func TestReleaseParallelismScalesAndFloors(t *testing.T) {
	// ReleaseParallelism on this runner
	got := ReleaseParallelism()
	// at least one worker
	if got < 1 {
		t.Errorf("ReleaseParallelism() = %d, want >= 1", got)
	}
	// a quarter of BuildParallelism, floored at 1
	build := BuildParallelism()
	want := build / 4
	if want < 1 {
		want = 1
	}
	if got != want {
		t.Errorf("ReleaseParallelism() = %d, want %d (BuildParallelism=%d)", got, want, build)
	}
	// 4/4 is 1
	if build == 4 && got != 1 {
		t.Errorf("at BuildParallelism=4, ReleaseParallelism() = %d, want 1", got)
	}
	// 5/4 is still 1
	if build == 5 && got != 1 {
		t.Errorf("at BuildParallelism=5, ReleaseParallelism() = %d, want 1", got)
	}
}

// TestArchSuffixesReturnsSortedUniqueArches covers archSuffixes returning
// matching arch suffixes in sorted order.
func TestArchSuffixesReturnsSortedUniqueArches(t *testing.T) {
	// two arch-suffixed tags under the base, in reverse sort order
	tags := []string{"v0.7.0-arm64", "v0.7.0-amd64"}
	// extract the arch suffixes
	got := archSuffixes(tags, "v0.7.0")
	// the suffixes come back sorted regardless of input order
	if want := []string{"amd64", "arm64"}; !slices.Equal(got, want) {
		t.Errorf("archSuffixes = %v, want %v", got, want)
	}
}

// TestArchSuffixesDropsNonMatchingTags covers archSuffixes dropping tags that
// lack the prefix, the bare base, and the empty-suffix base-with-dash.
func TestArchSuffixesDropsNonMatchingTags(t *testing.T) {
	// a non-prefix tag, the bare base, the dangling dash, one real arch tag, and a different-base arch tag
	tags := []string{"latest", "v0.7.0", "v0.7.0-", "v0.7.0-amd64", "other-arm64"}
	// extract the arch suffixes
	got := archSuffixes(tags, "v0.7.0")
	// only the matching amd64 suffix
	if want := []string{"amd64"}; !slices.Equal(got, want) {
		t.Errorf("archSuffixes = %v, want %v", got, want)
	}
}

// TestArchSuffixesDoesNotCrossContaminatePrefixBases covers archSuffixes
// not treating a longer base's tag as an arch of a shorter prefix base.
func TestArchSuffixesDoesNotCrossContaminatePrefixBases(t *testing.T) {
	// a GA per-arch tag and a longer-base per-arch tag
	tags := []string{"v0.7.0-amd64", "v0.7.0-dev-arm64"}
	// the longer base matches only its own arch tag
	got := archSuffixes(tags, "v0.7.0-dev")
	if want := []string{"arm64"}; !slices.Equal(got, want) {
		t.Errorf("archSuffixes(v0.7.0-dev) = %v, want %v", got, want)
	}
	// the shorter base matches only its own arch tag
	got = archSuffixes(tags, "v0.7.0")
	if want := []string{"amd64"}; !slices.Equal(got, want) {
		t.Errorf("archSuffixes(v0.7.0) = %v, want %v", got, want)
	}
}

// TestArchSuffixesDropsPrereleaseTagsFromAReleaseBase covers archSuffixes
// keeping prerelease per-arch tags off a GA base and still matching a
// prerelease base.
func TestArchSuffixesDropsPrereleaseTagsFromAReleaseBase(t *testing.T) {
	// GA per-arch tags beside several prerelease ones
	tags := []string{
		"v0.7.0-amd64",
		"v0.7.0-arm64",
		"v0.7.0-dev.3-amd64",
		"v0.7.0-dev.3-arm64",
		"v0.7.0-rc.1-amd64",
	}
	// a GA base keeps only the GA per-arch tags
	got := archSuffixes(tags, "v0.7.0")
	if want := []string{"amd64", "arm64"}; !slices.Equal(got, want) {
		t.Errorf("archSuffixes(v0.7.0) = %v, want %v", got, want)
	}
	// a prerelease base still resolves its own arch tags
	got = archSuffixes(tags, "v0.7.0-dev.3")
	if want := []string{"amd64", "arm64"}; !slices.Equal(got, want) {
		t.Errorf("archSuffixes(v0.7.0-dev.3) = %v, want %v", got, want)
	}
}

// TestArchSuffixesSortsRegardlessOfInputOrder covers archSuffixes sorting
// matching suffixes regardless of input order.
func TestArchSuffixesSortsRegardlessOfInputOrder(t *testing.T) {
	// the same arches supplied in descending order
	tags := []string{"v0.7.0-s390x", "v0.7.0-arm64", "v0.7.0-amd64"}
	// extract and sort the suffixes
	got := archSuffixes(tags, "v0.7.0")
	// the output is sorted independent of input order
	if want := []string{"amd64", "arm64", "s390x"}; !slices.Equal(got, want) {
		t.Errorf("archSuffixes = %v, want %v", got, want)
	}
}

// TestParseArchesSplitsAndCleans covers ParseArches splitting on commas,
// trimming whitespace, and dropping empty and trailing-comma entries.
func TestParseArchesSplitsAndCleans(t *testing.T) {
	// each case pairs a raw arch string with the cleaned slice it parses to
	cases := []struct {
		name string
		in   string
		want []string
	}{
		// a clean comma list splits into its two arches
		{"clean list", "amd64,arm64", []string{"amd64", "arm64"}},
		// surrounding whitespace on each entry is trimmed
		{"whitespace trimmed", " amd64 , arm64 ", []string{"amd64", "arm64"}},
		// empty entries and a trailing comma drop out
		{"empties and trailing comma dropped", "amd64,,arm64,", []string{"amd64", "arm64"}},
		// an empty string yields an empty slice
		{"empty string", "", []string{}},
		// a whitespace-only string yields an empty slice
		{"whitespace only", "   ", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// split and clean the arch string
			got := ParseArches(c.in)
			if !slices.Equal(got, c.want) {
				t.Errorf("ParseArches(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestImagetoolsArgsBuildsTargetAndSources covers imagetoolsArgs producing the
// canonical target tag and one per-arch source in input order.
func TestImagetoolsArgsBuildsTargetAndSources(t *testing.T) {
	// a repo, image, tag, and ordered arch set
	args, target, err := imagetoolsArgs("ghcr.io/threeport", "threeport-rest-api", "v0.7.0", []string{"amd64", "arm64"})
	// call succeeds
	if err != nil {
		t.Fatalf("imagetoolsArgs returned error: %v", err)
	}
	// the canonical target tag
	if want := "ghcr.io/threeport/threeport-rest-api:v0.7.0"; target != want {
		t.Errorf("target = %q, want %q", target, want)
	}
	// buildx imagetools create --tag, then one source ref per arch
	wantArgs := []string{
		"buildx", "imagetools", "create", "--tag",
		"ghcr.io/threeport/threeport-rest-api:v0.7.0",
		"ghcr.io/threeport/threeport-rest-api:v0.7.0-amd64",
		"ghcr.io/threeport/threeport-rest-api:v0.7.0-arm64",
	}
	// the per-arch sources follow the target in input order
	if !slices.Equal(args, wantArgs) {
		t.Errorf("args = %v, want %v", args, wantArgs)
	}
}

// TestImagetoolsArgsRejectsEmptyArches covers imagetoolsArgs rejecting an
// empty arch list.
func TestImagetoolsArgsRejectsEmptyArches(t *testing.T) {
	// an empty arch slice
	args, target, err := imagetoolsArgs("ghcr.io/threeport", "threeport-rest-api", "v0.7.0", nil)
	// failed to find per-arch tags
	if err == nil || !strings.Contains(err.Error(), "failed to find per-arch tags") {
		t.Fatalf("expected failed to find per-arch tags, got %v", err)
	}
	// no argv or target on the error path
	if args != nil || target != "" {
		t.Errorf("on error args = %v, target = %q, want nil and empty", args, target)
	}
}
