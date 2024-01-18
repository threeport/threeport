package observability

const grafanaValues = `
## Configure anonymous authentication
## https://grafana.com/docs/grafana/latest/setup-grafana/configure-security/configure-authentication/grafana/#anonymous-authentication
grafana.ini:
  auth.anonymous:
    enabled: true
    org_name: Main Org.
    org_role: Admin

persistence:
  enabled: true

datasources:
  loki-datasource.yaml:
    apiVersion: 1
    datasources:
    - name: loki
      access: proxy
      # basicAuth: true
      # basicAuthPassword: pass
      # basicAuthUser: daco
      editable: false
      isDefault: false
      jsonData:
          tlsSkipVerify: true
      type: loki
      url: http://loki:3100

adminPassword: password

rbac:
  ## If true, Grafana PSPs will be created
  ##
  pspEnabled: false

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
## Using default values from https://github.com/grafana/helm-charts/blob/main/charts/grafana/values.yaml
##
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