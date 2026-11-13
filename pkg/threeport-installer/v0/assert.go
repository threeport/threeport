package v0

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"sort"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/pkg/api-server/v0/database"
	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const assertDatabaseJobName = "threeport-assert-database"

// AssertRestoredControlPlane requires certificates, database state, and ready workloads.
func (cpi *ControlPlaneInstaller) AssertRestoredControlPlane(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	// require the database client secrets to share one CA
	if err := cpi.databaseClientCerts(kubeClient); err != nil {
		return err
	}

	// require the database user, the database, and any backup schedule
	requireSchedule := cpi.Opts.GcpBackupBucket != ""
	if err := runRestorePhase(
		kubeClient,
		cpi.Opts.Namespace,
		assertDatabaseJobName,
		restoredDatabaseCheckSQL(requireSchedule),
		0,
	); err != nil {
		return fmt.Errorf("failed to check the restored database: %w", err)
	}

	// require the selected controllers, the API, and the agent to be ready
	if err := cpi.assertSelectedControllers(kubeClient); err != nil {
		return err
	}

	// require the database and the message broker to be ready
	if err := assertReplicasReady(
		kubeClient,
		statefulSetGVR,
		cpi.Opts.Namespace,
		"crdb",
	); err != nil {
		return err
	}
	if err := assertReplicasReady(
		kubeClient,
		statefulSetGVR,
		cpi.Opts.Namespace,
		natsServiceName,
	); err != nil {
		return err
	}

	// require the public API certificate to chain to the control plane CA
	if !cpi.Opts.AuthEnabled || cpi.Opts.ExternalApiEndpoint == "" {
		return nil
	}
	authConfig, err := cpi.LoadAuthConfigFromNamespace(kubeClient, mapper, cpi.Opts.Namespace)
	if err != nil {
		return fmt.Errorf("failed to read the control plane CA: %w", err)
	}
	return assertEndpointUsesCA(cpi.Opts.ExternalApiEndpoint, authConfig.CAPemEncoded)
}

// databaseClientCerts requires the root and database-user client secrets to share one CA.
func (cpi *ControlPlaneInstaller) databaseClientCerts(kubeClient dynamic.Interface) error {
	// require each client secret to hold its certificate and key
	namespace := cpi.Opts.Namespace
	keys := map[string][]string{
		dbRootCertSecretName:      {"ca.crt", "client.root.crt", "client.root.key"},
		dbThreeportCertSecretName: {"ca.crt", "client.threeport.crt", "client.threeport.key"},
	}
	cas := map[string]string{}
	for name, keyList := range keys {
		secret, err := kubeClient.Resource(secretGVR).Namespace(namespace).Get(
			context.Background(),
			name,
			metav1.GetOptions{},
		)
		if err != nil {
			return fmt.Errorf("failed to read secret %s: %w", name, err)
		}
		for _, key := range keyList {
			value, err := secretValue(secret, key)
			if err != nil {
				return err
			}
			if value == "" {
				return fmt.Errorf("secret %s %s must be set", name, key)
			}
			if key == "ca.crt" {
				cas[name] = value
			}
		}
	}

	// require both secrets to carry the same CA
	if cas[dbRootCertSecretName] != cas[dbThreeportCertSecretName] {
		return fmt.Errorf(
			"%s and %s must be the same CA",
			dbRootCertSecretName,
			dbThreeportCertSecretName,
		)
	}
	return nil
}

// assertSelectedControllers requires the installed controllers to match the selected list.
// Each selected controller, the API, and the agent must be ready.
func (cpi *ControlPlaneInstaller) assertSelectedControllers(
	kubeClient dynamic.Interface,
) error {
	// compare the installed controller names with the selected list
	installed, _, err := DetectInstalledControllerNames(kubeClient, cpi.Opts.Namespace)
	if err != nil {
		return fmt.Errorf("failed to read installed controllers: %w", err)
	}
	wanted := enabledControllerNames(cpi.Opts.ControllerList)
	if err := controllerSetDifference(wanted, filterKnownControllers(installed)); err != nil {
		return err
	}

	// check each selected controller deployment is ready
	for _, controller := range cpi.Opts.ControllerList {
		if controller == nil || (controller.Enabled != nil && !*controller.Enabled) {
			continue
		}
		if err := assertReplicasReady(
			kubeClient,
			deploymentGVR,
			cpi.Opts.Namespace,
			cpi.controllerDeploymentName(*controller),
		); err != nil {
			return err
		}
	}

	// check the API deployment is fully ready
	if err := assertReplicasReady(
		kubeClient,
		deploymentGVR,
		cpi.Opts.Namespace,
		ThreeportAPIServiceResourceName,
	); err != nil {
		return err
	}

	// check the agent deployment is fully ready
	if cpi.Opts.SkipAgent {
		return nil
	}
	return assertReplicasReady(
		kubeClient,
		deploymentGVR,
		cpi.Opts.Namespace,
		ThreeportAgentDeployName,
	)
}

// assertReplicasReady requires every requested replica of the workload to be ready.
func assertReplicasReady(
	kubeClient dynamic.Interface,
	gvr schema.GroupVersionResource,
	namespace, name string,
) error {
	// read the workload
	object, err := kubeClient.Resource(gvr).Namespace(namespace).Get(
		context.Background(),
		name,
		metav1.GetOptions{},
	)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("%s %s not found", gvr.Resource, name)
		}
		return fmt.Errorf("failed to read %s %s: %w", gvr.Resource, name, err)
	}

	// require every requested replica to be ready
	spec, specFound, err := util.NestedInt64OrFloat64(object.Object, "spec", "replicas")
	if err != nil {
		return fmt.Errorf("failed to read %s %s replicas: %w", gvr.Resource, name, err)
	}
	if !specFound || spec < 1 {
		return fmt.Errorf("%s %s must be ready", gvr.Resource, name)
	}
	ready, _, err := util.NestedInt64OrFloat64(object.Object, "status", "readyReplicas")
	if err != nil {
		return fmt.Errorf("failed to read %s %s ready replicas: %w", gvr.Resource, name, err)
	}
	if ready != spec {
		return fmt.Errorf("%s %s must be ready, got %d of %d", gvr.Resource, name, ready, spec)
	}
	return nil
}

// restoredDatabaseCheckSQL returns the statements that prove the restored database is usable.
func restoredDatabaseCheckSQL(requireSchedule bool) []string {
	// require the database user and the restored database
	statements := []string{
		databasePresenceCheck(
			"threeport user must exist",
			fmt.Sprintf(
				"SELECT count(*) FROM [SHOW USERS] WHERE username = '%s'",
				database.ThreeportDatabaseUser,
			),
		),
		databasePresenceCheck(
			"threeport_api database must exist",
			fmt.Sprintf(
				"SELECT count(*) FROM [SHOW DATABASES] WHERE database_name = '%s'",
				database.ThreeportDatabaseName,
			),
		),
	}
	if !requireSchedule {
		return statements
	}

	// require a backup schedule, since two API replicas can each create one
	return append(statements, databaseAtLeastOneCheck(
		"threeport_api_backup schedule must exist",
		"SELECT count(*) FROM [SHOW SCHEDULES] WHERE label = '"+DatabaseBackupScheduleLabel+"'",
	))
}

// databaseAtLeastOneCheck returns a statement that fails when the count is zero.
// Cockroach evaluates only the taken branch, so the error runs only on failure.
func databaseAtLeastOneCheck(message, countQuery string) string {
	// fail when no row matches
	return fmt.Sprintf(
		"SELECT CASE WHEN (%s) >= 1 THEN 1 ELSE crdb_internal.force_error('XX000', '%s') END",
		countQuery,
		message,
	)
}

// databasePresenceCheck returns a statement that fails unless the count is one.
// Cockroach evaluates only the taken branch, so the error runs only on failure.
func databasePresenceCheck(message, countQuery string) string {
	return fmt.Sprintf(
		"SELECT CASE WHEN (%s) = 1 THEN 1 ELSE crdb_internal.force_error('XX000', '%s') END",
		countQuery,
		message,
	)
}

// assertEndpointUsesCA requires the endpoint certificate to chain to the control plane CA.
func assertEndpointUsesCA(endpoint, caPEM string) error {
	// trust only the control plane CA
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(caPEM)) {
		return fmt.Errorf("control plane CA must be a certificate")
	}

	// dial the endpoint and require that chain
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", endpoint, &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		return fmt.Errorf(
			"failed to verify the rest-api certificate against the control plane CA: %w",
			err,
		)
	}
	return conn.Close()
}

// controllerSetDifference reports controllers present on only one of the two lists.
func controllerSetDifference(wanted, installed []string) error {
	// collect the names each side holds
	need := map[string]struct{}{}
	for _, name := range wanted {
		need[name] = struct{}{}
	}
	have := map[string]struct{}{}
	for _, name := range installed {
		have[name] = struct{}{}
	}

	// report a name that appears on only one side
	var missing, extra []string
	for _, name := range wanted {
		if _, ok := have[name]; !ok {
			missing = append(missing, name)
		}
	}
	for _, name := range installed {
		if _, ok := need[name]; !ok {
			extra = append(extra, name)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return fmt.Errorf("controllers must be the selected set, missing %v extra %v", missing, extra)
}

// filterKnownControllers keeps names that belong to the control plane controller list.
func filterKnownControllers(names []string) []string {
	// drop names that are not control plane controllers
	known := map[string]struct{}{}
	for _, controller := range ThreeportControllerList {
		known[controller.Name] = struct{}{}
	}
	var filtered []string
	for _, name := range names {
		if _, ok := known[name]; ok {
			filtered = append(filtered, name)
		}
	}
	sort.Strings(filtered)
	return filtered
}

// enabledControllerNames returns the controller names the install will run.
func enabledControllerNames(controllers []*api.ControlPlaneComponent) []string {
	// A nil enabled flag means the controller is installed.
	var names []string
	for _, controller := range controllers {
		if controller == nil || (controller.Enabled != nil && !*controller.Enabled) {
			continue
		}
		names = append(names, controller.Name)
	}
	sort.Strings(names)
	return names
}
