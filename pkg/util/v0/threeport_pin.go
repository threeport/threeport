package v0

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// go list reports the original threeport path even when replaced;
// Replace holds the actual source. A local path replace has no
// published registry, and ghcr requires a lowercase owner.

// ResolveThreeportPin returns the owner/name, ghcr namespace, and version
// of the pinned threeport module. A replace supplies the path and version.
func ResolveThreeportPin() (repo, namespace, ver string, err error) {
	// list the threeport module
	out, err := exec.Command("go", "list", "-m", "-json", "github.com/threeport/threeport").Output()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to list threeport module: %w", err)
	}

	var mod struct {
		Path    string
		Version string
		Replace *struct {
			Path    string
			Version string
		}
	}
	// parse the module json
	if err := json.Unmarshal(out, &mod); err != nil {
		return "", "", "", fmt.Errorf("failed to parse module json: %w", err)
	}

	// prefer the replace path and version
	path, version := mod.Path, mod.Version
	if mod.Replace != nil {
		path, version = mod.Replace.Path, mod.Replace.Version
	}

	return imageCoords(path, version)
}

// imageCoords maps a github.com module path and version to owner/name,
// ghcr.io plus the lowercased owner, and version.
func imageCoords(modPath, version string) (repo, registry, ver string, err error) {
	const host = "github.com/"
	if !strings.HasPrefix(modPath, host) {
		// a local path replace has no published registry
		if strings.HasPrefix(modPath, "/") || strings.HasPrefix(modPath, ".") {
			return "", "", "", fmt.Errorf("threeport is replaced with the local path %q; no published registry to resolve, expected for local dev where CI uses the committed github pin", modPath)
		}
		return "", "", "", fmt.Errorf("module path %q is not a github.com path", modPath)
	}

	// take owner/name after github.com/
	repo = strings.TrimPrefix(modPath, host)
	owner, _, ok := strings.Cut(repo, "/")
	if !ok || owner == "" {
		return "", "", "", fmt.Errorf("module path %q has no owner/name", modPath)
	}

	// build the ghcr namespace from the owner
	registry = "ghcr.io/" + strings.ToLower(owner)
	return repo, registry, version, nil
}
