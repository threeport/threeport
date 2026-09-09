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

// gzippedTar builds an in-memory gzipped tar containing one regular-file entry
// at name holding content.
func gzippedTar(t *testing.T, name string, content []byte) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	gzWriter := gzip.NewWriter(buf)
	tarWriter := tar.NewWriter(gzWriter)

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
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tar Close error: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip Close error: %v", err)
	}
	return buf
}

// zippedFile writes a zip containing one regular-file entry at name holding
// content and returns the archive path.
func zippedFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	zipWriter := zip.NewWriter(file)
	entry, err := zipWriter.Create(name)
	if err != nil {
		t.Fatalf("Create zip entry error: %v", err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("zip Close error: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close error: %v", err)
	}
	return path
}

// TestReleaseAssetInfix_MapsOSAndArch asserts releaseAssetInfix reproduces the
// goreleaser archive naming: amd64 to x86_64, arm64 verbatim, 386 to i386, and
// title-cased OS tokens for linux, darwin, and windows.
func TestReleaseAssetInfix_MapsOSAndArch(t *testing.T) {
	// each case pairs a GOOS/GOARCH input with the expected goreleaser infix
	cases := []struct {
		goos   string
		goarch string
		want   string
	}{
		{"linux", "amd64", "Linux_x86_64"},     // amd64 maps to x86_64
		{"linux", "arm64", "Linux_arm64"},      // arm64 passes through verbatim
		{"linux", "386", "Linux_i386"},         // 386 maps to i386
		{"darwin", "amd64", "Darwin_x86_64"},   // darwin title-cases to Darwin
		{"darwin", "arm64", "Darwin_arm64"},    // darwin with arm64 passthrough
		{"windows", "amd64", "Windows_x86_64"}, // windows title-cases to Windows
		{"windows", "arm64", "Windows_arm64"},  // windows with arm64 passthrough
	}

	// assert each input produces the goreleaser infix
	for _, c := range cases {
		got := releaseAssetInfix(c.goos, c.goarch)
		if got != c.want {
			t.Errorf("releaseAssetInfix(%q, %q)=%q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

// TestReleaseAssetSuffix_SelectsArchiveExtension asserts windows archives end
// in .zip and every other OS ends in .tar.gz.
func TestReleaseAssetSuffix_SelectsArchiveExtension(t *testing.T) {
	// each case pairs a GOOS/GOARCH input with the expected archive suffix
	cases := []struct {
		goos   string
		goarch string
		want   string
	}{
		{"linux", "amd64", "_Linux_x86_64.tar.gz"},  // gzipped tar on linux
		{"darwin", "arm64", "_Darwin_arm64.tar.gz"}, // gzipped tar on darwin
		{"windows", "amd64", "_Windows_x86_64.zip"}, // zip on windows amd64
		{"windows", "arm64", "_Windows_arm64.zip"},  // zip on windows arm64
	}

	// assert each input produces the goreleaser archive suffix
	for _, c := range cases {
		got := releaseAssetSuffix(c.goos, c.goarch)
		if got != c.want {
			t.Errorf("releaseAssetSuffix(%q, %q)=%q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

// TestExtractBinary_InstallsExecutableFromVersionedDir asserts extractBinary
// strips the versioned top-level directory, writes the binary executable, and
// preserves its contents.
func TestExtractBinary_InstallsExecutableFromVersionedDir(t *testing.T) {
	// build an archive whose binary lives under a versioned top-level directory
	want := []byte("threeport-sdk binary bytes")
	archive := gzippedTar(t, "v1.2.3-dist/threeport-sdk", want)
	destDir := t.TempDir()

	// extract the binary by base name
	if err := extractBinary(archive, "threeport-sdk", destDir); err != nil {
		t.Fatalf("extractBinary error: %v", err)
	}

	// assert the binary lands at destDir/threeport-sdk
	destPath := filepath.Join(destDir, "threeport-sdk")
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}

	// assert the installed binary is executable
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%o, want 0755", info.Mode().Perm())
	}

	// assert the installed binary matches the archive contents
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("contents=%q, want %q", got, want)
	}
}

// TestExtractBinary_RejectsArchiveWithoutMatch asserts extractBinary returns an
// error when no tar entry matches the requested binary name.
func TestExtractBinary_RejectsArchiveWithoutMatch(t *testing.T) {
	// build an archive holding a different binary than the one requested
	archive := gzippedTar(t, "v1.2.3-dist/other-binary", []byte("nope"))
	destDir := t.TempDir()

	// extract the missing binary and assert an error is returned
	if err := extractBinary(archive, "threeport-sdk", destDir); err == nil {
		t.Fatalf("expected error for missing binary, got nil")
	}
}

// TestExtractBinary_LeavesNoPartialBinaryOnTruncatedArchive asserts a read that
// dies mid-entry installs nothing, so a later invocation cannot run a truncated
// executable.
func TestExtractBinary_LeavesNoPartialBinaryOnTruncatedArchive(t *testing.T) {
	// cut the archive short so the entry header parses but its contents do not
	full := gzippedTar(t, "v1.2.3-dist/threeport-sdk", bytes.Repeat([]byte("x"), 4096))
	truncated := bytes.NewReader(full.Bytes()[:full.Len()-64])
	destDir := t.TempDir()

	// extract from the truncated archive and assert it fails
	if err := extractBinary(truncated, "threeport-sdk", destDir); err == nil {
		t.Fatalf("expected error for truncated archive, got nil")
	}

	// assert no binary was installed at the destination
	if _, err := os.Stat(filepath.Join(destDir, "threeport-sdk")); !os.IsNotExist(err) {
		t.Fatalf("Stat error=%v, want a not-exist error", err)
	}

	// assert no staging file was left behind
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("destDir holds %d entries, want 0", len(entries))
	}
}

// TestExtractZip_InstallsExecutableFromVersionedDir asserts extractZip installs
// tptctl.exe executable from a versioned zip entry and preserves its contents.
func TestExtractZip_InstallsExecutableFromVersionedDir(t *testing.T) {
	// build a zip whose binary lives under a versioned top-level directory
	want := []byte("tptctl windows binary bytes")
	archivePath := zippedFile(t, "v1.2.3-dist/tptctl.exe", want)
	destDir := t.TempDir()

	// extract tptctl by its name without the .exe suffix
	if err := extractZip(archivePath, "tptctl", destDir); err != nil {
		t.Fatalf("extractZip error: %v", err)
	}

	// assert tptctl.exe is in destDir
	destPath := filepath.Join(destDir, "tptctl.exe")
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}

	// assert the installed binary is executable
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%o, want 0755", info.Mode().Perm())
	}

	// assert the installed binary matches the archive contents
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("contents=%q, want %q", got, want)
	}
}

// TestExtractZip_RejectsArchiveWithoutMatch asserts extractZip returns an
// error when the zip holds other-binary.exe rather than tptctl.exe.
func TestExtractZip_RejectsArchiveWithoutMatch(t *testing.T) {
	// build a zip that holds other-binary.exe instead of tptctl.exe
	archivePath := zippedFile(t, "v1.2.3-dist/other-binary.exe", []byte("nope"))
	destDir := t.TempDir()

	// extract tptctl and assert extractZip returns an error
	if err := extractZip(archivePath, "tptctl", destDir); err == nil {
		t.Fatalf("expected error for missing tptctl.exe, got nil")
	}
}

// TestParseChecksums_SelectsTheRequestedAsset asserts the digest returned is the
// one recorded for the requested asset and not a neighbouring line.
func TestParseChecksums_SelectsTheRequestedAsset(t *testing.T) {
	// build a checksum list holding several assets
	body := strings.Join([]string{
		"1111111111111111111111111111111111111111111111111111111111111111  threeport_Darwin_arm64.tar.gz",
		"2222222222222222222222222222222222222222222222222222222222222222  threeport_Linux_x86_64.tar.gz",
		"3333333333333333333333333333333333333333333333333333333333333333  threeport_Linux_arm64.tar.gz",
		"",
	}, "\n")

	// look up the linux amd64 archive
	got, err := parseChecksums(body, "threeport_Linux_x86_64.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksums error: %v", err)
	}

	// assert the digest belongs to the requested asset
	want := "2222222222222222222222222222222222222222222222222222222222222222"
	if got != want {
		t.Fatalf("digest=%q, want %q", got, want)
	}
}

// TestParseChecksums_RejectsMissingAsset asserts an asset absent from the list
// is an error rather than an empty digest that would compare equal to nothing.
func TestParseChecksums_RejectsMissingAsset(t *testing.T) {
	// look up an asset the list does not cover
	body := "4444444444444444444444444444444444444444444444444444444444444444  threeport_Linux_arm64.tar.gz\n"
	if _, err := parseChecksums(body, "threeport_Linux_x86_64.tar.gz"); err == nil {
		t.Fatalf("expected error for missing asset, got nil")
	}
}

// TestValidateRepo_RejectsPathTraversal asserts only an owner/name path is
// accepted, so a crafted value cannot move the request to another endpoint.
func TestValidateRepo_RejectsPathTraversal(t *testing.T) {
	// each case pairs a repository value with whether it should be accepted
	cases := []struct {
		repo string
		ok   bool
	}{
		{"threeport/threeport", true},   // the ordinary owner/name form
		{"a.b-c/d_e.f", true},           // punctuation GitHub allows
		{"../threeport", false},         // a traversal segment as the owner
		{"threeport/..", false},         // a traversal segment as the name
		{"threeport", false},            // no name segment
		{"a/b/c", false},                // an extra path segment
		{"", false},                     // empty
		{"threeport/thr eeport", false}, // a space
		{".hidden/threeport", false},    // a leading dot
		{"threeport/threeport/", false}, // a trailing separator
		{"http://x/threeport", false},   // a scheme
	}

	// assert each value is accepted or rejected as expected
	for _, c := range cases {
		err := validateRepo(c.repo)
		if c.ok && err != nil {
			t.Errorf("validateRepo(%q) error=%v, want nil", c.repo, err)
		}
		if !c.ok && err == nil {
			t.Errorf("validateRepo(%q)=nil, want an error", c.repo)
		}
	}
}

// TestValidateTag_RejectsPathTraversal asserts only a v-prefixed version tag is
// accepted, covering the prerelease and pseudo-version forms the release
// targets produce.
func TestValidateTag_RejectsPathTraversal(t *testing.T) {
	// each case pairs a tag value with whether it should be accepted
	cases := []struct {
		tag string
		ok  bool
	}{
		{"v0.7.0", true},                               // a GA tag
		{"v0.7.0-dev.19", true},                        // a dev channel tag
		{"v0.7.0-rc.1", true},                          // an rc channel tag
		{"v0.7.0-0.20260731214756-abcdef123456", true}, // a go pseudo-version
		{"../v0.7.0", false},                           // a traversal prefix
		{"v0.7.0/../../x", false},                      // an embedded traversal
		{"0.7.0", false},                               // no v prefix
		{"latest", false},                              // a floating name
		{"", false},                                    // empty
	}

	// assert each value is accepted or rejected as expected
	for _, c := range cases {
		err := validateTag(c.tag)
		if c.ok && err != nil {
			t.Errorf("validateTag(%q) error=%v, want nil", c.tag, err)
		}
		if !c.ok && err == nil {
			t.Errorf("validateTag(%q)=nil, want an error", c.tag)
		}
	}
}

// TestTokenBearingHost_LimitsTheCredentialToGithub asserts the credential is
// attached only for GitHub hosts, since the asset url arrives in a response
// body rather than being built locally.
func TestTokenBearingHost_LimitsTheCredentialToGithub(t *testing.T) {
	// each case pairs a host with whether the credential may be sent to it
	cases := []struct {
		host string
		ok   bool
	}{
		{"github.com", true},                          // the release host
		{"api.github.com", true},                      // the release api
		{"objects.githubusercontent.com", true},       // where asset downloads redirect
		{"attacker.com", false},                       // an unrelated host
		{"github.com.attacker.com", false},            // a suffix-extension lookalike
		{"githubusercontent.com.attacker.com", false}, // the same trick on the asset host
		{"notgithub.com", false},                      // a prefix-extension lookalike
		{"", false},                                   // no host
	}

	// assert each host is allowed or refused as expected
	for _, c := range cases {
		if got := tokenBearingHost(c.host); got != c.ok {
			t.Errorf("tokenBearingHost(%q)=%v, want %v", c.host, got, c.ok)
		}
	}
}

// TestGithubGet_BoundsTheRequestByTimeout asserts the timeout reaches the client
// rather than the request running against the package default client, which has
// none. An unbounded release download hangs the caller indefinitely against a
// host that accepts the connection and never answers.
func TestGithubGet_BoundsTheRequestByTimeout(t *testing.T) {
	// an already-expired deadline, which the client enforces before it dials, so
	// the assertion depends on no host being reachable from wherever this runs
	_, err := githubGet(
		"https://api.github.com/repos/example/example/releases/tags/v1.0.0",
		"", "", time.Nanosecond,
	)
	if err == nil {
		t.Fatal("githubGet returned no error, want the deadline to have been exceeded")
	}
	// assert the failure is the timeout and not some other transport error
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("githubGet error = %v, want context.DeadlineExceeded", err)
	}
}

// TestReleaseMetadataURL_KeepsTheOwnerAndNameSeparate covers the escaping the
// release lookup rests on. A repository is an owner/name pair spanning two
// path segments, and escaping it as one turns the slash into %2F, which
// addresses no repository and answers 404 on every download.
func TestReleaseMetadataURL_KeepsTheOwnerAndNameSeparate(t *testing.T) {
	got := releaseMetadataURL("randalljohnson/threeport", "v0.7.0-dev.23")

	want := "https://api.github.com/repos/randalljohnson/threeport/releases/tags/v0.7.0-dev.23"
	if got != want {
		t.Errorf("releaseMetadataURL = %q, want %q", got, want)
	}
	if strings.Contains(got, "%2F") {
		t.Errorf("releaseMetadataURL = %q, escaped the owner and name into one segment", got)
	}
}
