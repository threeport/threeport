package v0

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gzippedTar returns a gzip-compressed tar holding one regular file named name.
func gzippedTar(t *testing.T, name string, content []byte) *bytes.Buffer {
	t.Helper()
	// wrap gzip then tar around an in-memory buffer
	buf := &bytes.Buffer{}
	gzWriter := gzip.NewWriter(buf)
	tarWriter := tar.NewWriter(gzWriter)

	// write one regular file at name
	header := &tar.Header{
		Name:     name,
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("WriteHeader error: %v", err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatalf("Write error: %v", err)
	}
	// close tar then gzip so both trailers land
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tar Close error: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip Close error: %v", err)
	}
	return buf
}

// zippedFile writes a zip holding one file named name and returns its path.
func zippedFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	// create the zip in a temp directory
	path := filepath.Join(t.TempDir(), "archive.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// write one zip entry at name
	zipWriter := zip.NewWriter(file)
	entry, err := zipWriter.Create(name)
	if err != nil {
		t.Fatalf("Create zip entry error: %v", err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatalf("Write error: %v", err)
	}
	// close the zip writer then the file
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("zip Close error: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close error: %v", err)
	}
	return path
}

// TestReleaseAssetInfix_MapsOSAndArch covers releaseAssetInfix mapping GOOS
// and GOARCH to goreleaser archive infixes.
func TestReleaseAssetInfix_MapsOSAndArch(t *testing.T) {
	// GOOS/GOARCH pairs and the goreleaser archive infix
	cases := []struct {
		goos   string
		goarch string
		want   string
	}{
		{"linux", "amd64", "Linux_x86_64"},
		{"linux", "arm64", "Linux_arm64"},
		{"linux", "386", "Linux_i386"},
		{"darwin", "amd64", "Darwin_x86_64"},
		{"darwin", "arm64", "Darwin_arm64"},
		{"windows", "amd64", "Windows_x86_64"},
		{"windows", "arm64", "Windows_arm64"},
	}

	for _, c := range cases {
		// map the pair
		got := releaseAssetInfix(c.goos, c.goarch)
		// assert the goreleaser infix
		if got != c.want {
			t.Errorf("releaseAssetInfix(%q, %q)=%q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

// TestReleaseAssetSuffix_SelectsArchiveExtension covers releaseAssetSuffix
// using .zip on windows and .tar.gz on linux and darwin.
func TestReleaseAssetSuffix_SelectsArchiveExtension(t *testing.T) {
	// GOOS/GOARCH pairs and the goreleaser archive suffix
	cases := []struct {
		goos   string
		goarch string
		want   string
	}{
		{"linux", "amd64", "_Linux_x86_64.tar.gz"},
		{"darwin", "arm64", "_Darwin_arm64.tar.gz"},
		{"windows", "amd64", "_Windows_x86_64.zip"},
		{"windows", "arm64", "_Windows_arm64.zip"},
	}

	for _, c := range cases {
		// build the suffix
		got := releaseAssetSuffix(c.goos, c.goarch)
		// assert the leading underscore, infix, and archive extension
		if got != c.want {
			t.Errorf("releaseAssetSuffix(%q, %q)=%q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

// TestExtractBinary_InstallsExecutableFromVersionedDir covers extractBinary
// installing a matching binary at destDir by basename from a nested archive path.
func TestExtractBinary_InstallsExecutableFromVersionedDir(t *testing.T) {
	// a gzip tar with the binary nested under a versioned directory
	want := []byte("threeport-sdk binary bytes")
	archive := gzippedTar(t, "v1.2.3-dist/threeport-sdk", want)
	destDir := t.TempDir()

	// extract the matching basename into destDir
	if err := extractBinary(archive, "threeport-sdk", destDir); err != nil {
		t.Fatalf("extractBinary error: %v", err)
	}

	// install at destDir/threeport-sdk, not nested under the versioned directory
	destPath := filepath.Join(destDir, "threeport-sdk")
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}

	// assert mode 0755
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%o, want 0755", info.Mode().Perm())
	}

	// assert the dest bytes match the archive entry
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("contents=%q, want %q", got, want)
	}
}

// TestExtractBinary_RejectsArchiveWithoutMatch covers extractBinary returning
// an error when no archive entry matches the binary name.
func TestExtractBinary_RejectsArchiveWithoutMatch(t *testing.T) {
	// a gzip tar whose only file is a different basename
	archive := gzippedTar(t, "v1.2.3-dist/other-binary", []byte("nope"))
	destDir := t.TempDir()

	// extract looking for threeport-sdk
	if err := extractBinary(archive, "threeport-sdk", destDir); err == nil {
		t.Fatalf("expected error for missing binary, got nil")
	}
}

// TestExtractBinary_LeavesNoPartialBinaryOnTruncatedArchive covers extractBinary
// leaving destDir empty when the archive is truncated mid-stream.
func TestExtractBinary_LeavesNoPartialBinaryOnTruncatedArchive(t *testing.T) {
	// a valid gzip tar then a reader missing the last 64 bytes
	full := gzippedTar(t, "v1.2.3-dist/threeport-sdk", bytes.Repeat([]byte("x"), 4096))
	truncated := bytes.NewReader(full.Bytes()[:full.Len()-64])
	destDir := t.TempDir()

	// extract the truncated archive
	if err := extractBinary(truncated, "threeport-sdk", destDir); err == nil {
		t.Fatalf("expected error for truncated archive, got nil")
	}

	// check no dest binary a later run could execute
	if _, err := os.Stat(filepath.Join(destDir, "threeport-sdk")); !os.IsNotExist(err) {
		t.Fatalf("Stat error=%v, want a not-exist error", err)
	}

	// check destDir is empty
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("destDir holds %d entries, want 0", len(entries))
	}
}

// TestExtractZip_InstallsExecutableFromVersionedDir covers extractZip installing
// a matching .exe from a versioned archive directory.
func TestExtractZip_InstallsExecutableFromVersionedDir(t *testing.T) {
	// a zip with tptctl.exe nested under a versioned directory
	want := []byte("tptctl windows binary bytes")
	archivePath := zippedFile(t, "v1.2.3-dist/tptctl.exe", want)
	destDir := t.TempDir()

	// extract looking for tptctl without the .exe suffix
	if err := extractZip(archivePath, "tptctl", destDir); err != nil {
		t.Fatalf("extractZip error: %v", err)
	}

	// install as tptctl.exe, the archive entry's basename
	destPath := filepath.Join(destDir, "tptctl.exe")
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}

	// assert mode 0755
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%o, want 0755", info.Mode().Perm())
	}

	// assert the dest bytes match the zip entry
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("contents=%q, want %q", got, want)
	}
}

// TestExtractZip_RejectsArchiveWithoutMatch covers extractZip returning an
// error when no zip entry matches the binary name or its .exe form.
func TestExtractZip_RejectsArchiveWithoutMatch(t *testing.T) {
	// a zip whose only file is a different basename
	archivePath := zippedFile(t, "v1.2.3-dist/other-binary.exe", []byte("nope"))
	destDir := t.TempDir()

	// extract looking for tptctl
	if err := extractZip(archivePath, "tptctl", destDir); err == nil {
		t.Fatalf("expected error for missing tptctl.exe, got nil")
	}
}

// TestParseChecksums_SelectsTheRequestedAsset covers parseChecksums returning
// the digest for the named asset among several checksum lines.
func TestParseChecksums_SelectsTheRequestedAsset(t *testing.T) {
	// three checksums.txt lines plus a trailing blank
	body := strings.Join([]string{
		"1111111111111111111111111111111111111111111111111111111111111111  threeport_Darwin_arm64.tar.gz",
		"2222222222222222222222222222222222222222222222222222222222222222  threeport_Linux_x86_64.tar.gz",
		"3333333333333333333333333333333333333333333333333333333333333333  threeport_Linux_arm64.tar.gz",
		"",
	}, "\n")

	// parse the linux amd64 asset
	got, err := parseChecksums(body, "threeport_Linux_x86_64.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksums error: %v", err)
	}

	// assert the matching line's digest
	want := "2222222222222222222222222222222222222222222222222222222222222222"
	if got != want {
		t.Fatalf("digest=%q, want %q", got, want)
	}
}

// TestParseChecksums_RejectsMissingAsset covers parseChecksums returning an
// error when the named asset is absent.
func TestParseChecksums_RejectsMissingAsset(t *testing.T) {
	// a checksums body that lists a different asset
	body := "4444444444444444444444444444444444444444444444444444444444444444  threeport_Linux_arm64.tar.gz\n"
	// parse a linux amd64 name that is not listed
	if _, err := parseChecksums(body, "threeport_Linux_x86_64.tar.gz"); err == nil {
		t.Fatalf("expected error for missing asset, got nil")
	}
}

// TestValidateRepo_RejectsPathTraversal covers validateRepo accepting an
// owner/name path and rejecting traversal and malformed values.
func TestValidateRepo_RejectsPathTraversal(t *testing.T) {
	// owner/name paths, traversal, and malformed values
	cases := []struct {
		repo string
		ok   bool
	}{
		{"threeport/threeport", true},
		{"a.b-c/d_e.f", true},
		{"../threeport", false},
		{"threeport/..", false},
		{"threeport", false},
		{"a/b/c", false},
		{"", false},
		{"threeport/thr eeport", false},
		{".hidden/threeport", false},
		{"threeport/threeport/", false},
		{"http://x/threeport", false},
	}

	for _, c := range cases {
		// validate the repository path
		err := validateRepo(c.repo)
		// assert accepted paths return nil
		if c.ok && err != nil {
			t.Errorf("validateRepo(%q) error=%v, want nil", c.repo, err)
		}
		// assert rejected paths return an error
		if !c.ok && err == nil {
			t.Errorf("validateRepo(%q)=nil, want an error", c.repo)
		}
	}
}

// TestValidateTag_RejectsPathTraversal covers validateTag accepting v-prefixed
// versions, including prerelease and pseudo-version forms, and rejecting traversal.
func TestValidateTag_RejectsPathTraversal(t *testing.T) {
	// GA, dev, rc, and go pseudo-version tags, plus traversal and unprefixed values
	cases := []struct {
		tag string
		ok  bool
	}{
		{"v0.7.0", true},
		{"v0.7.0-dev.19", true},
		{"v0.7.0-rc.1", true},
		{"v0.7.0-0.20260731214756-abcdef123456", true},
		{"../v0.7.0", false},
		{"v0.7.0/../../x", false},
		{"0.7.0", false},
		{"latest", false},
		{"", false},
	}

	for _, c := range cases {
		// validate the release tag
		err := validateTag(c.tag)
		// assert accepted tags return nil
		if c.ok && err != nil {
			t.Errorf("validateTag(%q) error=%v, want nil", c.tag, err)
		}
		// assert rejected tags return an error
		if !c.ok && err == nil {
			t.Errorf("validateTag(%q)=nil, want an error", c.tag)
		}
	}
}

// TestTokenBearingHost_LimitsTheCredentialToGithub covers tokenBearingHost
// accepting GitHub hosts and rejecting lookalikes.
func TestTokenBearingHost_LimitsTheCredentialToGithub(t *testing.T) {
	// GitHub hosts, the asset-download redirect, and suffix or prefix lookalikes
	cases := []struct {
		host string
		ok   bool
	}{
		{"github.com", true},
		{"api.github.com", true},
		{"objects.githubusercontent.com", true},
		{"attacker.com", false},
		{"github.com.attacker.com", false},
		{"githubusercontent.com.attacker.com", false},
		{"notgithub.com", false},
		{"", false},
	}

	for _, c := range cases {
		// assert the host classification
		if got := tokenBearingHost(c.host); got != c.ok {
			t.Errorf("tokenBearingHost(%q)=%v, want %v", c.host, got, c.ok)
		}
	}
}

// TestGithubGet_BoundsTheRequestByTimeout covers githubGet returning
// context.DeadlineExceeded for a one-nanosecond client timeout.
func TestGithubGet_BoundsTheRequestByTimeout(t *testing.T) {
	// a one-nanosecond client timeout
	_, err := githubGet(
		"https://api.github.com/repos/example/example/releases/tags/v1.0.0",
		"", "", time.Nanosecond,
	)
	// check the call fails
	if err == nil {
		t.Fatal("githubGet returned no error, want the deadline to have been exceeded")
	}
	// check the failure is the deadline, not a transport error
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("githubGet error = %v, want context.DeadlineExceeded", err)
	}
}

// TestReleaseMetadataURL_KeepsTheOwnerAndNameSeparate covers releaseMetadataURL
// putting owner and name in separate path segments.
func TestReleaseMetadataURL_KeepsTheOwnerAndNameSeparate(t *testing.T) {
	// build the release metadata URL for a fork owner and a prerelease tag
	got := releaseMetadataURL("randalljohnson/threeport", "v0.7.0-dev.23")

	// assert the GitHub releases/tags URL
	want := "https://api.github.com/repos/randalljohnson/threeport/releases/tags/v0.7.0-dev.23"
	if got != want {
		t.Errorf("releaseMetadataURL = %q, want %q", got, want)
	}
	// reject a fused owner/name %2F that 404s on GitHub
	if strings.Contains(got, "%2F") {
		t.Errorf("releaseMetadataURL = %q, escaped the owner and name into one segment", got)
	}
}
