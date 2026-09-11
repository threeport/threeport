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

// These tests run generated handlers against a real CockroachDB in docker.

// databaseName is the database the API server writes to.
const databaseName = "threeport_api"

// startTimeout is how long the container gets to answer its first query.
const startTimeout = 60 * time.Second

// testDb is shared by every test in the package. Rows outlive the test
// that wrote them, so each test picks values no other test uses.
var testDb *gorm.DB

// testPort is the host port docker published for SQL.
var testPort string

// TestMain starts one CockroachDB container for the package.
func TestMain(m *testing.M) {
	// skip when docker is not on PATH
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Println("docker not available; skipping the database tests")
		os.Exit(0)
	}

	// start one container for the package
	container, port, err := startCockroach()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start cockroachdb: %v\n", err)
		os.Exit(1)
	}

	// os.Exit skips defers; every exit path below removes the container itself
	defer func() {
		if err := exec.Command("docker", "rm", "--force", container).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to remove the cockroachdb container: %v\n", err)
		}
	}()

	// publish the host port and build the shared schema
	testPort = port
	testDb, err = openSchema(port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build the schema: %v\n", err)
		exec.Command("docker", "rm", "--force", container).Run()
		os.Exit(1)
	}

	// run the package tests
	code := m.Run()

	// remove the container and return their exit code
	exec.Command("docker", "rm", "--force", container).Run()
	os.Exit(code)
}

// startCockroach runs a single-node CockroachDB and returns its id and host port.
func startCockroach() (string, string, error) {
	// start a detached single-node container
	run := exec.Command(
		"docker", "run", "--detach",
		"--publish", "26257",
		fmt.Sprintf("cockroachdb/cockroach:%s", installer.DatabaseImageTag),
		"start-single-node", "--insecure",
	)
	var stdout, stderr bytes.Buffer
	run.Stdout = &stdout
	run.Stderr = &stderr
	if err := run.Run(); err != nil {
		return "", "", fmt.Errorf("failed to run the container: %w: %s", err, stderr.String())
	}
	container := strings.TrimSpace(stdout.String())

	// read the host port docker published for 26257
	out, err := exec.Command("docker", "port", container, "26257/tcp").CombinedOutput()
	if err != nil {
		return container, "", fmt.Errorf("failed to read the published port: %w: %s", err, out)
	}

	// take the port after the last colon of the first mapping
	mapping := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	index := strings.LastIndex(mapping, ":")
	if index < 0 {
		return container, "", fmt.Errorf("published port not found in %q", mapping)
	}

	return container, mapping[index+1:], nil
}

// openSchema waits for SQL, creates the API database, and AutoMigrates the test models.
func openSchema(port string) (*gorm.DB, error) {
	// wait until SQL accepts connections
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

	// open a handle on it
	db, err := open(port, databaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to open the database: %w", err)
	}

	// AutoMigrate the models the handler tests share
	if err := db.AutoMigrate(
		&api_v0.AttachedObjectReference{},
		&api_v0.DomainNameDefinition{},
		&api_v0.ModuleApi{},
		&api_v0.ModuleObject{},
		&api_v0.ModuleApiRoute{},
	); err != nil {
		return nil, fmt.Errorf("failed to build the schema: %w", err)
	}

	return db, nil
}

// freshDatabase creates an empty database in the test container.
// threeport-sdk gen calls this by name, so the name and signature are fixed.
func freshDatabase(t *testing.T, name string) *gorm.DB {
	t.Helper()

	// create an empty database in the test container
	if err := testDb.Exec(fmt.Sprintf("CREATE DATABASE %s", name)).Error; err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}

	// open a handle on it
	db, err := open(testPort, name)
	if err != nil {
		t.Fatalf("open database %s: %v", name, err)
	}

	return db
}

// open returns a gorm handle on one database in the test container.
func open(port, name string) (*gorm.DB, error) {
	return gorm.Open(
		postgres.New(postgres.Config{
			DSN: fmt.Sprintf(
				"postgres://root@127.0.0.1:%s/%s?sslmode=disable",
				port, name,
			),
		}),
		&gorm.Config{Logger: logger.Discard},
	)
}
