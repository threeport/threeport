package cockroach

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// The API's write path recognizes a rejected write only as a typed pgx error
// carrying SQLSTATE 23505, and answers it 409 naming the API fields behind the
// conflicting columns. The sqlite database the handler unit tests run on cannot
// produce that error, so a gorm callback injects it there; this package runs
// the same handlers against a real CockroachDB in a docker container. It needs
// docker and no control plane, which is why mage test:cockroach is a target of
// its own rather than part of the integration suite.
//
// The container runs start-single-node --insecure, so root connects with no
// password and no TLS:
// https://docs.cockroachlabs.com/docs/stable/cockroach-start-single-node
//
// Docker pulls a missing image before it returns the container id, and picks
// the host port itself when a published port names no host side:
// https://docs.docker.com/reference/cli/docker/container/run/

// databaseName is the database the tests share, under the name the API server
// writes to. A deployed control plane has an init container create it, which
// this package does not run.
const databaseName = "threeport_api"

// startTimeout is how long the container gets to answer its first query. The
// image pull is already done when the clock starts, so the budget covers only
// the database coming up.
const startTimeout = 60 * time.Second

// testDb is the connection to databaseName every test in the package shares.
// Rows outlive the test that wrote them, so a test picks values no other test
// uses.
var testDb *gorm.DB

// testPort is the host port docker published the container's SQL port on.
var testPort string

// TestMain starts one CockroachDB container for the package, builds the schema
// in it, and removes the container once the tests finish.
func TestMain(m *testing.M) {
	// skip the package rather than fail it when docker is not installed
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Println("docker not available; skipping the database tests")
		os.Exit(0)
	}

	// start one container for every test in the package
	container, port, err := startCockroach()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start cockroachdb: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := exec.Command("docker", "rm", "--force", container).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to remove the cockroachdb container: %v\n", err)
		}
	}()

	// record the port and build the schema every test writes to
	testPort = port
	testDb, err = openSchema(port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build the schema: %v\n", err)
		// os.Exit runs no deferred call, so remove the container here too
		exec.Command("docker", "rm", "--force", container).Run()
		os.Exit(1)
	}

	// run the tests, then remove the container before exiting
	code := m.Run()

	exec.Command("docker", "rm", "--force", container).Run()
	os.Exit(code)
}

// startCockroach runs a single-node CockroachDB container and returns its id
// along with the host port docker published the SQL port on. Docker picks that
// port, so a run collides with neither a local database nor a second run.
func startCockroach() (string, string, error) {
	// run the CockroachDB version the installer deploys with a control plane
	run := exec.Command(
		"docker", "run", "--detach",
		"--publish", "26257",
		fmt.Sprintf("cockroachdb/cockroach:%s", installer.DatabaseImageTag),
		"start-single-node", "--insecure",
	)
	// read the container id off stdout alone, keeping docker's own messages out
	var stdout, stderr bytes.Buffer
	run.Stdout = &stdout
	run.Stderr = &stderr
	if err := run.Run(); err != nil {
		return "", "", fmt.Errorf("failed to run the container: %w: %s", err, stderr.String())
	}
	container := strings.TrimSpace(stdout.String())

	// read back the host port docker assigned
	out, err := exec.Command("docker", "port", container, "26257/tcp").CombinedOutput()
	if err != nil {
		return container, "", fmt.Errorf("failed to read the published port: %w: %s", err, out)
	}
	// take the port off the end of the first line, anchored on the last colon so
	// a host address carrying colons of its own stays whole
	mapping := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	index := strings.LastIndex(mapping, ":")
	if index < 0 {
		return container, "", fmt.Errorf("published port not found in %q", mapping)
	}

	return container, mapping[index+1:], nil
}

// openSchema waits for the container to accept queries, creates the API
// database, and builds its tables from the struct tags the deployed schema is
// built from.
func openSchema(port string) (*gorm.DB, error) {
	// wait for a statement to run rather than for a connection to open
	var root *gorm.DB
	deadline := time.Now().Add(startTimeout)
	for {
		var err error
		root, err = open(port, "defaultdb")
		if err == nil {
			err = root.Exec("SELECT 1").Error
		}
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("cockroachdb did not accept connections within %s: %w", startTimeout, err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// create the API database
	if err := root.Exec(fmt.Sprintf("CREATE DATABASE %s", databaseName)).Error; err != nil {
		return nil, fmt.Errorf("failed to create the database: %w", err)
	}

	db, err := open(port, databaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to open the database: %w", err)
	}

	// build the tables the tests write to
	if err := db.AutoMigrate(
		&api_v0.AttachedObjectReference{},
		&api_v0.DomainNameDefinition{},
		&api_v0.ModuleApi{},
	); err != nil {
		return nil, fmt.Errorf("failed to build the schema: %w", err)
	}

	return db, nil
}

// freshDatabase creates a database in the test container and returns a handle
// on it, so a test asserting what a migration chain builds is not reading
// tables another test created. threeport-sdk gen emits a call to it by name, so
// the name and signature are fixed.
func freshDatabase(t *testing.T, name string) *gorm.DB {
	t.Helper()

	if err := testDb.Exec(fmt.Sprintf("CREATE DATABASE %s", name)).Error; err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}

	db, err := open(testPort, name)
	if err != nil {
		t.Fatalf("open database %s: %v", name, err)
	}

	return db
}

// open returns a gorm handle on one database in the test container. A deployed
// API server authenticates with a client certificate instead, which does not
// change how the database answers a rejected write.
func open(port, name string) (*gorm.DB, error) {
	return gorm.Open(
		postgres.New(postgres.Config{
			DSN: fmt.Sprintf(
				"postgres://root@127.0.0.1:%s/%s?sslmode=disable",
				port, name,
			),
		}),
		// the tests provoke rejected writes, so gorm's error log is noise
		&gorm.Config{Logger: logger.Discard},
	)
}
