# Threeport Observability Controller

Manage observability systems for your deployed workloads.

This controller supports managing Prometheus for metrics collection, Promtail
for log forwarding, Loki for log storage and Grafana for an observability
dashboard.

This controller allows users to deploy an entire observability stack using these
components, or piecmeal.  It uses Helm charts for these community-support
projects under the hood.

