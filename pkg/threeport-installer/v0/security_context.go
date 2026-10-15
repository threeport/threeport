package v0

// The user and group every threeport image runs as.
//
// gcr.io/distroless/static:nonroot already defaults to this and the
// Dockerfile restates it with USER 65532:65532. It is set on the pod as well
// so the cluster enforces it rather than trusting the image: a base image
// whose default UID changed would otherwise move the control plane to a
// different user without anything failing.
const (
	nonRootUID = int64(65532)
	nonRootGID = int64(65532)
)

// restrictedContainerSecurityContext is what every threeport-authored
// container runs with.
//
// These are the container-level requirements of the Kubernetes restricted
// Pod Security Standard. The images are distroless with no shell and no
// package manager, which removes most of what an attacker would reach for;
// this removes the rest of the escalation paths, and does it at the cluster
// level where it can be audited rather than inside an image that has to be
// trusted.
//
// readOnlyRootFilesystem goes beyond the restricted standard, which does not
// require it. Several components do write at runtime - helm to its cache,
// terraform and pulumi to their working directories - so each container is
// given writableVolumeMounts() for exactly those paths and nothing else. The
// binary, the CLI tools layered beside it and everything else in the image
// are then immutable for the life of the pod.
func restrictedContainerSecurityContext() map[string]interface{} {
	return map[string]interface{}{
		"allowPrivilegeEscalation": false,
		"capabilities": map[string]interface{}{
			"drop": []interface{}{"ALL"},
		},
		"runAsNonRoot":           true,
		"runAsUser":              nonRootUID,
		"runAsGroup":             nonRootGID,
		"readOnlyRootFilesystem": true,
	}
}

// writableVolumeName is the single emptyDir every container writes through.
const writableVolumeName = "writable"

// writableVolume is the volume behind every writable path below.
//
// One volume with a subPath per mount rather than one volume per path: they
// are all scratch for the life of the pod, and a list of four emptyDirs
// invites the reader to look for a difference between them.
func writableVolume() map[string]interface{} {
	return map[string]interface{}{
		"name":     writableVolumeName,
		"emptyDir": map[string]interface{}{},
	}
}

// writableVolumeMounts are the paths a read-only root filesystem has to leave
// writable.
//
// Every container gets all of them rather than the ones its own image needs.
// Which paths a component writes to is decided by the image variant it is
// built from - plain, helm, terraform or pulumi - and that is not recorded on
// the component this installer is handed, so selecting per component would
// mean matching on names. A name that stopped matching would not fail at
// install: the container would start, run until it first wrote something, and
// fail there.
//
// /var/helm is mounted whole rather than at the two directories the image
// creates under it. Helm's repository config is written directly there, and
// mounting only the leaves leaves that file on the read-only root - which
// fails at the first chart rather than at install. The pre-created
// directories are not lost by mounting over them, because nothing relies on
// them surviving: the helm workload controller calls MkdirAll on the config
// directory and RemoveAll on the cache directory itself.
func writableVolumeMounts() []interface{} {
	mount := func(path, subPath string) map[string]interface{} {
		return map[string]interface{}{
			"name":      writableVolumeName,
			"mountPath": path,
			"subPath":   subPath,
		}
	}

	return []interface{}{
		// temporary files: terraform's working directories, helm's rendered
		// values, and anything the Go runtime writes
		mount("/tmp", "tmp"),
		// the home directory: pulumi's state and plugin cache, gcloud's
		// application default credentials, the OCI config file
		mount("/home/nonroot", "home"),
		// helm's repository config, cache and data, all of which live here
		mount("/var/helm", "helm"),
	}
}

// restrictedPodSecurityContext is the pod-level half.
//
// seccompProfile belongs here rather than on each container: it is the one
// restricted requirement that a pod sets once for everything it runs,
// including containers this installer did not write.
func restrictedPodSecurityContext() map[string]interface{} {
	return map[string]interface{}{
		"runAsNonRoot": true,
		"runAsUser":    nonRootUID,
		"runAsGroup":   nonRootGID,
		"seccompProfile": map[string]interface{}{
			"type": "RuntimeDefault",
		},
	}
}

// addWritablePaths adds the writable volume and whichever of its mounts the
// caller has not already provided.
//
// A component can bring its own volumes through AdditionalVolumes and
// AdditionalVolumeMounts, and a mount path has to be unique within a
// container. Two mounts at /tmp is not a conflict Kubernetes resolves, it is
// a deployment it refuses outright - so a module that mounts one of these
// paths would not get a worse filesystem, it would fail to install at all.
//
// The caller's mount wins where they overlap. It was asked for explicitly,
// and it is already writable storage, which is the whole requirement.
func addWritablePaths(vols, volMounts []interface{}) ([]interface{}, []interface{}) {
	// The volume is declared whether or not this container needs a mount from
	// it. It belongs to the pod, and the other containers in that pod mount
	// it on their own account: the api server's init containers and the
	// agent's vendored sidecar both do. Tying the declaration to one
	// container's needs leaves those mounts naming a volume that is not
	// there, which Kubernetes refuses with Not found: "writable" - so a
	// component that mounted all three paths itself would take the whole
	// deployment down with it.
	//
	// A caller that already has a volume under this name keeps theirs; the
	// mounts then use it.
	declared := false
	for _, raw := range vols {
		volume, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := volume["name"].(string); ok && name == writableVolumeName {
			declared = true
			break
		}
	}
	if !declared {
		vols = append(vols, writableVolume())
	}

	return vols, append(volMounts, missingWritableMounts(volMounts)...)
}

// missingWritableMounts returns the writable mounts a container does not
// already have, so a path the caller mounted is not mounted twice.
func missingWritableMounts(volMounts []interface{}) []interface{} {
	mounted := map[string]bool{}
	for _, raw := range volMounts {
		mount, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if path, ok := mount["mountPath"].(string); ok {
			mounted[path] = true
		}
	}

	var missing []interface{}
	for _, raw := range writableVolumeMounts() {
		path, _ := raw.(map[string]interface{})["mountPath"].(string)
		if mounted[path] {
			continue
		}
		missing = append(missing, raw)
	}

	return missing
}
