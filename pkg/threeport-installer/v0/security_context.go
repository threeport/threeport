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
