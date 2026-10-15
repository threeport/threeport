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
// readOnlyRootFilesystem is deliberately not here. Several components write
// to their own filesystem at runtime - helm to its cache, terraform and
// pulumi to their working directories - so it is not a flag that can be set
// uniformly, and it is not part of the restricted standard either.
func restrictedContainerSecurityContext() map[string]interface{} {
	return map[string]interface{}{
		"allowPrivilegeEscalation": false,
		"capabilities": map[string]interface{}{
			"drop": []interface{}{"ALL"},
		},
		"runAsNonRoot": true,
		"runAsUser":    nonRootUID,
		"runAsGroup":   nonRootGID,
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
