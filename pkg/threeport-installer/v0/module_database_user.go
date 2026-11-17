package v0

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/pkg/api-server/v0/database"
)

// A module connects to the control plane's database as a user of its own,
// which the control plane creates and the module's namespace gets only the
// password of.  The database's root credentials never leave the control
// plane's namespace: the statements that create, rotate and drop a module's
// user run there, in a Job that mounts them.
//
// A module's user is a member of the threeport user, so it can do what the
// modules could do as that user before: run its migrations against the
// control plane's database and serve its queries.  It is also a member of
// moduleDatabaseAuditRole, whose statements CockroachDB writes to its audit
// log with the module's user, so a statement can be traced to its module.
//
// A module authenticates with a password over TLS that verifies the
// database's certificate, rather than with a client certificate: the
// database's CA key isn't kept after the install, so a control plane can't
// sign one for a module.

const (
	// ModuleDatabaseCredsSecretName is the secret in a module's namespace with
	// what the module connects to the database with: the database's CA
	// certificate, and the module's user and password.
	ModuleDatabaseCredsSecretName = "db-module-creds"

	// ModuleDatabaseCredsAnnotation is set on a module's pod templates to a
	// digest of its credentials, so pods are replaced when they're rotated.
	ModuleDatabaseCredsAnnotation = "threeport.io/database-credentials"

	// moduleDatabaseUserPrefix starts every module's database user.
	moduleDatabaseUserPrefix = "threeport_"

	// moduleDatabaseUserMaxLength is the longest name CockroachDB accepts
	// for a user.
	moduleDatabaseUserMaxLength = 63

	// moduleDatabaseAuditRole is the role every module's user is a member
	// of, whose statements CockroachDB audits.
	moduleDatabaseAuditRole = "threeport_modules"

	// moduleDatabasePasswordKey is the key of the password in the control
	// plane's secret for a module's user.
	moduleDatabasePasswordKey = "password"

	// moduleDatabaseScriptKey is the key of the script a module user Job
	// runs, in the secret of the same name as the Job.
	moduleDatabaseScriptKey = "script.sh"

	// moduleDatabaseScriptPath is where a module user Job mounts its script.
	moduleDatabaseScriptPath = "/threeport/module-database"
)

// moduleDatabaseUserInvalid matches what a database user name can't hold.
var moduleDatabaseUserInvalid = regexp.MustCompile(`[^a-z0-9_]+`)

// ModuleDatabaseUser returns the database user of the module whose name, in
// kebab case, is kebab: threeport_ and the name in snake case.
func ModuleDatabaseUser(kebab string) string {
	name := moduleDatabaseUserInvalid.ReplaceAllString(strings.ToLower(kebab), "_")
	user := moduleDatabaseUserPrefix + strings.Trim(name, "_")
	if len(user) > moduleDatabaseUserMaxLength {
		user = strings.TrimRight(user[:moduleDatabaseUserMaxLength], "_")
	}

	return user
}

// ModuleDatabaseEnv returns the environment a module's database clients
// take their user and password from: its credentials secret.  A process
// that also loads the control plane's config file keeps these, as the
// file doesn't override what the environment already sets.
func ModuleDatabaseEnv() []interface{} {
	env := make([]interface{}, 0, 2)
	for _, key := range []string{"DB_USER", "DB_PASSWORD"} {
		env = append(env, map[string]interface{}{
			"name": key,
			"valueFrom": map[string]interface{}{
				"secretKeyRef": map[string]interface{}{
					"name": ModuleDatabaseCredsSecretName,
					"key":  key,
				},
			},
		})
	}

	return env
}

// moduleDatabaseUserSecretName is the control plane's secret holding the
// password of a module's user.  Deleting it rotates the password on the
// module's next install.
func moduleDatabaseUserSecretName(kebab string) string {
	return truncateName(fmt.Sprintf("db-module-user-%s", kebab))
}

// moduleDatabaseJobName is the Job that creates, rotates or drops a module's
// user.
func moduleDatabaseJobName(action, kebab string) string {
	return truncateName(fmt.Sprintf("threeport-db-%s-%s", action, kebab))
}

// truncateName shortens a Kubernetes object name to the 63 characters a
// label value, such as a Job's name on its pods, allows.
func truncateName(name string) string {
	if len(name) <= 63 {
		return name
	}

	return strings.TrimRight(name[:63], "-")
}

// ModuleDatabaseUserStatements returns the statements that create a module's
// user, or update it, with password: a member of the threeport user and of
// the audited role, which can use the control plane's database, and whose
// tables the threeport user can use too.  They also turn on the audit log of
// the modules' statements.  Each is idempotent.
func ModuleDatabaseUserStatements(user, password string) []string {
	return []string{
		fmt.Sprintf("USE %s", database.ThreeportDatabaseName),
		fmt.Sprintf("CREATE USER IF NOT EXISTS %s", database.ThreeportDatabaseUser),
		fmt.Sprintf("CREATE ROLE IF NOT EXISTS %s", moduleDatabaseAuditRole),
		fmt.Sprintf("CREATE USER IF NOT EXISTS %s", user),
		fmt.Sprintf("ALTER USER %s WITH LOGIN PASSWORD '%s'", user, password),
		fmt.Sprintf("GRANT %s, %s TO %s", database.ThreeportDatabaseUser, moduleDatabaseAuditRole, user),
		fmt.Sprintf("GRANT ALL ON DATABASE %s TO %s", database.ThreeportDatabaseName, user),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES FOR ROLE %s GRANT ALL ON TABLES TO %s", user, database.ThreeportDatabaseUser),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES FOR ROLE %s GRANT ALL ON SEQUENCES TO %s", user, database.ThreeportDatabaseUser),
		fmt.Sprintf("SET CLUSTER SETTING sql.log.user_audit = '%s ALL'", moduleDatabaseAuditRole),
	}
}

// ModuleDatabaseDropStatements returns the statements that drop a module's
// user.  The tables it created are kept, handed to the threeport user, as
// the control plane's data may refer to them.  The default privileges it
// gave the threeport user are revoked first, as CockroachDB won't drop a
// user that still has them.
func ModuleDatabaseDropStatements(user string) []string {
	return []string{
		fmt.Sprintf("USE %s", database.ThreeportDatabaseName),
		fmt.Sprintf("REASSIGN OWNED BY %s TO %s", user, database.ThreeportDatabaseUser),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES FOR ROLE %s REVOKE ALL ON TABLES FROM %s", user, database.ThreeportDatabaseUser),
		fmt.Sprintf("ALTER DEFAULT PRIVILEGES FOR ROLE %s REVOKE ALL ON SEQUENCES FROM %s", user, database.ThreeportDatabaseUser),
		fmt.Sprintf("DROP OWNED BY %s", user),
		fmt.Sprintf("DROP USER IF EXISTS %s", user),
	}
}

// EnsureModuleDatabaseUser creates the database user of the module named
// kebab, or updates it, from the control plane in threeportNamespace, and
// writes what the module connects with to ModuleDatabaseCredsSecretName in
// moduleNamespace.  The password is kept in the control plane's namespace
// and reused, so installing the module again leaves its user as it is,
// unless that secret was deleted to rotate it.  It returns a digest of the
// credentials for the module's pod templates.
func EnsureModuleDatabaseUser(
	kubeClient dynamic.Interface,
	threeportNamespace string,
	moduleNamespace string,
	kebab string,
) (string, error) {
	user := ModuleDatabaseUser(kebab)
	password, err := moduleDatabasePassword(kubeClient, threeportNamespace, kebab)
	if err != nil {
		return "", err
	}

	if err := runModuleDatabaseJob(
		kubeClient,
		threeportNamespace,
		moduleDatabaseJobName("user", kebab),
		ModuleDatabaseUserScript(user, password),
	); err != nil {
		return "", fmt.Errorf("failed to create database user %s: %w", user, err)
	}

	// the module verifies the database with the CA's certificate the
	// control plane's own clients use
	threeportCerts, err := kubeClient.Resource(secretGVR).Namespace(threeportNamespace).Get(
		context.Background(), dbThreeportCertSecretName, metav1.GetOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to read the database's CA certificate: %w", err)
	}
	caCert, err := secretValue(threeportCerts, "ca.crt")
	if err != nil {
		return "", err
	}
	creds := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      ModuleDatabaseCredsSecretName,
			"namespace": moduleNamespace,
		},
		"type": "Opaque",
		"data": map[string]interface{}{
			"ca.crt":      base64.StdEncoding.EncodeToString([]byte(caCert)),
			"DB_USER":     base64.StdEncoding.EncodeToString([]byte(user)),
			"DB_PASSWORD": base64.StdEncoding.EncodeToString([]byte(password)),
		},
	}}
	if err := applySecret(kubeClient, creds); err != nil {
		return "", fmt.Errorf("failed to write the module's database credentials: %w", err)
	}

	digest := sha256.Sum256([]byte(user + "\n" + password + "\n" + caCert))

	return hex.EncodeToString(digest[:]), nil
}

// DropModuleDatabaseUser drops the database user of the module named kebab
// from the control plane in threeportNamespace, and deletes its password, so
// the module can no longer connect.  The control plane and other modules
// keep their own users.  A user that doesn't exist is left alone.
func DropModuleDatabaseUser(kubeClient dynamic.Interface, threeportNamespace, kebab string) error {
	user := ModuleDatabaseUser(kebab)
	if err := runModuleDatabaseJob(
		kubeClient, threeportNamespace, moduleDatabaseJobName("drop", kebab), ModuleDatabaseDropScript(user),
	); err != nil {
		return fmt.Errorf("failed to drop database user %s: %w", user, err)
	}

	if err := kubeClient.Resource(secretGVR).Namespace(threeportNamespace).Delete(
		context.Background(), moduleDatabaseUserSecretName(kebab), metav1.DeleteOptions{},
	); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete the password of database user %s: %w", user, err)
	}

	return nil
}

// moduleDatabasePassword returns the password of a module's user, which the
// control plane keeps in its namespace, and makes one when there's none.
func moduleDatabasePassword(kubeClient dynamic.Interface, namespace, kebab string) (string, error) {
	name := moduleDatabaseUserSecretName(kebab)
	secrets := kubeClient.Resource(secretGVR).Namespace(namespace)
	existing, err := secrets.Get(context.Background(), name, metav1.GetOptions{})
	if err == nil {
		password, err := secretValue(existing, moduleDatabasePasswordKey)
		if err == nil && password != "" {
			return password, nil
		}
	} else if !k8serrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to read the password of a module's database user: %w", err)
	}

	password, err := newModuleDatabasePassword()
	if err != nil {
		return "", err
	}
	secret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"type": "Opaque",
		"data": map[string]interface{}{
			moduleDatabasePasswordKey: base64.StdEncoding.EncodeToString([]byte(password)),
		},
	}}
	setPersistent(secret)
	if err := applySecret(kubeClient, secret); err != nil {
		return "", fmt.Errorf("failed to keep the password of a module's database user: %w", err)
	}

	return password, nil
}

// newModuleDatabasePassword returns a random password of 32 bytes, written
// with only letters, digits, - and _, so it needs no quoting in SQL.
func newModuleDatabasePassword() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate a database password: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// applySecret creates a secret, or replaces the data of the one there.
func applySecret(kubeClient dynamic.Interface, secret *unstructured.Unstructured) error {
	secrets := kubeClient.Resource(secretGVR).Namespace(secret.GetNamespace())
	existing, err := secrets.Get(context.Background(), secret.GetName(), metav1.GetOptions{})
	switch {
	case k8serrors.IsNotFound(err):
		_, err = secrets.Create(context.Background(), secret, metav1.CreateOptions{})
		return err
	case err != nil:
		return err
	}
	existing.Object["data"] = secret.Object["data"]
	delete(existing.Object, "stringData")
	labels := existing.GetLabels()
	for key, value := range secret.GetLabels() {
		if labels == nil {
			labels = map[string]string{}
		}
		labels[key] = value
	}
	existing.SetLabels(labels)
	_, err = secrets.Update(context.Background(), existing, metav1.UpdateOptions{})

	return err
}

// cockroachSQL is the database client command a module user Job runs, as
// root, with the certificates the control plane's database was created with.
var cockroachSQL = fmt.Sprintf(
	"/cockroach/cockroach sql --certs-dir=%s --host=%s --port=%s",
	dbRootCertsMountPath, database.ThreeportDatabaseHost, database.ThreeportDatabasePort,
)

// sqlStatements runs statements in one client session, each on its own, as
// SET CLUSTER SETTING can't run in a transaction with others.  They are
// passed on the client's standard input, so a password in them appears in
// neither the Job's spec nor its command line.
func sqlStatements(statements []string) string {
	return fmt.Sprintf("%s <<'EOF'\n%s;\nEOF\n", cockroachSQL, strings.Join(statements, ";\n"))
}

// ModuleDatabaseUserScript is the script a module user Job runs to create
// a module's user, or update it, with password.
func ModuleDatabaseUserScript(user, password string) string {
	return "set -e\n" + sqlStatements(ModuleDatabaseUserStatements(user, password))
}

// ModuleDatabaseDropScript is the script a module user Job runs to drop a
// module's user, if it exists.
func ModuleDatabaseDropScript(user string) string {
	exists := fmt.Sprintf("SELECT count(*) FROM [SHOW USERS] WHERE username = '%s'", user)

	return fmt.Sprintf(`set -e
count=$(%s --format=csv --execute %q | tail -n 1)
if [ "$count" = "0" ]; then
  echo "database user %s does not exist"
  exit 0
fi
%s`,
		cockroachSQL, exists, user, sqlStatements(ModuleDatabaseDropStatements(user)),
	)
}

// runModuleDatabaseJob runs a module user Job.  A test stands in for it.
var runModuleDatabaseJob = runModuleDatabaseJobInCluster

// runModuleDatabaseJobInCluster runs script in a Job in the control plane's
// namespace, with the database's root credentials, and waits for it.  The
// script is kept in a secret of the Job's name, as it may hold a password,
// and both are deleted once the Job succeeded.  A Job that failed is left
// for its pod's logs to be read.
func runModuleDatabaseJobInCluster(kubeClient dynamic.Interface, namespace, jobName, script string) error {
	if err := deleteJob(kubeClient, namespace, jobName); err != nil {
		return fmt.Errorf("failed to remove the leftover job %s: %w", jobName, err)
	}

	scriptSecret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      jobName,
			"namespace": namespace,
		},
		"type": "Opaque",
		"data": map[string]interface{}{
			moduleDatabaseScriptKey: base64.StdEncoding.EncodeToString([]byte(script)),
		},
	}}
	if err := applySecret(kubeClient, scriptSecret); err != nil {
		return fmt.Errorf("failed to write the script of job %s: %w", jobName, err)
	}

	job := moduleDatabaseJob(namespace, jobName)
	if _, err := kubeClient.Resource(jobGVR).Namespace(namespace).Create(
		context.Background(), job, metav1.CreateOptions{},
	); err != nil {
		return fmt.Errorf("failed to create job %s: %w", jobName, err)
	}

	succeeded, failure, err := waitForRestoreJob(kubeClient, namespace, jobName)
	if err != nil {
		if deleteErr := deleteJob(kubeClient, namespace, jobName); deleteErr != nil {
			return fmt.Errorf("job %s did not finish (%w) and could not be deleted: %w", jobName, err, deleteErr)
		}
		return fmt.Errorf("job %s did not finish and was stopped: %w", jobName, err)
	}
	if !succeeded {
		return fmt.Errorf("job %s failed (%s): inspect its pod logs in namespace %s", jobName, failure, namespace)
	}

	if err := deleteJob(kubeClient, namespace, jobName); err != nil {
		return err
	}
	if err := kubeClient.Resource(secretGVR).Namespace(namespace).Delete(
		context.Background(), jobName, metav1.DeleteOptions{},
	); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete the script of job %s: %w", jobName, err)
	}

	return nil
}

// moduleDatabaseJob returns the Job running the script in the secret of its
// name, as root, with the database's own certificates, which exist from the
// start of the install.  Its pod carries the label the database's network
// policy admits.
func moduleDatabaseJob(namespace, jobName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      jobName,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"backoffLimit":          int64(3),
			"activeDeadlineSeconds": int64(restoreDatabaseTimeout.Seconds()),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{LabelMaintenance: "true"},
				},
				"spec": map[string]interface{}{
					"restartPolicy": "Never",
					// the database's own user, whose group can read the
					// root-owned certificates
					"securityContext": dependencyPodSecurityContext(),
					"containers": []interface{}{
						map[string]interface{}{
							"name":            "module-database",
							"securityContext": restrictedContainerSecurityContext(),
							"image":           fmt.Sprintf("docker.io/cockroachdb/cockroach:%s", DatabaseImageTag),
							"command": []interface{}{
								"/bin/sh", fmt.Sprintf("%s/%s", moduleDatabaseScriptPath, moduleDatabaseScriptKey),
							},
							"volumeMounts": []interface{}{
								map[string]interface{}{
									"name":      dbCredsSecretName,
									"mountPath": dbRootCertsMountPath,
								},
								map[string]interface{}{
									"name":      "script",
									"mountPath": moduleDatabaseScriptPath,
								},
							},
						},
					},
					"volumes": []interface{}{
						map[string]interface{}{
							"name": dbCredsSecretName,
							"secret": map[string]interface{}{
								"secretName":  dbCredsSecretName,
								"defaultMode": int64(256),
							},
						},
						map[string]interface{}{
							"name": "script",
							"secret": map[string]interface{}{
								"secretName":  jobName,
								"defaultMode": int64(256),
							},
						},
					},
				},
			},
		},
	}}
}
