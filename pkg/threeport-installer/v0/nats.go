package v0

// The resources of the message broker's containers.  The NATS server measured
// 6Mi at rest, 15Mi holding 13 streams and a lock bucket with 39,000
// messages, and 199Mi at its peak with a consumer reading each of those
// streams at once, far more traffic than a control plane makes.  The config
// reloader measured 4Mi and the metrics exporter 16Mi.
const (
	natsServerRequestCPU    = "50m"
	natsServerRequestMemory = "64Mi"
	natsServerLimitCPU      = "1"
	natsServerLimitMemory   = "512Mi"

	// natsServerGoMemLimit is 90% of the server's memory limit, the share the
	// NATS Helm chart gives the Go runtime so it collects garbage before the
	// container is killed for running out of memory.
	natsServerGoMemLimit = "460MiB"

	natsReloaderRequestCPU    = "5m"
	natsReloaderRequestMemory = "16Mi"
	natsReloaderLimitCPU      = "100m"
	natsReloaderLimitMemory   = "64Mi"

	natsExporterRequestCPU    = "5m"
	natsExporterRequestMemory = "32Mi"
	natsExporterLimitCPU      = "100m"
	natsExporterLimitMemory   = "128Mi"

	// nats-box idles until an operator runs the nats CLI in it
	natsBoxRequestCPU    = "5m"
	natsBoxRequestMemory = "16Mi"
	natsBoxLimitCPU      = "200m"
	natsBoxLimitMemory   = "128Mi"

	// natsLivenessPath is the health check the liveness probe uses: whether
	// JetStream is enabled, without the full check's wait for every stream
	// to be current, which fails while a restarted server catches up.
	natsLivenessPath = "/healthz?js-enabled-only=true"
)

// natsServerResources returns the NATS server container's resources.
func natsServerResources() map[string]interface{} {
	return containerResources(natsServerRequestCPU, natsServerRequestMemory, natsServerLimitCPU, natsServerLimitMemory)
}

// natsReloaderResources returns the config reloader container's resources.
func natsReloaderResources() map[string]interface{} {
	return containerResources(natsReloaderRequestCPU, natsReloaderRequestMemory, natsReloaderLimitCPU, natsReloaderLimitMemory)
}

// natsExporterResources returns the metrics exporter container's resources.
func natsExporterResources() map[string]interface{} {
	return containerResources(natsExporterRequestCPU, natsExporterRequestMemory, natsExporterLimitCPU, natsExporterLimitMemory)
}

// natsBoxResources returns the nats-box container's resources.
func natsBoxResources() map[string]interface{} {
	return containerResources(natsBoxRequestCPU, natsBoxRequestMemory, natsBoxLimitCPU, natsBoxLimitMemory)
}

// containerResources returns a container's resource requests and limits.
func containerResources(requestCPU, requestMemory, limitCPU, limitMemory string) map[string]interface{} {
	return map[string]interface{}{
		"requests": map[string]interface{}{
			"cpu":    requestCPU,
			"memory": requestMemory,
		},
		"limits": map[string]interface{}{
			"cpu":    limitCPU,
			"memory": limitMemory,
		},
	}
}
