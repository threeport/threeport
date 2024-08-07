# Observability

Threeport offers observability systems for your applications as a support service
so that you can access metrics and logs for your workloads.

Threeport uses [Prometheus](https://prometheus.io/docs/introduction/overview/)
for metrics collection and alerting,
[Promtail](https://grafana.com/docs/loki/latest/send-data/promtail/) for log
forwarding, [Loki](https://github.com/grafana/loki) for log storage and
[Grafana](https://github.com/grafana/grafana) to access this info.

## Observability Stack Definition

This object defines the entire observability stack using the projects mentioned
above.  They can be configured to your liking, however, the default values can
be used to set up an observability stack without input values.

Reference: [ObservabilityStackDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#ObservabilityStackDefinition)

## Observability Stack Instance

When you create an instance, you can disable metrics or logging if you don't
need one of them and you can specify the Kubernetes Runtime Instance you would
like the observability stack deployed to.  Once deployed, all workload metrics
and logs will be collected and made available to the user.

Reference: [ObservabilityStackInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#ObservabilityStackInstance)

