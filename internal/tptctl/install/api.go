package install

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/threeport/threeport/internal/tptctl/output"
)

const (
	ThreeportControlPlaneNs  = "threeport-control-plane"
	ThreeportAPIInternalPort = "1323"
	APIDepsManifestPath      = "/tmp/threeport-api-deps-manifest.yaml"
	APIServerManifestPath    = "/tmp/threeport-api-server-manifest.yaml"
	APIIngressManifestPath   = "/tmp/threeport-api-ingress-manifest.yaml"
	APIIngressResourceName   = "threeport-api-ingress"
	PostgresImage            = "postgres:15-alpine"
	NATSBoxImage             = "natsio/nats-box:0.13.3"
	NATSServerImage          = "nats:2.9-alpine"
	NATSConfigReloaderImage  = "natsio/nats-server-config-reloader:0.7.4"
	NATSPromExporterImage    = "natsio/prometheus-nats-exporter:0.10.1"
	ThreeportRESTAPIImage    = "ghcr.io/threeport/threeport-rest-api:v1.1.7"
)

// InstallAPI installs the threeport API into a target Kubernetes cluster
func InstallAPI(kubeconfig, threeportAPISubdomain, rootDomain, loadBalancerURL string) error {
	// write API dependencies manifest to /tmp directory
	apiDepsManifest, err := os.Create(APIDepsManifestPath)
	if err != nil {
		return fmt.Errorf("failed to write API dependency manifests to disk: %w", err)
	}
	defer apiDepsManifest.Close()
	apiDepsManifest.WriteString(APIDepsManifest())
	output.Info("Threeport API dependencies manifest written to /tmp directory")

	// install API dependencies
	output.Info("Installing Threeport API dependencies")
	apiDepsCreate := exec.Command(
		"kubectl",
		"--kubeconfig",
		kubeconfig,
		"apply",
		"-f",
		APIDepsManifestPath,
	)
	apiDepsCreateOut, err := apiDepsCreate.CombinedOutput()
	if err != nil {
		output.Error(fmt.Sprintf("kubectl error: %s", apiDepsCreateOut), nil)
		return fmt.Errorf("failed to install API dependencies: %w", err)
	}

	psqlConfigCreate := exec.Command(
		"kubectl",
		"--kubeconfig",
		kubeconfig,
		"create",
		"configmap",
		"postgres-config-data",
		"-n",
		ThreeportControlPlaneNs,
	)
	psqlConfigCreateOut, err := psqlConfigCreate.CombinedOutput()
	if err != nil {
		fmt.Println(psqlConfigCreateOut)
		//output.Error(fmt.Sprintf("kubectl error: %s", psqlConfigCreateOut), nil)
		//return fmt.Errorf("failed to create API database config: %w", err)
	}
	output.Info("Threeport API dependencies created")

	// write Threeport API server manifest to /tmp directory
	apiServerManifest, err := os.Create(APIServerManifestPath)
	if err != nil {
		return fmt.Errorf("failed to write API manifest to disk: %w", err)
	}
	defer apiServerManifest.Close()
	apiServerManifest.WriteString(APIServerManifest())
	output.Info("Threeport API server manifest written to /tmp directory")

	// install Threeport API
	output.Info("installing Threeport API server")
	apiServerCreate := exec.Command(
		"kubectl",
		"--kubeconfig",
		kubeconfig,
		"apply",
		"-f",
		APIServerManifestPath,
	)
	apiServerCreateOut, err := apiServerCreate.CombinedOutput()
	if err != nil {
		output.Error(fmt.Sprintf("kubectl error: %s", apiServerCreateOut), nil)
		return fmt.Errorf("failed to create API server: %w", err)
	}

	// write Threeport API ingress manifest to /tmp directory
	apiIngressManifest, err := os.Create(APIIngressManifestPath)
	if err != nil {
		return fmt.Errorf("failed to write API ingress manifest to disk: %w", err)
	}
	defer apiIngressManifest.Close()
	if rootDomain != "" {
		apiIngressManifest.WriteString(APIIngressWithTLSManifest(threeportAPISubdomain, rootDomain))
	} else {
		apiIngressManifest.WriteString(APIIngressManifest(loadBalancerURL))
	}
	output.Info("Threeport API ingress manifest written to /tmp directory")

	// install Threeport API ingress resource
	output.Info("installing Threeport API ingress")
	apiIngressCreate := exec.Command(
		"kubectl",
		"--kubeconfig",
		kubeconfig,
		"apply",
		"-f",
		APIIngressManifestPath,
	)
	apiIngressCreateOut, err := apiIngressCreate.CombinedOutput()
	if err != nil {
		output.Error(fmt.Sprintf("kubectl error: %s", apiIngressCreateOut), nil)
		return fmt.Errorf("failed to create API ingress: %w", err)
	}

	output.Info("Threeport API server created")

	return nil
}

// UninstallAPIIngress deletes the ingress resource for the Threeport API.  This
// must be done before deleting infra so the DNS records tied to the ingress are
// removed.
func UninstallAPIIngress(kubeconfig string) error {
	apiIngressDelete := exec.Command(
		"kubectl",
		"--kubeconfig",
		kubeconfig,
		"delete",
		"ingress",
		"-n",
		ThreeportControlPlaneNs,
		APIIngressResourceName,
	)
	apiIngressDeleteOut, err := apiIngressDelete.CombinedOutput()
	if err != nil {
		output.Error(fmt.Sprintf("kubectl error: %s", apiIngressDeleteOut), nil)
		return fmt.Errorf("failed to delete Threeport API ingress resource: %w", err)
	}

	output.Info("Threeport API ingress resource removed")
	output.Info("waiting for DNS recoreds to be deleted...")

	time.Sleep(time.Second * 80)

	return nil
}

// GetThreeportAPIEndpoint returns the threeport API endpoint
func GetThreeportAPIEndpoint() string {
	var apiProtocol string
	var apiHostname string
	var apiPort string

	//switch infraProvider {
	//case "kind":
	//	apiProtocol = provider.KindThreeportAPIProtocol
	//	apiHostname = provider.KindThreeportAPIHostname
	//	apiPort = provider.KindThreeportAPIPort
	//case "eks":
	//	apiProtocol = "?"
	//	apiHostname = "?"
	//	apiPort = "?"
	//}

	return fmt.Sprintf(
		"%s://%s:%s",
		apiProtocol, apiHostname, apiPort,
	)
}

// APIDepsManifest returns a yaml manifest for the threeport API dependencies
// with the namespace included.
func APIDepsManifest() string {
	return fmt.Sprintf(`---
apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: threeport-api-db-config
  namespace: %[1]s
  labels:
    app: threeport-api-db
data:
  POSTGRES_DB: threeport_api
  POSTGRES_USER: tp_rest_api
  POSTGRES_PASSWORD: tp-rest-api-pwd
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: threeport-api-db
  namespace: %[1]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: threeport-api-db
  template:
    metadata:
      labels:
        app: threeport-api-db
    spec:
      containers:
        - name: postgres
          image: %[2]s
          imagePullPolicy: "IfNotPresent"
          ports:
            - containerPort: 5432
          envFrom:
            - configMapRef:
                name: threeport-api-db-config
          volumeMounts:
            - mountPath: /var/lib/postgresql/data
              name: postgredb
            - mountPath: /docker-entrypoint-initdb.d
              name: postgresql-initdb
      volumes:
        - name: postgredb
          emptyDir: {}
        - name: postgresql-initdb
          configMap:
            name: postgres-config-data
---
apiVersion: v1
kind: Service
metadata:
  name: threeport-api-db
  namespace: %[1]s
  labels:
    app: threeport-api-db
spec:
  ports:
   - port: 5432
  selector:
   app: threeport-api-db
---
# Source: nats/templates/pdb.yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: threeport-message-broker
  namespace: %[1]s
  labels:
    helm.sh/chart: nats-0.18.2
    app.kubernetes.io/name: nats
    app.kubernetes.io/instance: threeport-message-broker
    app.kubernetes.io/version: "2.9.3"
    app.kubernetes.io/managed-by: Helm
spec:
  maxUnavailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: nats
      app.kubernetes.io/instance: threeport-message-broker
---
# Source: nats/templates/rbac.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: threeport-message-broker
  namespace: %[1]s
  labels:
    helm.sh/chart: nats-0.18.2
    app.kubernetes.io/name: nats
    app.kubernetes.io/instance: threeport-message-broker
    app.kubernetes.io/version: "2.9.3"
    app.kubernetes.io/managed-by: Helm
---
# Source: nats/templates/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: threeport-message-broker-config
  namespace: %[1]s
  labels:
    helm.sh/chart: nats-0.18.2
    app.kubernetes.io/name: nats
    app.kubernetes.io/instance: threeport-message-broker
    app.kubernetes.io/version: "2.9.3"
    app.kubernetes.io/managed-by: Helm
data:
  nats.conf: |
    # NATS Clients Port
    port: 4222

    # PID file shared with configuration reloader.
    pid_file: "/var/run/nats/nats.pid"

    ###############
    #             #
    # Monitoring  #
    #             #
    ###############
    http: 8222
    server_name:$POD_NAME
    ###################################
    #                                 #
    # NATS JetStream                  #
    #                                 #
    ###################################
    jetstream {
      max_mem: 30Mi
    }
    lame_duck_grace_period: 10s
    lame_duck_duration: 30s
---
# Source: nats/templates/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: threeport-message-broker
  namespace: %[1]s
  labels:
    helm.sh/chart: nats-0.18.2
    app.kubernetes.io/name: nats
    app.kubernetes.io/instance: threeport-message-broker
    app.kubernetes.io/version: "2.9.3"
    app.kubernetes.io/managed-by: Helm
spec:
  selector:
    app.kubernetes.io/name: nats
    app.kubernetes.io/instance: threeport-message-broker
  clusterIP: None
  publishNotReadyAddresses: true
  ports:
  - name: client
    port: 4222
    appProtocol: tcp
  - name: cluster
    port: 6222
    appProtocol: tcp
  - name: monitor
    port: 8222
    appProtocol: http
  - name: metrics
    port: 7777
    appProtocol: http
  - name: leafnodes
    port: 7422
    appProtocol: tcp
  - name: gateways
    port: 7522
    appProtocol: tcp
---
# Source: nats/templates/nats-box.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: threeport-message-broker-box
  namespace: %[1]s
  labels:
    app: threeport-message-broker-box
    chart: nats-0.18.2
spec:
  replicas: 1
  selector:
    matchLabels:
      app: threeport-message-broker-box
  template:
    metadata:
      labels:
        app: threeport-message-broker-box
    spec:
      volumes:
      containers:
      - name: nats-box
        image: %[3]s
        imagePullPolicy: IfNotPresent
        resources:
          null
        env:
        - name: NATS_URL
          value: threeport-message-broker
        command:
        - "tail"
        - "-f"
        - "/dev/null"
        volumeMounts:
---
# Source: nats/templates/statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: threeport-message-broker
  namespace: %[1]s
  labels:
    helm.sh/chart: nats-0.18.2
    app.kubernetes.io/name: nats
    app.kubernetes.io/instance: threeport-message-broker
    app.kubernetes.io/version: "2.9.3"
    app.kubernetes.io/managed-by: Helm
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: nats
      app.kubernetes.io/instance: threeport-message-broker
  replicas: 1
  serviceName: threeport-message-broker

  podManagementPolicy: Parallel

  template:
    metadata:
      annotations:
        prometheus.io/path: /metrics
        prometheus.io/port: "7777"
        prometheus.io/scrape: "true"
        checksum/config: 3b398e973c292bf8c2eb90d62acb846274c0489643aad560d8c4aed123f20ce7
      labels:
        app.kubernetes.io/name: nats
        app.kubernetes.io/instance: threeport-message-broker
    spec:
      # Common volumes for the containers.
      volumes:
      - name: config-volume
        configMap:
          name: threeport-message-broker-config

      # Local volume shared with the reloader.
      - name: pid
        emptyDir: {}

      #################
      #               #
      #  TLS Volumes  #
      #               #
      #################

      serviceAccountName: threeport-message-broker

      # Required to be able to HUP signal and apply config
      # reload to the server without restarting the pod.
      shareProcessNamespace: true

      #################
      #               #
      #  NATS Server  #
      #               #
      #################
      terminationGracePeriodSeconds: 60
      containers:
      - name: nats
        image: %[4]s
        imagePullPolicy: IfNotPresent
        resources:
          {}
        ports:
        - containerPort: 4222
          name: client
        - containerPort: 6222
          name: cluster
        - containerPort: 8222
          name: monitor

        command:
        - "nats-server"
        - "--config"
        - "/etc/nats-config/nats.conf"

        # Required to be able to define an environment variable
        # that refers to other environment variables.  This env var
        # is later used as part of the configuration file.
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: SERVER_NAME
          value: $(POD_NAME)
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: CLUSTER_ADVERTISE
          value: $(POD_NAME).threeport-message-broker.$(POD_NAMESPACE).svc.cluster.local
        volumeMounts:
        - name: config-volume
          mountPath: /etc/nats-config
        - name: pid
          mountPath: /var/run/nats

        #######################
        #                     #
        # Healthcheck Probes  #
        #                     #
        #######################
        livenessProbe:
          failureThreshold: 3
          httpGet:
            path: /
            port: 8222
          initialDelaySeconds: 10
          periodSeconds: 30
          successThreshold: 1
          timeoutSeconds: 5
        readinessProbe:
          failureThreshold: 3
          httpGet:
            path: /
            port: 8222
          initialDelaySeconds: 10
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 5
        startupProbe:
          # for NATS server versions >=2.7.1, /healthz will be enabled
          # startup probe checks that the JS server is enabled, is current with the meta leader,
          # and that all streams and consumers assigned to this JS server are current
          failureThreshold: 30
          httpGet:
            path: /healthz
            port: 8222
          initialDelaySeconds: 10
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 5

        # Gracefully stop NATS Server on pod deletion or image upgrade.
        #
        lifecycle:
          preStop:
            exec:
              # send the lame duck shutdown signal to trigger a graceful shutdown
              # nats-server will ignore the TERM signal it receives after this
              #
              command:
              - "nats-server"
              - "-sl=ldm=/var/run/nats/nats.pid"

      #################################
      #                               #
      #  NATS Configuration Reloader  #
      #                               #
      #################################
      - name: reloader
        image: %[5]s
        imagePullPolicy: IfNotPresent
        resources:
          null
        command:
        - "nats-server-config-reloader"
        - "-pid"
        - "/var/run/nats/nats.pid"
        - "-config"
        - "/etc/nats-config/nats.conf"
        volumeMounts:
        - name: config-volume
          mountPath: /etc/nats-config
        - name: pid
          mountPath: /var/run/nats

      ##############################
      #                            #
      #  NATS Prometheus Exporter  #
      #                            #
      ##############################
      - name: metrics
        image: %[6]s
        imagePullPolicy: IfNotPresent
        resources:
          {}
        args:
        - -connz
        - -routez
        - -subz
        - -varz
        - -prefix=nats
        - -use_internal_server_id
        - -jsz=all
        - http://localhost:8222/
        ports:
        - containerPort: 7777
          name: metrics

  volumeClaimTemplates:
---
# Source: nats/templates/tests/test-request-reply.yaml
apiVersion: v1
kind: Pod
metadata:
  name: "threeport-message-broker-test-request-reply"
  namespace: %[1]s
  labels:
    chart: nats-0.18.2
    app: threeport-message-broker-test-request-reply
  annotations:
    "helm.sh/hook": test
spec:
  containers:
  - name: nats-box
    image: synadia/nats-box
    env:
    - name: NATS_HOST
      value: threeport-message-broker
    command:
    - /bin/sh
    - -ec
    - |
      nats reply -s nats://$NATS_HOST:4222 'name.>' --command "echo 1" &
    - |
      "&&"
    - |
      name=$(nats request -s nats://$NATS_HOST:4222 name.test '' 2>/dev/null)
    - |
      "&&"
    - |
      [ $name = test ]

  restartPolicy: Never
`, ThreeportControlPlaneNs, PostgresImage, NATSBoxImage, NATSServerImage,
		NATSConfigReloaderImage, NATSPromExporterImage)
}

// APIServerManifest returns a yaml manifest for the threeport API with the
// namespace included.
func APIServerManifest() string {
	return fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: db-config
  namespace: %[1]s
stringData:
  env: |
    DB_HOST=threeport-api-db
    DB_USER=tp_rest_api
    DB_PASSWORD=tp-rest-api-pwd
    DB_NAME=threeport_api
    DB_PORT=5432
    DB_SSL_MODE=disable
    NATS_HOST=threeport-message-broker
    NATS_PORT=4222
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: threeport-api-server
  namespace: %[1]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: threeport-api-server
  template:
    metadata:
      labels:
        app.kubernetes.io/name: threeport-api-server
    spec:
      containers:
      - name: api-server
        image: %[3]s
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: %[2]s
          hostPort: %[2]s
          name: http
          protocol: TCP
        volumeMounts:
        - name: db-config
          mountPath: "/etc/threeport/"
      volumes:
      - name: db-config
        secret:
          secretName: db-config
---
apiVersion: v1
kind: Service
metadata:
  name: threeport-api-server
  namespace: %[1]s
spec:
  selector:
    app.kubernetes.io/name: threeport-api-server
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: %[2]s
`, ThreeportControlPlaneNs, ThreeportAPIInternalPort, ThreeportRESTAPIImage)
}

func APIIngressWithTLSManifest(threeportAPISubdomain, rootDomain string) string {
	return fmt.Sprintf(`---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %[1]s
  namespace: threeport-control-plane
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-staging
spec:
  ingressClassName: kong
  rules:
  - host: %[2]s.%[3]s
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: threeport-api-server
            port:
              number: 80
  tls:
  - hosts:
    - %[2]s.%[3]s
    secretName: threeport-api-ingress-cert
`, APIIngressResourceName, threeportAPISubdomain, rootDomain)
}

func APIIngressManifest(loadBalancerURL string) string {
	return fmt.Sprintf(`---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %[1]s
  namespace: threeport-control-plane
spec:
  ingressClassName: kong
  rules:
  - host: %[2]s
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: threeport-api-server
            port:
              number: 80
`, APIIngressResourceName, loadBalancerURL)
}
