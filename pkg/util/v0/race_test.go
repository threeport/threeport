package v0

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRaceTestPackages_FindsRaceFiles covers listing packages that hold *_race_test.go.
func TestRaceTestPackages_FindsRaceFiles(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "internal", "provider")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "pulumi_workspace_race_test.go"), []byte("package provider\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "other_test.go"), []byte("package provider\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "ignored_race_test.go"), []byte("package git\n"), 0o644))
	testdata := filepath.Join(root, "testdata")
	require.NoError(t, os.MkdirAll(testdata, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(testdata, "fixture_race_test.go"), []byte("package testdata\n"), 0o644))

	got, err := RaceTestPackages(root)
	require.NoError(t, err)
	assert.Equal(t, []string{"./internal/provider"}, got)
}

// TestRaceTestPackages_Empty covers a tree with no race tests.
func TestRaceTestPackages_Empty(t *testing.T) {
	got, err := RaceTestPackages(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, got)
}
