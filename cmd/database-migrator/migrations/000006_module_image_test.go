package migrations

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestModuleImageColumnsAcceptAWrite creates a module table without the image
// columns, runs the migration, and inserts a module row.
func TestModuleImageColumnsAcceptAWrite(t *testing.T) {
	// create the table the way an older install left it
	db, err := sql.Open("sqlite3", t.TempDir()+"/api.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE v0_module_apis (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO v0_module_apis (image_namespace, image_tag) VALUES ('repo', 'tag')`); err == nil {
		t.Fatal("insert succeeded before the columns existed")
	}

	// add the columns and write a module image
	if err := Up000006(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO v0_module_apis (image_namespace, image_tag) VALUES ('repo', 'tag')`); err != nil {
		t.Fatal(err)
	}
	var namespace, tag string
	if err := db.QueryRow(`SELECT image_namespace, image_tag FROM v0_module_apis`).Scan(&namespace, &tag); err != nil {
		t.Fatal(err)
	}
	if namespace != "repo" || tag != "tag" {
		t.Fatalf("stored %s %s", namespace, tag)
	}
}

// TestModuleImageColumnsAlreadyExist runs the migration on a current table.
func TestModuleImageColumnsAlreadyExist(t *testing.T) {
	// create the table the way a fresh install leaves it
	db, err := sql.Open("sqlite3", t.TempDir()+"/api.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE v0_module_apis (
		id INTEGER PRIMARY KEY,
		image_namespace STRING,
		image_tag STRING
	)`); err != nil {
		t.Fatal(err)
	}
	if err := Up000006(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO v0_module_apis (image_namespace, image_tag) VALUES ('repo', 'tag')`); err != nil {
		t.Fatal(err)
	}
}
