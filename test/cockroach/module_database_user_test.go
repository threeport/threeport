package cockroach

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// secureCockroach is a CockroachDB node that runs with certificates, as a
// control plane's does, which password authentication needs: the shared
// test node is insecure.  It answers as crdb, the host a module user Job
// connects to, with its certificates where the Job mounts them.
type secureCockroach struct {
	t         *testing.T
	container string
}

// startSecureCockroach starts a secure node, with client certificates for
// root and the threeport user, and the control plane's database, as an
// install leaves it.  It is removed when the test ends.
func startSecureCockroach(t *testing.T) *secureCockroach {
	t.Helper()
	container := fmt.Sprintf("threeport-module-users-%d", time.Now().UnixNano())
	certs := "/cockroach/cockroach-certs"
	start := strings.Join([]string{
		"set -e",
		"mkdir -p " + certs + " /ca",
		"/cockroach/cockroach cert create-ca --certs-dir=" + certs + " --ca-key=/ca/ca.key",
		"/cockroach/cockroach cert create-node crdb localhost 127.0.0.1 --certs-dir=" + certs + " --ca-key=/ca/ca.key",
		"/cockroach/cockroach cert create-client root --certs-dir=" + certs + " --ca-key=/ca/ca.key",
		"/cockroach/cockroach cert create-client threeport --certs-dir=" + certs + " --ca-key=/ca/ca.key",
		"exec /cockroach/cockroach start-single-node --certs-dir=" + certs,
	}, "\n")
	out, err := exec.Command(
		"docker", "run", "--detach", "--name", container,
		"--add-host", "crdb:127.0.0.1",
		"--entrypoint", "/bin/sh",
		fmt.Sprintf("cockroachdb/cockroach:%s", installer.DatabaseImageTag),
		"-c", start,
	).CombinedOutput()
	require.NoError(t, err, string(out))
	node := &secureCockroach{t: t, container: container}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "--force", container).Run() })

	require.Eventually(t, func() bool {
		_, err := node.sql("root", "SELECT 1")
		return err == nil
	}, 2*time.Minute, 2*time.Second, "the secure node never answered")

	// as the control plane's install leaves the database, with a table its
	// migrations created
	node.mustSQL("root",
		"CREATE USER IF NOT EXISTS threeport",
		"CREATE DATABASE IF NOT EXISTS threeport_api encoding='utf-8'",
		"GRANT ALL ON DATABASE threeport_api TO threeport",
	)
	node.mustSQL("threeport", "CREATE TABLE threeport_api.core (id INT PRIMARY KEY, name STRING)")

	return node
}

// run runs a script in the node's container, as a module user Job does.
func (n *secureCockroach) run(script string) (string, error) {
	command := exec.Command("docker", "exec", "-i", n.container, "/bin/sh", "-s")
	command.Stdin = strings.NewReader(script)
	out, err := command.CombinedOutput()

	return string(out), err
}

// sql runs statements as a user that authenticates with its certificate.
func (n *secureCockroach) sql(user string, statements ...string) (string, error) {
	args := []string{"exec", n.container, "/cockroach/cockroach", "sql",
		"--certs-dir=/cockroach/cockroach-certs", "--host=crdb", "--user=" + user,
		"--database=threeport_api", "--format=csv"}
	for _, statement := range statements {
		args = append(args, "--execute", statement)
	}
	out, err := exec.Command("docker", args...).CombinedOutput()

	return string(out), err
}

// mustSQL runs statements as a user that authenticates with its certificate,
// and fails the test when they fail.
func (n *secureCockroach) mustSQL(user string, statements ...string) string {
	n.t.Helper()
	out, err := n.sql(user, statements...)
	require.NoError(n.t, err, out)

	return out
}

// passwordSQL runs statements as a module does: with its user's password,
// verifying the node with the CA's certificate alone.
func (n *secureCockroach) passwordSQL(user, password string, statements ...string) (string, error) {
	url := fmt.Sprintf(
		"postgresql://%s:%s@crdb:26257/threeport_api?sslmode=verify-full&sslrootcert=/cockroach/cockroach-certs/ca.crt",
		user, password,
	)
	args := []string{"exec", n.container, "/cockroach/cockroach", "sql", "--url", url, "--format=csv"}
	for _, statement := range statements {
		args = append(args, "--execute", statement)
	}
	out, err := exec.Command("docker", args...).CombinedOutput()

	return string(out), err
}

// TestModuleDatabaseUsersOnCockroach runs the scripts a module user Job
// runs against a secure CockroachDB: each module gets a user of its own,
// which connects with its password, can do what modules did as the threeport
// user, is audited, and can be rotated, disabled or dropped without touching
// the threeport user or another module's.
func TestModuleDatabaseUsersOnCockroach(t *testing.T) {
	if testDb == nil {
		t.Skip("docker not available")
	}
	node := startSecureCockroach(t)
	acme := installer.ModuleDatabaseUser("acme")
	widget := installer.ModuleDatabaseUser("widget")

	out, err := node.run(installer.ModuleDatabaseUserScript(acme, "acme-password-1"))
	require.NoError(t, err, out)
	out, err = node.run(installer.ModuleDatabaseUserScript(widget, "widget-password"))
	require.NoError(t, err, out)
	out, err = node.run(installer.ModuleDatabaseUserScript(acme, "acme-password-1"))
	require.NoError(t, err, "creating a user again is a no-op: %s", out)

	// a module connects with its password, as itself
	out, err = node.passwordSQL(acme, "acme-password-1", "SELECT current_user")
	require.NoError(t, err, out)
	assert.Contains(t, out, acme)

	// it does what modules did as the threeport user: use and change the
	// control plane's tables, and create its own
	_, err = node.passwordSQL(acme, "acme-password-1",
		"INSERT INTO core VALUES (1, 'from acme')",
		"ALTER TABLE core ADD COLUMN acme_note STRING",
		"CREATE TABLE acme_widgets (id INT PRIMARY KEY)",
		"INSERT INTO acme_widgets VALUES (7)",
	)
	require.NoError(t, err)
	// whose tables the control plane, and its other modules, can use
	assert.Contains(t, node.mustSQL("threeport", "SELECT id FROM acme_widgets"), "7")
	out, err = node.passwordSQL(widget, "widget-password", "SELECT id FROM acme_widgets")
	require.NoError(t, err, out)
	assert.Contains(t, out, "7")

	// its statements are audited
	assert.Contains(t, node.mustSQL("root", "SHOW CLUSTER SETTING sql.log.user_audit"), "threeport_modules ALL")
	require.Eventually(t, func() bool {
		log, _ := exec.Command("docker", "exec", node.container, "/bin/sh", "-c",
			"cat /cockroach/cockroach-data/logs/cockroach-sql-audit.log").CombinedOutput()
		return strings.Contains(string(log), `"User":"‹`+acme+`›"`) &&
			strings.Contains(string(log), "acme_widgets")
	}, 30*time.Second, time.Second, "the module's statements are in the audit log, with its user")

	// a rotated password replaces the old one
	out, err = node.run(installer.ModuleDatabaseUserScript(acme, "acme-password-2"))
	require.NoError(t, err, out)
	_, err = node.passwordSQL(acme, "acme-password-1", "SELECT 1")
	assert.Error(t, err, "the old password")
	_, err = node.passwordSQL(acme, "acme-password-2", "SELECT 1")
	assert.NoError(t, err)

	// disabled, the module can't connect, and nothing else changes
	node.mustSQL("root", fmt.Sprintf("ALTER USER %s WITH NOLOGIN", acme))
	_, err = node.passwordSQL(acme, "acme-password-2", "SELECT 1")
	assert.Error(t, err)
	_, err = node.passwordSQL(widget, "widget-password", "SELECT count(*) FROM core")
	assert.NoError(t, err, "another module")
	node.mustSQL("threeport", "SELECT count(*) FROM core")
	// and creating it again lets it in
	out, err = node.run(installer.ModuleDatabaseUserScript(acme, "acme-password-2"))
	require.NoError(t, err, out)
	_, err = node.passwordSQL(acme, "acme-password-2", "SELECT 1")
	assert.NoError(t, err)

	// dropped, it is gone, and its tables are kept for the control plane
	out, err = node.run(installer.ModuleDatabaseDropScript(acme))
	require.NoError(t, err, out)
	assert.NotContains(t, node.mustSQL("root", "SHOW USERS"), acme)
	_, err = node.passwordSQL(acme, "acme-password-2", "SELECT 1")
	assert.Error(t, err)
	assert.Contains(t, node.mustSQL("root", "SELECT owner FROM [SHOW TABLES] WHERE table_name = 'acme_widgets'"), "threeport")
	assert.Contains(t, node.mustSQL("threeport", "SELECT id FROM acme_widgets"), "7")
	_, err = node.passwordSQL(widget, "widget-password", "SELECT id FROM acme_widgets")
	assert.NoError(t, err, "another module keeps its access")
	assert.Contains(t, node.mustSQL("root", "SHOW USERS"), "\nthreeport,", "the control plane's user stays")

	out, err = node.run(installer.ModuleDatabaseDropScript(acme))
	require.NoError(t, err, "dropping a user already dropped: %s", out)
	assert.Contains(t, out, "does not exist")
}
