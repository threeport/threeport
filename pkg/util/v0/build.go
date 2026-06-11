package v0

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// BuildBinaries compiles every binary for every arch with one go build
// invocation per arch. Arches run in parallel, and within each arch all
// binaries are passed to a single go build call so dependency compilation
// is shared across components. Each package dir under ./cmd/<name>
// produces bin/<arch>/<name>. CGO is disabled for cross-compile. A
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
	// PARALLEL_GO_BUILD caps concurrent compile workers per go build
	// invocation. CI sets it (e.g. 2) to keep memory bounded on small
	// runners; locally we leave it unset so the Go default (GOMAXPROCS)
	// uses every available core.
	if p := os.Getenv("PARALLEL_GO_BUILD"); p != "" {
		args = append(args, "-p="+p)
	}
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

// ensureMultiArchBuilder makes sure a docker-container builder named
// multiArchBuilderName exists. The mutex serializes setup across
// goroutines so concurrent parallel image builds don't all race to
// create the builder and have all but one fail. Every call re-runs
// `docker buildx inspect` so external deletion of the builder
// (e.g. `docker buildx rm threeport-multi` in another terminal, or a
// Docker Desktop reset) is recovered transparently on the next call.
func ensureMultiArchBuilder() error {
	multiArchBuilderMu.Lock()
	defer multiArchBuilderMu.Unlock()

	inspect := exec.Command("docker", "buildx", "inspect", multiArchBuilderName)
	if err := inspect.Run(); err == nil {
		return nil
	}
	fmt.Printf("creating docker-container buildx builder %q for multi-arch builds...\n", multiArchBuilderName)
	create := exec.Command(
		"docker", "buildx", "create",
		"--name", multiArchBuilderName,
		"--driver", "docker-container",
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
// if absent. The default `docker` driver does not support multi-platform
// output.
//
// Push vs load. pushImage and loadImage are mutually exclusive.
// Multi-arch builds require pushImage because buildx cannot --load
// multiple platforms into a single docker daemon. loadImage drives
// `kind load docker-image` against loadClusterName after the build.
//
// Build args. BINARY is always set from the `binary` parameter.
// GIT_REVISION, GIT_TAG, and BUILD_CREATED are filled from env vars of
// the same name first and fall back to git probes / a current-time
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
	// stays pure so this side effect lives here in the caller
	if len(platforms) > 1 {
		if err := ensureMultiArchBuilder(); err != nil {
			return fmt.Errorf("failed to prepare multi-arch builder: %w", err)
		}
	}

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
// GIT_REVISION / GIT_TAG / BUILD_CREATED for OCI image labels, falling
// back to git probes for the unset label values. Emits no other side
// effects.
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
	if extraBuildArgs == nil {
		extraBuildArgs = map[string]string{}
	}
	resolveLabelArg(extraBuildArgs, "GIT_REVISION", func() string {
		return gitOutput(threeportPath, "rev-parse", "HEAD")
	})
	resolveLabelArg(extraBuildArgs, "GIT_TAG", func() string {
		return gitOutput(threeportPath, "describe", "--tags", "--always", "--dirty")
	})
	resolveLabelArg(extraBuildArgs, "BUILD_CREATED", func() string {
		return time.Now().UTC().Format(time.RFC3339)
	})
	keys := make([]string, 0, len(extraBuildArgs))
	for k := range extraBuildArgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", k, extraBuildArgs[k]))
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
	cmd := exec.Command("git", append([]string{"-C", workingDir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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

// PushMultiArchManifest stitches per-arch image tags into a multi-arch
// manifest list and pushes the result under the canonical tag. Sources
// are assumed to already exist at <repo>/<image>:<tag>-<arch> for each
// arch in the comma-separated arches list; the result publishes to
// <repo>/<image>:<tag>. Implemented via `docker buildx imagetools
// create`, which reads the source manifests from the registry and
// writes a fan-in manifest list back without re-uploading any blobs.
func PushMultiArchManifest(imageRepo, imageName, imageTag, arches string) error {
	archList := ParseArches(arches)
	if len(archList) == 0 {
		return errors.New("--arches is required")
	}

	// build the canonical target tag and the per-arch source tags
	target := fmt.Sprintf("%s/%s:%s", imageRepo, imageName, imageTag)
	sources := make([]string, 0, len(archList))
	for _, a := range archList {
		sources = append(sources, fmt.Sprintf("%s/%s:%s-%s", imageRepo, imageName, imageTag, a))
	}

	// assemble the buildx imagetools create invocation
	args := []string{"buildx", "imagetools", "create", "--tag", target}
	args = append(args, sources...)

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
