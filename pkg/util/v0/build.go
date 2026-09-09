package v0

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// memBytesPerWorker is the available-memory budget per concurrent
// compile worker, 5 GiB. Dividing available memory by this yields a
// worker count a memory-capped runner can sustain without an
// out-of-memory kill.
const memBytesPerWorker = 1024 * 1024 * 1024 * 5

// archToken matches a GOARCH suffix on a per-arch image tag
// (<base>-amd64). Dots and hyphens fail, so a prerelease tag such as
// <base>-dev.3-amd64 is not an architecture.
var archToken = regexp.MustCompile(`^[a-z0-9]+$`)

// registryListTimeout bounds a registry /tags/list call.
const registryListTimeout = 60 * time.Second

// BuildParallelism returns the number of concurrent compile workers,
// the lesser of the CPU count and available memory divided by 5 GiB,
// floored at 1. Callers pass it as go's -p. When neither memory
// source is readable, as on a non-Linux runner, it falls back to the
// CPU count.
func BuildParallelism() int {
	// floor cpu count at one
	cpus := runtime.NumCPU()
	if cpus < 1 {
		cpus = 1
	}

	// fall back to cpus when host and cgroup memory are both unknown
	memBytes, ok := availableMemoryBytes()
	if !ok {
		return cpus
	}

	return clampWorkers(memBytes, cpus)
}

// clampWorkers returns how many compile workers memBytes can hold at
// 5 GiB each, floored at 1 and capped at cpus. Non-positive memBytes
// falls back to cpus.
func clampWorkers(memBytes int64, cpus int) int {
	if memBytes <= 0 {
		return cpus
	}
	workers := int(memBytes / memBytesPerWorker)
	if workers < 1 {
		return 1
	}
	if workers > cpus {
		return cpus
	}
	return workers
}

// ReleaseParallelism returns a quarter of the compile worker count,
// floored at 1. Goreleaser builds whole-tree targets, each of which
// links the full tree, so it cannot share the compile worker count.
func ReleaseParallelism() int {
	if p := BuildParallelism() / 4; p > 1 {
		return p
	}
	return 1
}

// availableMemoryBytes returns the smaller of host MemAvailable and
// the cgroup memory limit. The /proc/meminfo file inside a container
// reports the node's memory, not the container's. Without the cgroup
// reading, a pod sizes parallelism to the node. ok is false when
// neither host MemAvailable nor a finite cgroup limit is available.
func availableMemoryBytes() (int64, bool) {
	host, hostOK := procMemAvailable()
	limit, limitOK := cgroupMemoryLimit()
	switch {
	case hostOK && limitOK:
		if limit < host {
			return limit, true
		}
		return host, true
	case hostOK:
		return host, true
	case limitOK:
		return limit, true
	default:
		return 0, false
	}
}

// procMemAvailable reads MemAvailable from the /proc/meminfo file.
// Inside a container that file reports the node's memory, not the
// container's.
func procMemAvailable() (int64, bool) {
	contents, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	return parseMemAvailable(string(contents))
}

// cgroupMemoryLimit reads the current cgroup's memory hard limit,
// trying cgroup v2 memory.max then v1 memory.limit_in_bytes.
func cgroupMemoryLimit() (int64, bool) {
	// try cgroup v2 memory.max first
	if b, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		return parseCgroupV2Max(string(b))
	}
	// fall back to cgroup v1 memory.limit_in_bytes
	if b, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		return parseCgroupV1Limit(string(b))
	}
	return 0, false
}

// parseCgroupV2Max parses a memory.max value in bytes. The token
// "max" is the kernel's unlimited default and means no limit.
func parseCgroupV2Max(contents string) (int64, bool) {
	s := strings.TrimSpace(contents)
	if s == "max" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// parseCgroupV1Limit parses a memory.limit_in_bytes value. Values at
// or above 1<<62 are treated as unlimited. The kernel's no-limit
// placeholder is PAGE_COUNTER_MAX, near int64 max.
func parseCgroupV1Limit(contents string) (int64, bool) {
	s := strings.TrimSpace(contents)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 || v >= (1<<62) {
		return 0, false
	}
	return v, true
}

// parseMemAvailable reads the MemAvailable field from a /proc/meminfo
// snapshot. The kernel reports kilobytes; the result is in bytes.
func parseMemAvailable(contents string) (int64, bool) {
	for _, line := range strings.Split(contents, "\n") {
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}

// BuildBinaries compiles every binary for every arch with one go build
// invocation per arch. Arches run in parallel, and within each arch all
// binaries are passed to a single go build call so dependency compilation
// is shared across components. Each package dir under ./cmd/<name>
// produces bin/<arch>/<name>. CGO is disabled and GOOS is linux so the
// binary links statically for the distroless image. A
// single-element packageDirs slice is also valid and produces just that
// binary; per-image targets use this form for standalone use, and pick up
// a Go cache hit when AllImages pre-built the same package earlier.
// noCache=true passes -a to force a full rebuild ignoring Go's local
// build cache.
func BuildBinaries(
	threeportPath string,
	arches []string,
	packageDirs []string,
	noCache bool,
) error {
	// queue one go build per architecture
	tasks := make([]func() error, 0, len(arches))
	for _, a := range arches {
		arch := strings.TrimSpace(a)
		if arch == "" {
			continue
		}
		tasks = append(tasks, func() error {
			return buildArchBinaries(threeportPath, arch, packageDirs, noCache)
		})
	}
	return RunParallel(len(tasks), tasks)
}

// buildArchBinaries runs one go build for the given arch that compiles
// every package dir into bin/<arch>/<name>. Shared dependency compilation
// within the invocation means a cold build is much faster than running
// one go build per binary.
func buildArchBinaries(threeportPath, arch string, packageDirs []string, noCache bool) error {
	// create the per-arch output directory
	outDir := filepath.Join(threeportPath, "bin", arch)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outDir, err)
	}

	// -buildvcs=false skips Go's git probe. It trips on worktree-shaped
	// checkouts and some symlinked workspace layouts, and we don't
	// consume the embedded VCS info anyway (OCI labels on built images
	// carry the commit SHA via the GIT_REVISION build arg).
	args := []string{"build", "-buildvcs=false"}
	if noCache {
		args = append(args, "-a")
	}
	// size compile workers with BuildParallelism so a small CI runner
	// does not run out of memory
	args = append(args, fmt.Sprintf("-p=%d", BuildParallelism()))
	args = append(args, "-o", filepath.Join("bin", arch)+string(os.PathSeparator))
	// prefix each package dir with ./ so go build treats them as local
	// import paths rather than stdlib lookups.
	for _, dir := range packageDirs {
		if !strings.HasPrefix(dir, "./") && !strings.HasPrefix(dir, "/") {
			dir = "./" + dir
		}
		args = append(args, dir)
	}

	fmt.Printf("go %s\n", strings.Join(args, " "))

	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH="+arch,
	)
	cmd.Dir = threeportPath

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build %s binaries with output '%s': %w", arch, string(output), err)
	}

	return nil
}

// prefixWriter wraps an io.Writer and prefixes each line with the given
// string (e.g. "[rest-api]"). Buffers partial lines so the prefix always
// lands at line start. Line-level atomic on stdout/stderr so parallel
// builds don't tear individual lines.
type prefixWriter struct {
	prefix string
	out    io.Writer
	buf    bytes.Buffer
}

// Write splits incoming bytes into lines and writes each with the prefix.
// Skips lines that are only whitespace so buildx's section-separator blanks
// don't produce naked-prefix noise in the output.
func (p *prefixWriter) Write(data []byte) (int, error) {
	p.buf.Write(data)
	for {
		line, err := p.buf.ReadBytes('\n')
		if err != nil {
			// hold the partial line for the next write or flush
			p.buf.Reset()
			p.buf.Write(line)
			return len(data), nil
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(p.out, "%s %s", p.prefix, line); err != nil {
			return len(data), err
		}
	}
}

// flush writes any buffered partial line (no trailing newline) with
// the prefix and a synthetic newline. Buildx's final progress lines
// often arrive without a newline before the process exits; without
// flush they remain buffered until a newline that never comes and are
// silently dropped when the process exits.
func (p *prefixWriter) flush() error {
	if p.buf.Len() == 0 {
		return nil
	}
	line := p.buf.Bytes()
	p.buf.Reset()
	if len(bytes.TrimSpace(line)) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(p.out, "%s %s\n", p.prefix, line)
	return err
}

// multiArchBuilderName is the buildx builder created on demand for
// multi-architecture builds. The default `docker` driver does not support
// multi-platform output, so multi-arch builds route through a dedicated
// docker-container builder. Single-arch builds keep using whatever builder
// is currently active.
const multiArchBuilderName = "threeport-multi"

// multiArchBuilderMu serializes setup so concurrent workers under
// RunParallel don't race on inspect-then-create against the docker daemon.
var multiArchBuilderMu sync.Mutex

// multiArchBuilderMaxCacheSize is the BuildKit cache ceiling for the
// dedicated multi-arch builder, 20GB. The docker-container driver
// runs its own buildkit, so the docker daemon's builder garbage
// collection never reaches it, and the default cap is 60% of disk
// or 100GB.
const multiArchBuilderMaxCacheSize = "20GB"

// multiArchBuilderConfig is the buildkitd.toml written for the
// dedicated builder. gc = true enables collection; all = true lets
// the policy prune any cache record once maxUsedSpace is exceeded.
const multiArchBuilderConfig = `[worker.oci]
  gc = true
  maxUsedSpace = "` + multiArchBuilderMaxCacheSize + `"

  [[worker.oci.gcpolicy]]
    all = true
    maxUsedSpace = "` + multiArchBuilderMaxCacheSize + `"
`

// writeMultiArchBuilderConfig writes the dedicated builder's
// buildkitd config to a temp file and returns its path. buildx copies
// the content into the builder at create time, so the caller removes
// the file as soon as create returns.
func writeMultiArchBuilderConfig() (string, error) {
	file, err := os.CreateTemp("", "threeport-buildkitd-*.toml")
	if err != nil {
		return "", fmt.Errorf("failed to create buildkit config file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(multiArchBuilderConfig); err != nil {
		os.Remove(file.Name())
		return "", fmt.Errorf("failed to write buildkit config file: %w", err)
	}
	return file.Name(), nil
}

// ensureMultiArchBuilder makes sure a docker-container builder named
// threeport-multi exists, creating it with the docker-container
// driver and the cache-capped buildkitd config if inspect fails.
// Every call re-runs inspect and recreates the builder if something
// else deleted it. An existing builder keeps the policy it was
// created with; remove it to pick up a changed cache cap.
func ensureMultiArchBuilder() error {
	multiArchBuilderMu.Lock()
	defer multiArchBuilderMu.Unlock()

	// reuse the builder when inspect succeeds
	inspect := exec.Command("docker", "buildx", "inspect", multiArchBuilderName)
	if err := inspect.Run(); err == nil {
		return nil
	}

	// hand the builder its cache cap at create time, the only point buildx reads it
	configPath, err := writeMultiArchBuilderConfig()
	if err != nil {
		return err
	}
	defer os.Remove(configPath)

	// create and bootstrap the docker-container builder
	fmt.Printf("creating docker-container buildx builder %q for multi-arch builds...\n", multiArchBuilderName)
	create := exec.Command(
		"docker", "buildx", "create",
		"--name", multiArchBuilderName,
		"--driver", "docker-container",
		"--buildkitd-config", configPath,
		"--bootstrap",
	)
	create.Stdout = os.Stdout
	create.Stderr = os.Stderr
	if err := create.Run(); err != nil {
		return fmt.Errorf("docker buildx create failed: %w", err)
	}
	return nil
}

// BuildImage packages a pre-built binary into a container image via
// docker buildx, optionally pushing it to a registry or loading it into
// a local kind cluster.
//
// Build context. Binary inputs are expected at
// <threeportPath>/<binDir>/<arch>/<binary> for each arch in `arch`. The
// docker context is set to <threeportPath>/<binDir>, so the Dockerfile's
// `COPY ${TARGETARCH}/${BINARY}` resolves per platform during multi-arch
// builds. dockerfilePath is resolved relative to threeportPath.
//
// Architectures. `arch` is a comma-separated list (e.g. "amd64" or
// "amd64,arm64"); each entry is prefixed with "linux/" to form
// --platform. Multi-arch builds route through a dedicated
// docker-container builder named threeport-multi, created on first use
// if absent. Multi-arch builds require pushImage: this builder uses
// the docker-container driver, and --load of a multi-platform result
// still needs the containerd image store.
//
// Push vs load. pushImage and loadImage are mutually exclusive.
// Multi-arch builds require pushImage because buildx cannot --load
// multiple platforms into a single docker daemon. loadImage drives
// `kind load docker-image` against loadClusterName after the build.
//
// Build args. BINARY is always set from the `binary` parameter.
// GIT_REVISION, GIT_TAG, and BUILD_CREATED take a caller extra if
// present, then the env var of the same name, then git probes / a current-time
// timestamp, so locally-built images carry the same OCI labels as CI
// builds. extraBuildArgs pass through verbatim for per-target args like
// TERRAFORM_VERSION or PULUMI_VERSION; keys are emitted in sorted order
// for stable command output.
//
// Output. Stdout and stderr lines are prefixed with `[shortName]` so
// concurrent matrix builds remain distinguishable in the combined stream.
func BuildImage(
	threeportPath string,
	dockerfilePath string,
	target string,
	arch string,
	binary string,
	binDir string,
	extraBuildArgs map[string]string,
	imageRepo string,
	imageName string,
	imageTag string,
	pushImage bool,
	loadImage bool,
	loadClusterName string,
) error {
	// parse arch list into linux/<arch> platforms and validate push/load combos
	platforms := []string{}
	for _, a := range strings.Split(arch, ",") {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		platforms = append(platforms, "linux/"+a)
	}
	if len(platforms) == 0 {
		return errors.New("--arch is required")
	}
	if len(platforms) > 1 && !pushImage {
		return fmt.Errorf(
			"multi-arch builds require --push (--load only works for a single platform); got --arch=%s",
			arch,
		)
	}
	if pushImage && loadImage {
		return errors.New("--push and --load are mutually exclusive")
	}

	// prepare the multi-arch builder once before exec; the arg helper
	// does not create it, so this side effect lives here in the caller
	if len(platforms) > 1 {
		if err := ensureMultiArchBuilder(); err != nil {
			return fmt.Errorf("failed to prepare multi-arch builder: %w", err)
		}
	}

	// assemble the buildx argv, image ref, and log prefix
	args, image, shortName := buildxBuildArgs(
		threeportPath, dockerfilePath, target,
		platforms, binary, binDir,
		extraBuildArgs,
		imageRepo, imageName, imageTag,
		pushImage,
	)

	// run buildx with prefixed stdout/stderr. Arch isn't in the prefix
	// because buildx's own per-step labels already identify the
	// platform; only the short component name is needed for
	// disambiguation across concurrent matrix cells.
	prefix := fmt.Sprintf("[%s]", shortName)
	stdoutPrefixer := &prefixWriter{prefix: prefix, out: os.Stdout}
	stderrPrefixer := &prefixWriter{prefix: prefix, out: os.Stderr}
	dockerBuildCmd := exec.Command("docker", args...)
	dockerBuildCmd.Stdout = stdoutPrefixer
	var stderrBuf bytes.Buffer
	dockerBuildCmd.Stderr = io.MultiWriter(stderrPrefixer, &stderrBuf)
	runErr := dockerBuildCmd.Run()

	// drain partial trailing lines. buildx's final progress lines
	// often arrive without a newline and would be silently dropped on
	// process exit.
	_ = stdoutPrefixer.flush()
	_ = stderrPrefixer.flush()

	if runErr != nil {
		// surface the most common multi-arch setup gap as actionable
		// guidance instead of a raw buildx error
		if len(platforms) > 1 && strings.Contains(stderrBuf.String(), "Multi-platform build is not supported for the docker driver") {
			return fmt.Errorf(
				"image build failed for %s: the active buildx builder uses the `docker` driver, which can't do multi-arch builds. Create and switch to a docker-container builder with:\n  docker buildx create --name threeport-multi --driver docker-container --bootstrap --use\nThen retry. Underlying error: %w",
				image, runErr,
			)
		}
		return fmt.Errorf("image build failed for %s: %w", image, runErr)
	}

	if pushImage {
		fmt.Printf("%s built and pushed\n", prefix)
	} else {
		fmt.Printf("%s built\n", prefix)
	}

	// optionally load into a local kind cluster
	if loadImage {
		kindLoadCmd := exec.Command(
			"kind",
			"load",
			"docker-image",
			image,
			"--name",
			loadClusterName,
		)
		output, err := kindLoadCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf(
				"failed to load image %s to kind cluster with output '%s': %w",
				image,
				string(output),
				err,
			)
		}
		fmt.Printf("%s image loaded to kind cluster\n", image)
	}

	return nil
}

// buildxBuildArgs assembles the docker buildx invocation argv, the full
// image ref, and the short component name used for log prefixes. Reads
// GIT_REVISION, GIT_TAG, and BUILD_CREATED for OCI image labels; an unset
// GIT_TAG falls back to the image tag, the others to git probes and the
// current time.
func buildxBuildArgs(
	threeportPath string,
	dockerfilePath string,
	target string,
	platforms []string,
	binary string,
	binDir string,
	extraBuildArgs map[string]string,
	imageRepo string,
	imageName string,
	imageTag string,
	pushImage bool,
) (args []string, image string, shortName string) {
	image = fmt.Sprintf("%s/%s:%s", imageRepo, imageName, imageTag)
	shortName = strings.TrimPrefix(imageName, "threeport-")

	args = []string{"buildx", "build"}

	// route multi-arch through the dedicated docker-container builder; the
	// default docker driver can't emit a multi-platform manifest
	if len(platforms) > 1 {
		args = append(args, "--builder", multiArchBuilderName)
	}

	// push to a registry or load into the local docker daemon
	if pushImage {
		args = append(args, "--push")
	} else {
		args = append(args, "--load")
	}

	args = append(args, fmt.Sprintf("--platform=%s", strings.Join(platforms, ",")))

	if target != "" {
		args = append(args, "--target", target)
	}

	// build args: BINARY always; OCI labels from env or fallbacks;
	// caller extras emitted in sorted order so command output stays
	// stable across runs
	args = append(args, "--build-arg", fmt.Sprintf("BINARY=%s", binary))
	// copy the caller's extras before filling in the label defaults, so the
	// resolved labels do not land in a map the caller still holds
	buildArgs := make(map[string]string, len(extraBuildArgs)+3)
	for k, v := range extraBuildArgs {
		buildArgs[k] = v
	}
	resolveLabelArg(buildArgs, "GIT_REVISION", func() string {
		return gitOutput(threeportPath, "rev-parse", "HEAD")
	})
	resolveLabelArg(buildArgs, "GIT_TAG", func() string {
		// fall back to the image tag, then git describe; an empty
		// result is omitted so the Dockerfile ARG default applies
		if imageTag != "" {
			return imageTag
		}
		return gitOutput(threeportPath, "describe", "--tags", "--always", "--dirty")
	})
	resolveLabelArg(buildArgs, "BUILD_CREATED", func() string {
		return time.Now().UTC().Format(time.RFC3339)
	})
	keys := make([]string, 0, len(buildArgs))
	for k := range buildArgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", k, buildArgs[k]))
	}

	// plain progress keeps lines independent so concurrent builds
	// interleave cleanly under per-component prefixes
	args = append(args, "--progress=plain")

	// build context is the per-arch bin root; Dockerfile is referenced from the repo root via -f
	contextDir := filepath.Join(threeportPath, binDir)
	dockerfile := filepath.Join(threeportPath, dockerfilePath)
	args = append(args, "-t", image, "-f", dockerfile, contextDir)

	return args, image, shortName
}

// resolveLabelArg fills key in args from an env var of the same name,
// or from the fallback closure if the env var is unset. An existing
// caller-supplied value wins over both. Empty results are dropped so
// the Dockerfile ARG default takes over.
func resolveLabelArg(args map[string]string, key string, fallback func() string) {
	if _, ok := args[key]; ok {
		return
	}
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		args[key] = v
		return
	}
	if v := strings.TrimSpace(fallback()); v != "" {
		args[key] = v
	}
}

// gitOutput runs git in workingDir with the given args and returns
// trimmed stdout, or "" if git fails. Used to derive label values for
// local builds where the caller hasn't set GIT_REVISION/GIT_TAG.
func gitOutput(workingDir string, args ...string) string {
	out, err := gitCommand(workingDir, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// DiscoverArches lists tags at imageRef and returns the architecture
// suffixes of tags matching <baseTag>-<arch>, sorted. imageRef is a
// repository with no tag. The result covers whichever per-arch images
// the registry already holds.
func DiscoverArches(imageRef, baseTag string) ([]string, error) {
	// allow HTTP for a loopback registry that has no TLS
	opts := []name.Option{}
	if registryAllowsHTTP(imageRef) {
		opts = append(opts, name.Insecure)
	}
	repo, err := name.NewRepository(imageRef, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image repository %q: %w", imageRef, err)
	}
	// a hung list holds up every other component's manifest stitch
	ctx, cancel := context.WithTimeout(context.Background(), registryListTimeout)
	defer cancel()
	// list tags via the registry's /v2/<name>/tags/list
	tags, err := remote.List(repo, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to list tags for %q: %w", imageRef, err)
	}
	return archSuffixes(tags, baseTag), nil
}

// registryAllowsHTTP reports whether imageRef names a loopback host
// so the caller can pass name.Insecure. HTTP is already used for
// localhost with a port, 127.0.0.1, and ::1; Insecure covers bare
// localhost.
func registryAllowsHTTP(imageRef string) bool {
	// take the host[:port] before the first slash
	host := imageRef
	if i := strings.Index(imageRef, "/"); i >= 0 {
		host = imageRef[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// archSuffixes returns the sorted GOARCH suffixes of tags of the form
// <baseTag>-<arch>. A suffix that is not a lowercase alphanumeric
// token is dropped, so a prerelease tag does not stitch into a
// release manifest and publish under a release tag.
func archSuffixes(tags []string, baseTag string) []string {
	prefix := baseTag + "-"
	arches := []string{}
	for _, tag := range tags {
		suffix := strings.TrimPrefix(tag, prefix)
		if suffix == tag || !archToken.MatchString(suffix) {
			continue
		}
		arches = append(arches, suffix)
	}
	sort.Strings(arches)
	return arches
}

// ParseArches splits a comma-separated arch string into a clean slice,
// trimming whitespace from each entry and dropping empties.
func ParseArches(arch string) []string {
	out := []string{}
	for _, a := range strings.Split(arch, ",") {
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

// imagetoolsArgs builds the docker buildx imagetools create argv that
// tags <repo>/<image>:<tag> as a manifest list of the per-arch sources
// <repo>/<image>:<tag>-<arch>. An empty arches slice is an error,
// since a manifest needs at least one source.
func imagetoolsArgs(repo, image, tag string, arches []string) (args []string, target string, err error) {
	if len(arches) == 0 {
		return nil, "", fmt.Errorf("failed to find per-arch tags for %s/%s:%s", repo, image, tag)
	}

	// build the canonical target tag and the per-arch source tags
	target = fmt.Sprintf("%s/%s:%s", repo, image, tag)
	sources := make([]string, 0, len(arches))
	for _, a := range arches {
		sources = append(sources, fmt.Sprintf("%s/%s:%s-%s", repo, image, tag, a))
	}

	// assemble the buildx imagetools create invocation with --append
	args = []string{"buildx", "imagetools", "create", "--tag", target}
	args = append(args, sources...)
	return args, target, nil
}

// PushMultiArchManifest stitches per-arch image tags into a multi-arch
// manifest list and pushes the result under the canonical tag. Sources
// are assumed to already exist at <repo>/<image>:<tag>-<arch> for each
// arch in the comma-separated arches list; the result publishes to
// <repo>/<image>:<tag>. Implemented via `docker buildx imagetools
// create`, which reads the source manifests from the registry and
// writes a fan-in manifest list back without re-uploading any blobs.
func PushMultiArchManifest(imageRepo, imageName, imageTag, arches string) error {
	args, target, err := imagetoolsArgs(imageRepo, imageName, imageTag, ParseArches(arches))
	if err != nil {
		return err
	}

	// run with prefixed stdout/stderr so concurrent component runs
	// stay disambiguated in interleaved CI output
	shortName := strings.TrimPrefix(imageName, "threeport-")
	prefix := fmt.Sprintf("[%s]", shortName)
	stdoutPrefixer := &prefixWriter{prefix: prefix, out: os.Stdout}
	stderrPrefixer := &prefixWriter{prefix: prefix, out: os.Stderr}

	cmd := exec.Command("docker", args...)
	cmd.Stdout = stdoutPrefixer
	cmd.Stderr = stderrPrefixer
	runErr := cmd.Run()

	// drain partial trailing lines so a final no-newline write isn't
	// silently dropped on process exit
	_ = stdoutPrefixer.flush()
	_ = stderrPrefixer.flush()

	if runErr != nil {
		return fmt.Errorf("failed to create multi-arch manifest %s: %w", target, runErr)
	}
	return nil
}
