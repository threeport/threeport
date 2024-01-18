package observability

const grafanaValues = `
persistence:
  enabled: true

adminPassword: password

sidecar:
  dashboards:
    enabled: true
    label: grafana_dashboard
    labelValue: "1"
    # Allow discovery in all namespaces for dashboards
    searchNamespace: ALL

  datasources:
    enabled: true
    label: grafana_datasource
    labelValue: "1"
    # Allow discovery in all namespaces for dashboards
    searchNamespace: ALL
`

const grafanaLokiDatasource = `
datasources:
  loki-datasource.yaml:
    apiVersion: 1
    datasources:
    - name: loki
      access: proxy
      editable: false
      isDefault: false
      jsonData:
          tlsSkipVerify: true
      type: loki
      url: http://loki:3100
`

const grafanaPrometheusServiceMonitor = `
serviceMonitor:
  # If true, a ServiceMonitor CRD is created for a prometheus operator
  # https://github.com/coreos/prometheus-operator
  #
  enabled: true

  # Scrape interval. If not set, the Prometheus default scrape interval is used.
  #
  interval: ""
`

const kubePrometheusStackValues = `
grafana:
  enabled: false

  ## ForceDeployDatasources Create datasource configmap even if grafana deployment has been disabled
  ##
  forceDeployDatasources: true

  ## ForceDeployDashboard Create dashboard configmap even if grafana deployment has been disabled
  ##
  forceDeployDashboards: true
`

const lokiValues = `
loki:
  auth_enabled: false
  commonConfig:
    replication_factor: 1
  storage:
    type: 'filesystem'
singleBinary:
  replicas: 1
`