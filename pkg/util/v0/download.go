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

// maxArchiveBytes caps how many bytes are read from a release archive, both as
// it downloads and as it decompresses, so a hostile or corrupt archive cannot
// fill the disk. The largest threeport binary is under 300 MB.
const maxArchiveBytes = 1 << 30

// checksumAssetName is the asset goreleaser publishes alongside the release
// archives, holding one "<sha256>  <archive name>" line per archive.
const checksumAssetName = "checksums.txt"

// releaseMetadataTimeout bounds a request for the small assets: the release JSON
// and the checksum list. Both are a few kilobytes, so a request still running
// after this has stalled rather than merely being slow.
const releaseMetadataTimeout = 30 * time.Second

// releaseDownloadTimeout bounds the archive transfer, which is the one request
// whose size justifies waiting. The largest published binary is under 300 MB, so
// this leaves room for a slow link while still failing a stalled transfer rather
// than hanging the caller forever.
const releaseDownloadTimeout = 10 * time.Minute

// repoSegmentPattern matches one segment of a GitHub "owner/name" path. The
// leading character must be alphanumeric, which rejects "." and ".." and keeps
// a crafted repo value from moving the request to another API endpoint.
var repoSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// tagPattern matches a release tag: a leading "v", three dot-separated numbers,
// and an optional prerelease or build segment.
var tagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+([-+][A-Za-z0-9.-]+)?$`)

// releaseArchSuffix maps a GOARCH value to the architecture token goreleaser
// embeds in archive names: amd64 to x86_64, 386 to i386, and every other
// architecture to the GOARCH string verbatim.
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

// releaseAssetInfix returns the OS and architecture token goreleaser embeds in
// archive names for goos and goarch, for example "Linux_x86_64" or
// "Darwin_arm64". The OS is title-cased and the architecture follows the
// goreleaser mapping.
func releaseAssetInfix(goos, goarch string) string {
	return titleCaseOS(goos) + "_" + releaseArchSuffix(goarch)
}

// releaseArchiveExt returns the archive extension goreleaser publishes for
// goos: zip on windows, gzipped tar everywhere else.
func releaseArchiveExt(goos string) string {
	if goos == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

// releaseAssetSuffix returns the goreleaser archive name suffix for goos and goarch.
func releaseAssetSuffix(goos, goarch string) string {
	return "_" + releaseAssetInfix(goos, goarch) + releaseArchiveExt(goos)
}

// titleCaseOS upper-cases the first letter of a GOOS value to match the
// title-cased OS token goreleaser embeds in archive names, for example linux to
// Linux.
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

// githubReleaseAsset is the subset of a GitHub release asset the download path
// reads.
type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// validateRepo returns an error unless repo is an "owner/name" path whose two
// segments are both safe to interpolate into a request path.
func validateRepo(repo string) error {
	owner, name, found := strings.Cut(repo, "/")
	if !found || !repoSegmentPattern.MatchString(owner) || !repoSegmentPattern.MatchString(name) {
		return fmt.Errorf("failed to validate repository %q: expected an owner/name path", repo)
	}
	return nil
}

// releaseMetadataURL builds the GitHub API address of one release, named by
// the owner/name repository pair and the release tag.
//
// The repository spans two path segments, so each half escapes on its own.
// Escaping the pair as a single segment turns its slash into %2F, which
// addresses a repository that does not exist and answers 404. Callers validate
// both halves and the tag against the segment alphabet before reaching here.
func releaseMetadataURL(repo, tag string) string {
	owner, name, _ := strings.Cut(repo, "/")

	return fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/releases/tags/%s",
		url.PathEscape(owner),
		url.PathEscape(name),
		url.PathEscape(tag),
	)
}

// validateTag returns an error unless tag is a version tag that is safe to
// interpolate into a request path.
func validateTag(tag string) error {
	if !tagPattern.MatchString(tag) {
		return fmt.Errorf("failed to validate release tag %q: expected a v-prefixed version", tag)
	}
	return nil
}

// tokenBearingHost reports whether host is a GitHub host the credential may be
// sent to. The asset URL arrives in an API response body rather than being
// built here, so its host is checked before the header is attached.
func tokenBearingHost(host string) bool {
	return host == "github.com" ||
		host == "api.github.com" ||
		host == "githubusercontent.com" ||
		strings.HasSuffix(host, ".githubusercontent.com")
}

// githubGet issues a GET to rawURL, attaching token as a bearer credential only
// when the host is a GitHub host, and bounding the whole exchange by timeout.
// The caller closes the returned body. A client with a zero Transport shares the
// default one, so a per-call client still reuses the connection pool.
func githubGet(rawURL, token, accept string, timeout time.Duration) (*http.Response, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("failed to request %s: expected an https url", parsed.Redacted())
	}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if token != "" && tokenBearingHost(parsed.Hostname()) {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", parsed.Redacted(), err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("failed to fetch %s: unexpected status %s", parsed.Redacted(), resp.Status)
	}

	return resp, nil
}

// DownloadReleaseBinary downloads binaryName from the tag release of repo (an
// "owner/name" path on github.com) and installs it executable into destDir.
// Windows archives install as binaryName.exe. The archive is verified against
// the release's published checksums before anything is extracted, so a release
// without them fails rather than installing an unverified binary. token may be
// empty for public repos.
func DownloadReleaseBinary(repo, tag, binaryName, destDir, token string) error {
	if err := validateRepo(repo); err != nil {
		return err
	}
	if err := validateTag(tag); err != nil {
		return err
	}

	// resolve the archive asset matching the running OS and architecture
	assetSuffix := releaseAssetSuffix(runtime.GOOS, runtime.GOARCH)
	releaseURL := releaseMetadataURL(repo, tag)
	resp, err := githubGet(releaseURL, token, "application/vnd.github+json", releaseMetadataTimeout)
	if err != nil {
		return fmt.Errorf("failed to fetch release metadata: %w", err)
	}
	defer resp.Body.Close()

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to decode release metadata: %w", err)
	}

	// select the archive by name suffix and the checksums published beside it
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

	wantDigest, err := releaseAssetDigest(checksums.BrowserDownloadURL, token, archive.Name)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// stage the archive on disk so its digest can be checked before extraction
	archivePath, gotDigest, err := downloadToTemp(archive.BrowserDownloadURL, token, destDir)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	if gotDigest != wantDigest {
		return fmt.Errorf(
			"failed to verify %s: checksum %s does not match the published %s",
			archive.Name, gotDigest, wantDigest,
		)
	}

	// extract a zip asset with the zip reader
	if strings.HasSuffix(archive.Name, ".zip") {
		return extractZip(archivePath, binaryName, destDir)
	}

	staged, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open staged archive: %w", err)
	}
	defer staged.Close()

	return extractBinary(staged, binaryName, destDir)
}

// releaseAssetDigest downloads the checksum list at checksumsURL and returns the
// SHA-256 recorded for assetName.
func releaseAssetDigest(checksumsURL, token, assetName string) (string, error) {
	resp, err := githubGet(checksumsURL, token, "", releaseMetadataTimeout)
	if err != nil {
		return "", fmt.Errorf("failed to download %s: %w", checksumAssetName, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxArchiveBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", checksumAssetName, err)
	}

	return parseChecksums(string(body), assetName)
}

// parseChecksums returns the SHA-256 recorded for assetName in a goreleaser
// checksum list, whose lines pair a hex digest with the asset it covers.
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

// downloadToTemp streams the asset at assetURL into a temporary file in destDir
// and returns that path alongside the SHA-256 of what was written. The caller
// removes the file.
func downloadToTemp(assetURL, token, destDir string) (string, string, error) {
	resp, err := githubGet(assetURL, token, "", releaseDownloadTimeout)
	if err != nil {
		return "", "", fmt.Errorf("failed to download release asset: %w", err)
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp(destDir, ".threeport-download-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer tmp.Close()

	digest := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, digest), io.LimitReader(resp.Body, maxArchiveBytes)); err != nil {
		os.Remove(tmp.Name())
		return "", "", fmt.Errorf("failed to write release asset: %w", err)
	}

	return tmp.Name(), hex.EncodeToString(digest.Sum(nil)), nil
}

// extractBinary reads a gzipped tar from r and writes binaryName (matched by
// base name) into destDir, executable. The binary is staged under a temporary
// name and renamed into place, so an interrupted read leaves no partial
// executable behind.
func extractBinary(r io.Reader, binaryName, destDir string) error {
	gzReader, err := gzip.NewReader(io.LimitReader(r, maxArchiveBytes))
	if err != nil {
		return fmt.Errorf("failed to open gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		// match the binary by base name to strip the versioned top-level directory
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binaryName {
			continue
		}

		return installExtractedBinary(tarReader, binaryName, destDir)
	}

	return fmt.Errorf("failed to find %s in release archive", binaryName)
}

// extractZip reads the zip at archivePath and writes binaryName into destDir,
// executable, keeping a .exe suffix when the archive entry has one. The binary
// is staged under a temporary name and renamed into place, so an interrupted
// read leaves no partial executable.
func extractZip(archivePath, binaryName, destDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip archive: %w", err)
	}
	defer reader.Close()

	exeName := binaryName + ".exe"
	for _, file := range reader.File {
		// match the binary by base name, including a windows .exe suffix
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

// installExtractedBinary writes r into destDir/destName, executable. The binary
// is staged under a temporary name and renamed into place, so an interrupted
// read leaves no partial executable.
func installExtractedBinary(r io.Reader, destName, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	out, err := os.CreateTemp(destDir, "."+destName+"-*")
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	stagedPath := out.Name()

	if _, err := io.Copy(out, io.LimitReader(r, maxArchiveBytes)); err != nil {
		out.Close()
		os.Remove(stagedPath)
		return fmt.Errorf("failed to write binary: %w", err)
	}
	if err := out.Close(); err != nil {
		os.Remove(stagedPath)
		return fmt.Errorf("failed to close binary: %w", err)
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		os.Remove(stagedPath)
		return fmt.Errorf("failed to set binary mode: %w", err)
	}
	if err := os.Rename(stagedPath, filepath.Join(destDir, destName)); err != nil {
		os.Remove(stagedPath)
		return fmt.Errorf("failed to install binary: %w", err)
	}

	return nil
}
