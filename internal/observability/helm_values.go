package observability

const grafanaValues = `
## Configure anonymous authentication
## https://grafana.com/docs/grafana/latest/setup-grafana/configure-security/configure-authentication/grafana/#anonymous-authentication
grafana.ini:
  auth.anonymous:
    enabled: true
    org_name: Main Org.
    org_role: Viewer

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

###### BEGIN KUBE-PROMETHEUS-STACK VALUES ######
## This chart uses the default values from the kube-prometheus-stack chart.
## https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-55.8.1/charts/kube-prometheus-stack/values.yaml#L927

adminPassword: prom-operator

rbac:
  ## If true, Grafana PSPs will be created
  ##
  pspEnabled: false

ingress:
  ## If true, Grafana Ingress will be created
  ##
  enabled: false

  ## IngressClassName for Grafana Ingress.
  ## Should be provided if Ingress is enable.
  ##
  # ingressClassName: nginx

  ## Annotations for Grafana Ingress
  ##
  annotations: {}
    # kubernetes.io/ingress.class: nginx
    # kubernetes.io/tls-acme: "true"

  ## Labels to be added to the Ingress
  ##
  labels: {}

  ## Hostnames.
  ## Must be provided if Ingress is enable.
  ##
  # hosts:
  #   - grafana.domain.com
  hosts: []

  ## Path for grafana ingress
  path: /

  ## TLS configuration for grafana Ingress
  ## Secret must be manually created in the namespace
  ##
  tls: []
  # - secretName: grafana-general-tls
  #   hosts:
  #   - grafana.example.com

sidecar:
  dashboards:
    enabled: true
    label: grafana_dashboard
    labelValue: "1"
    # Allow discovery in all namespaces for dashboards
    searchNamespace: ALL

    ## Annotations for Grafana dashboard configmaps
    ##
    annotations: {}
    multicluster:
      global:
        enabled: false
      etcd:
        enabled: false
    provider:
      allowUiUpdates: false
  datasources:
    enabled: true
    defaultDatasourceEnabled: true
    isDefaultDatasource: true

    uid: prometheus

    ## Set method for HTTP to send query to datasource
    httpMethod: POST

    ## Create datasource for each Pod of Prometheus StatefulSet;
    ## this uses headless service 'prometheus-operated' which is
    ## created by Prometheus Operator
    ## ref: https://github.com/prometheus-operator/prometheus-operator/blob/0fee93e12dc7c2ea1218f19ae25ec6b893460590/pkg/prometheus/statefulset.go#L255-L286
    createPrometheusReplicasDatasources: false
    label: grafana_datasource
    labelValue: "1"

    ## Field with internal link pointing to existing data source in Grafana.
    ## Can be provisioned via additionalDataSources
    exemplarTraceIdDestinations: {}
      # datasourceUid: Jaeger
      # traceIdLabelName: trace_id
    alertmanager:
      enabled: true
      uid: alertmanager
      handleGrafanaManagedAlerts: false
      implementation: prometheus

## Passed to grafana subchart and used by servicemonitor below
##
service:
  portName: http-web

serviceMonitor:
  # If true, a ServiceMonitor CRD is created for a prometheus operator
  # https://github.com/coreos/prometheus-operator
  #
  enabled: true

  # Path to use for scraping metrics. Might be different if server.root_url is set
  # in grafana.ini
  path: "/metrics"

  #  namespace: monitoring  (defaults to use the namespace this chart is deployed to)

  # labels for the ServiceMonitor
  labels: {}

  # Scrape interval. If not set, the Prometheus default scrape interval is used.
  #
  interval: ""
  scheme: http
  tlsConfig: {}
  scrapeTimeout: 30s

###### END KUBE-PROMETHEUS-STACK VALUES ######
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