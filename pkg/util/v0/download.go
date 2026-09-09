package v0

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// goreleaser publishes one archive per GOOS/GOARCH, zip on Windows and
// tar.gz elsewhere, checksums.txt beside them, and the binary wrapped
// in a directory so extractors match on basename.

// maxArchiveBytes is the cap on a downloaded archive, checksums file, and
// extracted binary, 1 GiB. A hostile or corrupt payload cannot write
// without bound.
const maxArchiveBytes = 1 << 30

// checksumAssetName is the goreleaser checksums asset, one
// "<sha256>  <archive name>" line per archive.
const checksumAssetName = "checksums.txt"

// releaseMetadataTimeout bounds a request for the release JSON and the
// checksum list. A request still running after this has stalled.
const releaseMetadataTimeout = 30 * time.Second

// releaseDownloadTimeout bounds the archive transfer. It leaves room for a
// slow link while still failing a stalled transfer.
const releaseDownloadTimeout = 10 * time.Minute

// repoSegmentPattern matches one segment of a GitHub owner/name path. A
// leading alphanumeric character rejects "." and ".." so a crafted repo
// cannot move the request to another API endpoint.
var repoSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// tagPattern matches a v-prefixed semantic version, with an optional prerelease
// or build suffix.
var tagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+([-+][A-Za-z0-9.-]+)?$`)

// releaseArchSuffix maps a GOARCH value to the archive arch token, x86_64 for
// amd64 and i386 for 386.
func releaseArchSuffix(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "386":
		return "i386"
	default:
		return goarch
	}
}

// releaseAssetInfix returns the OS and architecture token used in a release
// archive name, for example Linux_x86_64.
func releaseAssetInfix(goos, goarch string) string {
	return titleCaseOS(goos) + "_" + releaseArchSuffix(goarch)
}

// releaseArchiveExt returns the archive extension for goos, zip on Windows and
// tar.gz otherwise.
func releaseArchiveExt(goos string) string {
	if goos == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

// releaseAssetSuffix returns the archive name suffix for goos and goarch.
func releaseAssetSuffix(goos, goarch string) string {
	return "_" + releaseAssetInfix(goos, goarch) + releaseArchiveExt(goos)
}

// titleCaseOS upper-cases the first letter of a GOOS value, for example linux
// to Linux.
func titleCaseOS(goos string) string {
	if goos == "" {
		return goos
	}
	return strings.ToUpper(goos[:1]) + goos[1:]
}

// githubRelease is the subset of the GitHub release API response the download
// path reads.
type githubRelease struct {
	Assets []githubReleaseAsset `json:"assets"`
}

// githubReleaseAsset is the subset of a GitHub release asset the download
// path reads.
type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// validateRepo reports an error unless repo is an owner/name path whose
// segments are safe to interpolate into a request path.
func validateRepo(repo string) error {
	owner, name, found := strings.Cut(repo, "/")
	if !found || !repoSegmentPattern.MatchString(owner) || !repoSegmentPattern.MatchString(name) {
		return fmt.Errorf("failed to validate repository %q: expected an owner/name path", repo)
	}
	return nil
}

// releaseMetadataURL returns the GitHub API URL for the tagged release.
// Owner and name escape as separate path segments so the slash is not %2F.
func releaseMetadataURL(repo, tag string) string {
	owner, name, _ := strings.Cut(repo, "/")

	return fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/releases/tags/%s",
		url.PathEscape(owner),
		url.PathEscape(name),
		url.PathEscape(tag),
	)
}

// validateTag reports an error unless tag is a v-prefixed version safe to
// interpolate into a request path.
func validateTag(tag string) error {
	if !tagPattern.MatchString(tag) {
		return fmt.Errorf("failed to validate release tag %q: expected a v-prefixed version", tag)
	}
	return nil
}

// tokenBearingHost reports whether host is a GitHub host the credential is
// sent to. The asset URL comes from an API response body, so its host is
// checked before the header is attached.
func tokenBearingHost(host string) bool {
	return host == "github.com" ||
		host == "api.github.com" ||
		host == "githubusercontent.com" ||
		strings.HasSuffix(host, ".githubusercontent.com")
}

// githubGet GETs rawURL over HTTPS, requiring a 200 response. The caller
// closes the body. token is sent as a Bearer credential only on a GitHub host.
func githubGet(rawURL, token, accept string, timeout time.Duration) (*http.Response, error) {
	// parse the request URL
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}
	// require HTTPS
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("failed to request %s: expected an https url", parsed.Redacted())
	}

	// build the GET request
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	// set the optional Accept header
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	// attach the token only on a GitHub host
	if token != "" && tokenBearingHost(parsed.Hostname()) {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// send the request; a nil Transport shares the default connection pool
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", parsed.Redacted(), err)
	}
	// require a 200 response
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("failed to fetch %s: unexpected status %s", parsed.Redacted(), resp.Status)
	}

	return resp, nil
}

// DownloadReleaseBinary downloads the named binary from the GitHub release at
// repo and tag for this OS and architecture, verifies the archive checksum,
// and installs it executable into destDir. Windows archives install as
// binaryName.exe. An empty token is accepted for a public release.
func DownloadReleaseBinary(repo, tag, binaryName, destDir, token string) error {
	// validate the repository path and release tag
	if err := validateRepo(repo); err != nil {
		return err
	}
	if err := validateTag(tag); err != nil {
		return err
	}

	// resolve the archive suffix for this OS and architecture
	assetSuffix := releaseAssetSuffix(runtime.GOOS, runtime.GOARCH)
	releaseURL := releaseMetadataURL(repo, tag)
	// fetch the GitHub release metadata
	resp, err := githubGet(releaseURL, token, "application/vnd.github+json", releaseMetadataTimeout)
	if err != nil {
		return fmt.Errorf("failed to fetch release metadata: %w", err)
	}
	defer resp.Body.Close()

	// decode the release assets
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to decode release metadata: %w", err)
	}

	// select the platform archive and the checksums asset
	var archive, checksums *githubReleaseAsset
	for i := range release.Assets {
		switch {
		case strings.HasSuffix(release.Assets[i].Name, assetSuffix):
			archive = &release.Assets[i]
		case release.Assets[i].Name == checksumAssetName:
			checksums = &release.Assets[i]
		}
	}
	if archive == nil {
		return fmt.Errorf("failed to find release asset ending in %s for %s %s", assetSuffix, repo, tag)
	}
	if checksums == nil {
		return fmt.Errorf(
			"failed to find %s for %s %s: refusing to install an unverified binary",
			checksumAssetName, repo, tag,
		)
	}

	// read the published digest for the archive
	wantDigest, err := releaseAssetDigest(checksums.BrowserDownloadURL, token, archive.Name)
	if err != nil {
		return err
	}

	// create the destination directory
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// stage the archive on disk so its digest can be checked before extraction
	archivePath, gotDigest, err := downloadToTemp(archive.BrowserDownloadURL, token, destDir)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	// verify the downloaded bytes against the published digest
	if gotDigest != wantDigest {
		return fmt.Errorf(
			"failed to verify %s: checksum %s does not match the published %s",
			archive.Name, gotDigest, wantDigest,
		)
	}

	// extract the named binary from a zip archive
	if strings.HasSuffix(archive.Name, ".zip") {
		return extractZip(archivePath, binaryName, destDir)
	}

	// extract the named binary from a tar.gz archive
	staged, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open staged archive: %w", err)
	}
	defer staged.Close()

	return extractBinary(staged, binaryName, destDir)
}

// releaseAssetDigest returns the lowercase SHA-256 published for assetName in
// the checksums file at checksumsURL.
func releaseAssetDigest(checksumsURL, token, assetName string) (string, error) {
	// fetch the checksums asset
	resp, err := githubGet(checksumsURL, token, "", releaseMetadataTimeout)
	if err != nil {
		return "", fmt.Errorf("failed to download %s: %w", checksumAssetName, err)
	}
	defer resp.Body.Close()

	// read the checksums body, bounded by maxArchiveBytes
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxArchiveBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", checksumAssetName, err)
	}

	// parse the checksums for the named asset
	return parseChecksums(string(body), assetName)
}

// parseChecksums returns the lowercase SHA-256 listed for assetName in a
// checksums file body, one hex digest and filename per line.
func parseChecksums(body, assetName string) (string, error) {
	for _, line := range strings.Split(body, "\n") {
		digest, name, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found {
			continue
		}
		if strings.TrimSpace(name) == assetName {
			return strings.ToLower(strings.TrimSpace(digest)), nil
		}
	}

	return "", fmt.Errorf("failed to find %s in %s", assetName, checksumAssetName)
}

// downloadToTemp writes the asset at assetURL to a temp file in destDir and
// returns the path and the SHA-256 of the bytes written. The caller removes
// the file.
func downloadToTemp(assetURL, token, destDir string) (string, string, error) {
	// fetch the release asset
	resp, err := githubGet(assetURL, token, "", releaseDownloadTimeout)
	if err != nil {
		return "", "", fmt.Errorf("failed to download release asset: %w", err)
	}
	defer resp.Body.Close()

	// create a temp file in destDir
	tmp, err := os.CreateTemp(destDir, ".threeport-download-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer tmp.Close()

	// write the asset and compute its sha256 digest
	digest := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, digest), io.LimitReader(resp.Body, maxArchiveBytes)); err != nil {
		os.Remove(tmp.Name())
		return "", "", fmt.Errorf("failed to write release asset: %w", err)
	}

	return tmp.Name(), hex.EncodeToString(digest.Sum(nil)), nil
}

// extractBinary installs binaryName from a gzip-compressed tar stream into
// destDir.
func extractBinary(r io.Reader, binaryName, destDir string) error {
	// open the gzip stream
	gzReader, err := gzip.NewReader(io.LimitReader(r, maxArchiveBytes))
	if err != nil {
		return fmt.Errorf("failed to open gzip reader: %w", err)
	}
	defer gzReader.Close()

	// scan tar entries for the named binary
	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		// match the binary by basename, ignoring the wrapped directory
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binaryName {
			continue
		}

		return installExtractedBinary(tarReader, binaryName, destDir)
	}

	return fmt.Errorf("failed to find %s in release archive", binaryName)
}

// extractZip installs binaryName from a zip archive into destDir, keeping a
// .exe suffix when the archive entry has one.
func extractZip(archivePath, binaryName, destDir string) error {
	// open the zip from a seekable file; the format keeps its directory at the end
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip archive: %w", err)
	}
	defer reader.Close()

	exeName := binaryName + ".exe"
	for _, file := range reader.File {
		// match the binary by basename, including a windows .exe suffix
		info := file.FileInfo()
		base := filepath.Base(file.Name)
		if !info.Mode().IsRegular() || (base != binaryName && base != exeName) {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open zip entry: %w", err)
		}
		defer rc.Close()

		return installExtractedBinary(rc, base, destDir)
	}

	return fmt.Errorf("failed to find %s in release archive", binaryName)
}

// installExtractedBinary writes r to destDir/destName as an executable. The
// write is staged and renamed so an interrupted read leaves no partial
// executable.
func installExtractedBinary(r io.Reader, destName, destDir string) error {
	// create the destination directory
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// stage the write in destDir so the later rename stays on one filesystem
	out, err := os.CreateTemp(destDir, "."+destName+"-*")
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	stagedPath := out.Name()

	// copy the binary, bounded by maxArchiveBytes
	if _, err := io.Copy(out, io.LimitReader(r, maxArchiveBytes)); err != nil {
		out.Close()
		os.Remove(stagedPath)
		return fmt.Errorf("failed to write binary: %w", err)
	}
	// close the staged file
	if err := out.Close(); err != nil {
		os.Remove(stagedPath)
		return fmt.Errorf("failed to close binary: %w", err)
	}
	// set mode 0755
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		os.Remove(stagedPath)
		return fmt.Errorf("failed to set binary mode: %w", err)
	}
	// rename over the destination, replacing an existing binary
	if err := os.Rename(stagedPath, filepath.Join(destDir, destName)); err != nil {
		os.Remove(stagedPath)
		return fmt.Errorf("failed to install binary: %w", err)
	}

	return nil
}
