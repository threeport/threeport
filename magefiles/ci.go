package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	mg "github.com/magefile/mage/mg"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Ci provides a type for methods that emit values for CI workflow steps.
type Ci mg.Namespace

// Env prints KEY=value lines for the workflow to append to GITHUB_ENV. It emits
// only the values that non-mage steps consume: GOFLAGS, the memory-derived
// go-build worker count inherited by the non-mage go test step and by
// goreleaser's per-target compiles, and GORELEASER_PARALLELISM, a quarter of
// that worker count, the number of whole-tree targets goreleaser builds at once
// since each links the full tree. mage's own build targets self-derive their
// parallelism, so no build-worker count is emitted for them.
func (Ci) Env() error {
	fmt.Printf("GOFLAGS=-p=%d\n", util.BuildParallelism())
	fmt.Printf("GORELEASER_PARALLELISM=%d\n", util.ReleaseParallelism())
	return nil
}

// Teardown removes what an integration job leaves behind so the next run on
// the same node starts against an empty dind. It is gated on CI=true, so
// running `mage ci:teardown` locally does nothing and a developer keeps their
// kind clusters, tptctl config, and registry.
//
// Order matters. Graceful control-plane teardown comes first, while tptctl can
// still reach its own state files. Then every kind cluster is force-deleted:
// `--all` catches whatever this run created, and kind cluster names have
// drifted across releases. Then any threeport-* control-plane container with a
// restart-on-failure policy is removed by hand, because kind stops those with
// docker stop and docker's restart policy resurrects them when the next dind
// starts against the shared /opt/dind-storage hostPath. That was the observed
// "cluster already exists" failure on repeated self-hosted runs. Then networks,
// anonymous volumes, and stopped containers are reaped. Not `docker system
// prune -a`, which wipes the roughly 13 GB image layer cache the shared
// hostPath exists to preserve.
func (Ci) Teardown() error {
	if os.Getenv("CI") != "true" {
		fmt.Println("ci:teardown: not running in CI, skipping")
		return nil
	}

	// take the control plane down gracefully, best effort. the binary is the
	// one build:tptctl produces, and the name is the one test:up creates
	teardownStep("./bin/tptctl", "down", "-n", testControlPlaneName)

	// force-delete every kind cluster the shared dind knows about, since the
	// cluster name has changed across releases
	teardownStep("kind", "delete", "clusters", "--all")

	// remove by hand any container labelled a kind-cluster node or named
	// threeport-*, which survives when kind is killed mid-flight, when dind
	// loses track of it, or when a restart policy beats kind to the process.
	// sh -c gets the pipe and xargs, whose -r no-ops on empty input
	teardownStep("sh", "-c",
		`docker ps -aq --filter "label=io.x-k8s.kind.cluster" | xargs -r docker rm -f`)
	teardownStep("sh", "-c",
		`docker ps -aq --filter "name=threeport-" | xargs -r docker rm -f`)

	// remove the buildkit builder this run created. The setup action names its
	// builder after a fresh uuid on every run, so the cache in its state volume
	// is written once and never read again, and the volume outlives the job
	// because a running container's volume is not prunable. Leaving them costs
	// several gigabytes per run and buys no cache hit.
	teardownStep("sh", "-c",
		`docker ps -aq --filter "name=buildx_buildkit_" | xargs -r docker rm -f`)

	// reap kind networks and stopped-container metadata, which is what the
	// auto-restart failure latches onto: a network and a mount point outlive
	// the container that owned them. Volumes are pruned further down, after
	// every container that could still be holding one is gone.
	teardownStep("docker", "network", "prune", "-f")
	teardownStep("docker", "container", "prune", "-f")

	// remove any leftover tptctl config, since a failed run leaves tptctl
	// unable to clear its own control-plane entry and the next bring-up on
	// this pod fails with an already-exists error
	if home, err := os.UserHomeDir(); err == nil {
		teardownStep("rm", "-f", filepath.Join(home, ".threeport", "config.yaml"))
	}

	// take the local registry down through the dev target so its container,
	// port, and network are all handled in one place
	if err := (Dev{}).LocalRegistryDown(); err != nil {
		fmt.Fprintf(os.Stderr, "ci:teardown: remove local registry: %v\n", err)
	}

	// prune volumes last. A prune skips any volume a container still holds, so
	// running it before the containers above are gone leaves each run's
	// registry storage on the node, and the next run adds its own.
	teardownStep("docker", "volume", "prune", "-f")

	return nil
}

// teardownStep runs a cleanup command best-effort, logging on failure so one
// failed command doesn't abort the rest of teardown.
func teardownStep(cmd string, args ...string) {
	if out, err := exec.Command(cmd, args...).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "ci:teardown: %s %v failed: %v (%s)\n",
			cmd, args, err, string(out))
	}
}
