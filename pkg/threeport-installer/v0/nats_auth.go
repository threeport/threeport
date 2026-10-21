package v0

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	auth "github.com/threeport/threeport/pkg/auth/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// The message broker requires TLS and a user and password on every client
// connection, on every tier.  Its CA, its certificate and the credentials are
// kept in secrets of the control plane namespace, created on the first
// install and reused by every install after it, so a reinstall keeps the
// clients and the server agreeing.
const (
	// natsCASecretName holds the CA that signs the broker's certificate:
	// its certificate and private key, which only the installer reads.
	natsCASecretName = "nats-ca"

	// natsServerCertSecretName holds the broker's certificate and key, and
	// the CA's certificate, which the server mounts.
	natsServerCertSecretName = "nats-server-cert"

	// NatsCACertSecretName holds only the CA's certificate, which every
	// client mounts to verify the broker.  Modules copy it into their own
	// namespace.
	NatsCACertSecretName = "nats-ca-cert"

	// natsCredentialsSecretName holds the user and password every client
	// authenticates with.
	natsCredentialsSecretName = "nats-credentials"

	// NatsUser is the broker user every Threeport component connects as.
	NatsUser = "threeport"

	// natsServerCertsPath is where the server mounts its certificate.
	natsServerCertsPath = "/etc/nats-certs"

	// NatsClientCAPath is where a client mounts the CA's certificate, and
	// NatsClientCAFile the certificate's path.
	NatsClientCAPath = "/etc/threeport/nats-ca"
	NatsClientCAFile = NatsClientCAPath + "/ca.crt"

	// natsClientCAVolumeName is the client volume of the CA's certificate.
	natsClientCAVolumeName = "nats-ca-cert"

	// natsPasswordBytes is how many random bytes a password is made from.
	natsPasswordBytes = 32
)

// natsCredentials are the credentials every client connects to the message
// broker with.
type natsCredentials struct {
	User     string
	Password string
}

// natsServerAltNames returns the names the broker's certificate is valid
// for: the Service a client connects to, in every form a client may name
// it, and each pod behind it, which the routes between the servers of a
// production cluster name.  Every certificate also carries localhost and
// 127.0.0.1, which a developer reaches the broker on through a port-forward.
func natsServerAltNames(namespace string) []string {
	service := natsServiceName
	return []string{
		service,
		fmt.Sprintf("%s.%s", service, namespace),
		fmt.Sprintf("%s.%s.svc", service, namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", service, namespace),
		fmt.Sprintf("*.%s", service),
		fmt.Sprintf("*.%s.%s", service, namespace),
		fmt.Sprintf("*.%s.%s.svc", service, namespace),
		fmt.Sprintf("*.%s.%s.svc.cluster.local", service, namespace),
	}
}

// secretGVR is the core v1 secrets resource.
var secretGVR = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

// ensureMessageBrokerSecrets creates the message broker's CA, certificate and
// credentials when they do not exist yet.  Each secret that exists is kept,
// so every install after the first reuses what the first created.
func (cpi *ControlPlaneInstaller) ensureMessageBrokerSecrets(kubeClient dynamic.Interface) error {
	if _, err := cpi.ensureNatsCredentials(kubeClient); err != nil {
		return err
	}

	ca, err := cpi.ensureNatsCA(kubeClient)
	if err != nil {
		return err
	}

	// the server's certificate
	serverCert, err := cpi.getSecret(kubeClient, natsServerCertSecretName)
	if err != nil {
		return err
	}
	if serverCert == nil {
		cert, key, err := auth.GenerateCertificate(
			ca.CAConfig, &ca.CAPrivateKey,
			natsServiceName, "threeport", "messagebroker",
			natsServerAltNames(cpi.Opts.Namespace)...,
		)
		if err != nil {
			return fmt.Errorf("failed to generate the message broker's certificate: %w", err)
		}
		if err := cpi.createSecret(kubeClient, natsServerCertSecretName, map[string]interface{}{
			"tls.crt": cert,
			"tls.key": key,
			"ca.crt":  ca.CAPemEncoded,
		}); err != nil {
			return err
		}
	}

	// the CA's certificate on its own, for clients
	caCert, err := cpi.getSecret(kubeClient, NatsCACertSecretName)
	if err != nil {
		return err
	}
	if caCert == nil {
		if err := cpi.createSecret(kubeClient, NatsCACertSecretName, map[string]interface{}{
			"ca.crt": ca.CAPemEncoded,
		}); err != nil {
			return err
		}
	}

	return nil
}

// ensureNatsCredentials returns the credentials clients connect with,
// generating them on the first install.
func (cpi *ControlPlaneInstaller) ensureNatsCredentials(kubeClient dynamic.Interface) (*natsCredentials, error) {
	existing, err := cpi.getSecret(kubeClient, natsCredentialsSecretName)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		password, err := generateNatsPassword()
		if err != nil {
			return nil, err
		}
		if err := cpi.createSecret(kubeClient, natsCredentialsSecretName, map[string]interface{}{
			"user":     NatsUser,
			"password": password,
		}); err != nil {
			return nil, err
		}
		// another install may have created it first, so the stored one is
		// the one used
		if existing, err = cpi.getSecret(kubeClient, natsCredentialsSecretName); err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, fmt.Errorf("secret %s is missing after it was created", natsCredentialsSecretName)
		}
	}

	return natsCredentialsFromSecret(existing)
}

// messageBrokerCredentials returns the credentials clients connect to the
// message broker with, which the install of the control plane's
// dependencies created.
func (cpi *ControlPlaneInstaller) messageBrokerCredentials(kubeClient dynamic.Interface) (*natsCredentials, error) {
	existing, err := cpi.getSecret(kubeClient, natsCredentialsSecretName)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf(
			"secret %s does not exist, so the control plane's dependencies are not installed",
			natsCredentialsSecretName,
		)
	}

	return natsCredentialsFromSecret(existing)
}

// natsCredentialsFromSecret reads the credentials from their secret.
func natsCredentialsFromSecret(secret *unstructured.Unstructured) (*natsCredentials, error) {
	user, err := secretValue(secret, "user")
	if err != nil {
		return nil, err
	}
	password, err := secretValue(secret, "password")
	if err != nil {
		return nil, err
	}
	if user == "" || password == "" {
		return nil, fmt.Errorf("secret %s holds no user or password", secret.GetName())
	}

	return &natsCredentials{User: user, Password: password}, nil
}

// ensureNatsCA returns the CA that signs the broker's certificate,
// generating it on the first install.
func (cpi *ControlPlaneInstaller) ensureNatsCA(kubeClient dynamic.Interface) (*auth.AuthConfig, error) {
	existing, err := cpi.getSecret(kubeClient, natsCASecretName)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		ca, err := auth.GetAuthConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to generate the message broker's CA: %w", err)
		}
		if err := cpi.createSecret(kubeClient, natsCASecretName, map[string]interface{}{
			"tls.crt": ca.CAPemEncoded,
			"tls.key": ca.CAPrivateKeyPemEncoded,
		}); err != nil {
			return nil, err
		}
		if existing, err = cpi.getSecret(kubeClient, natsCASecretName); err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, fmt.Errorf("secret %s is missing after it was created", natsCASecretName)
		}
	}

	caPem, err := secretValue(existing, "tls.crt")
	if err != nil {
		return nil, err
	}
	keyPem, err := secretValue(existing, "tls.key")
	if err != nil {
		return nil, err
	}

	return authConfigFromPEM([]byte(caPem), []byte(keyPem))
}

// generateNatsPassword returns a random password.  It starts with letters,
// since the server reads it from its environment into its config, where an
// unquoted value that starts with a digit may be read as a number.
func generateNatsPassword() (string, error) {
	random := make([]byte, natsPasswordBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("failed to generate the message broker password: %w", err)
	}

	return "tp" + base64.RawURLEncoding.EncodeToString(random), nil
}

// getSecret returns a secret of the control plane namespace, or nil when it
// does not exist.
func (cpi *ControlPlaneInstaller) getSecret(kubeClient dynamic.Interface, name string) (*unstructured.Unstructured, error) {
	secret, err := kubeClient.Resource(secretGVR).Namespace(cpi.Opts.Namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if k8serrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read secret %s: %w", name, err)
	}

	return secret, nil
}

// createSecret creates a persistent secret of the control plane namespace.
// One that another install created meanwhile is left as it is.
func (cpi *ControlPlaneInstaller) createSecret(
	kubeClient dynamic.Interface,
	name string,
	data map[string]interface{},
) error {
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       "Opaque",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": cpi.Opts.Namespace,
			},
			"stringData": data,
		},
	}
	setPersistent(secret)
	setManagedByLabel(secret)
	if _, err := kubeClient.Resource(secretGVR).Namespace(cpi.Opts.Namespace).Create(
		context.Background(), secret, metav1.CreateOptions{},
	); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create secret %s: %w", name, err)
	}

	return nil
}

// secretValue returns one value of a secret, from its data as the API server
// returns it, or from its string data as it was written.
func secretValue(secret *unstructured.Unstructured, key string) (string, error) {
	if encoded, found, _ := unstructured.NestedString(secret.Object, "data", key); found {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", fmt.Errorf("failed to decode %s of secret %s: %w", key, secret.GetName(), err)
		}
		return string(decoded), nil
	}
	if value, found, _ := unstructured.NestedString(secret.Object, "stringData", key); found {
		return value, nil
	}

	return "", fmt.Errorf("secret %s holds no %s", secret.GetName(), key)
}

// authConfigFromPEM rebuilds a CA from its PEM-encoded certificate and
// PKCS1 private key.
func authConfigFromPEM(caPem, keyPem []byte) (*auth.AuthConfig, error) {
	caBlock, _ := pem.Decode(caPem)
	if caBlock == nil {
		return nil, errors.New("ca cert pem block not found")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ca cert: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPem)
	if keyBlock == nil {
		return nil, errors.New("ca key pem block not found")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ca key: %w", err)
	}

	return &auth.AuthConfig{
		CAConfig:                  caCert,
		CAPrivateKey:              *caKey,
		CA:                        caBlock.Bytes,
		CAPemEncoded:              string(caPem),
		CABase64Encoded:           util.Base64Encode(string(caPem)),
		CAPrivateKeyPemEncoded:    string(keyPem),
		CAPrivateKeyBase64Encoded: util.Base64Encode(string(keyPem)),
	}, nil
}

// NatsClientCAVolume returns the volume of the CA's certificate a client
// mounts.
func NatsClientCAVolume() map[string]interface{} {
	return map[string]interface{}{
		"name": natsClientCAVolumeName,
		"secret": map[string]interface{}{
			"secretName": NatsCACertSecretName,
		},
	}
}

// NatsClientCAVolumeMount returns the mount of the CA's certificate at
// NatsClientCAPath.
func NatsClientCAVolumeMount() map[string]interface{} {
	return map[string]interface{}{
		"name":      natsClientCAVolumeName,
		"mountPath": NatsClientCAPath,
		"readOnly":  true,
	}
}

// natsCLIEnv returns the environment the nats CLI connects with: over TLS,
// verifying the broker against the CA's certificate mounted at
// NatsClientCAPath, as the threeport user.
func natsCLIEnv() []interface{} {
	return []interface{}{
		map[string]interface{}{
			"name":  "NATS_URL",
			"value": fmt.Sprintf("tls://%s:4222", natsServiceName),
		},
		map[string]interface{}{
			"name":  "NATS_CA",
			"value": NatsClientCAFile,
		},
		natsSecretEnv("NATS_USER", "user"),
		natsSecretEnv("NATS_PASSWORD", "password"),
	}
}

// natsSecretEnv returns an environment variable set from the credentials
// secret.
func natsSecretEnv(name, key string) map[string]interface{} {
	return map[string]interface{}{
		"name": name,
		"valueFrom": map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				"name": natsCredentialsSecretName,
				"key":  key,
			},
		},
	}
}

// natsServerCertVolumeName is the server's volume of its certificate.
const natsServerCertVolumeName = "nats-server-cert"

// natsServerCertVolume returns the server's volume of its certificate.
func natsServerCertVolume() map[string]interface{} {
	return map[string]interface{}{
		"name": natsServerCertVolumeName,
		"secret": map[string]interface{}{
			"secretName": natsServerCertSecretName,
		},
	}
}

// natsServerCertVolumeMount returns the server's mount of its certificate at
// natsServerCertsPath.
func natsServerCertVolumeMount() map[string]interface{} {
	return map[string]interface{}{
		"name":      natsServerCertVolumeName,
		"mountPath": natsServerCertsPath,
		"readOnly":  true,
	}
}

// natsServerTLSAuthConfig is the part of the server's config that requires
// TLS and the threeport user's password on every client connection.  The
// server reads the user and password from its environment, which the
// credentials secret sets, so they never sit in the config map.  The
// monitoring port stays plain HTTP inside the pod, for the probes and the
// metrics exporter.
const natsServerTLSAuthConfig = `
###############
#             #
# TLS         #
#             #
###############
tls {
  cert_file: "` + natsServerCertsPath + `/tls.crt"
  key_file: "` + natsServerCertsPath + `/tls.key"
  timeout: 2
}

###################
#                 #
# Authorization   #
#                 #
###################
authorization {
  user: $NATS_USER
  password: $NATS_PASSWORD
}
`

// statefulSetGVR is the apps v1 stateful sets resource.
var statefulSetGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}

// createOrUpdateStatefulSet creates a stateful set, or replaces the pod
// template of one that exists, which rolls its pods onto the new template.
// The rest of its spec is left as it is: the volume claim templates, the
// selector and the service name cannot change once it exists, and the
// replicas may have been scaled.
func (cpi *ControlPlaneInstaller) createOrUpdateStatefulSet(
	statefulSet *unstructured.Unstructured,
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	statefulSets := kubeClient.Resource(statefulSetGVR).Namespace(statefulSet.GetNamespace())
	existing, err := statefulSets.Get(context.Background(), statefulSet.GetName(), metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return cpi.CreateOrUpdateKubeResource(statefulSet, kubeClient, mapper)
	}
	if err != nil {
		return fmt.Errorf("failed to read stateful set %s: %w", statefulSet.GetName(), err)
	}

	template, found, err := unstructured.NestedFieldNoCopy(statefulSet.Object, "spec", "template")
	if err != nil || !found {
		return fmt.Errorf("stateful set %s has no pod template", statefulSet.GetName())
	}
	spec, found, err := unstructured.NestedFieldNoCopy(existing.Object, "spec")
	existingSpec, ok := spec.(map[string]interface{})
	if err != nil || !found || !ok {
		return fmt.Errorf("existing stateful set %s has no spec", statefulSet.GetName())
	}
	existingSpec["template"] = template

	if _, err := statefulSets.Update(context.Background(), existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update the pod template of stateful set %s: %w", statefulSet.GetName(), err)
	}

	return nil
}

// natsDropJobEnv returns the environment the stream drop job connects with.
// The drop runs against the broker before the install that follows, which
// on a control plane installed before the broker required TLS is still
// plain and has no credentials secret, so the secret and the CA's
// certificate are optional there and the job connects without them.
func natsDropJobEnv() []interface{} {
	env := natsCLIEnv()
	for _, e := range env {
		variable := e.(map[string]interface{})
		if valueFrom, ok := variable["valueFrom"].(map[string]interface{}); ok {
			valueFrom["secretKeyRef"].(map[string]interface{})["optional"] = true
		}
	}

	return env
}

// natsDropJobCAVolume returns the drop job's volume of the CA's certificate,
// which is optional for the same reason as its credentials.
func natsDropJobCAVolume() map[string]interface{} {
	volume := NatsClientCAVolume()
	volume["secret"].(map[string]interface{})["optional"] = true

	return volume
}

// natsDropJobConnect is the drop job's shell prelude, which falls back to a
// plain connection without credentials when the broker does not require TLS
// yet: either the credentials secret does not exist, or a TLS connection
// fails because the broker has not rolled onto its new config.
const natsDropJobConnect = `if [ -z "$NATS_PASSWORD" ] || ! nats account info > /dev/null 2>&1; then
  echo "connecting to the message broker without TLS or credentials"
  export NATS_URL=nats://` + natsServiceName + `:4222
  unset NATS_CA NATS_USER NATS_PASSWORD
fi
`
