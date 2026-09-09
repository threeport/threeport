package v0

import (
	"fmt"
	"strconv"
	"strings"
)

// A go.mod replace may name another module path and version, or a
// filesystem path. Only a module path and version correspond to an
// owner/name path and a versioned release.

// threeportModulePath is the module path of threeport in a go.mod
// require or replace.
const threeportModulePath = "github.com/threeport/threeport"

// ParseThreeportDependency returns the owner/name and version of the
// threeport module declared in gomod. A versioned replace wins over a
// require and names the replacement repository. A replace to a
// filesystem path returns an error. A go.mod with no threeport
// dependency returns found=false.
func ParseThreeportDependency(gomod string) (repo, version string, found bool, err error) {
	// prefer a replace of the threeport module over a require
	repo, version, kind := parseThreeportReplace(gomod)
	switch kind {
	case replaceVersioned:
		return repo, version, true, nil
	case replaceLocal:
		return "", "", false, fmt.Errorf("failed to resolve threeport release: go.mod replaces github.com/threeport/threeport with a local path; install the binary from that checkout")
	}
	// fall back to a require of the threeport module
	if version, ok := parseThreeportRequire(gomod); ok {
		return shortModulePath(threeportModulePath), version, true, nil
	}
	return "", "", false, nil
}

// threeportReplaceKind is the kind of replace found for the threeport
// module: none, a versioned module path, or a filesystem path.
type threeportReplaceKind int

const (
	replaceNone threeportReplaceKind = iota
	replaceVersioned
	replaceLocal
)

// parseThreeportReplace returns the first replace of the threeport
// module in gomod, grouped or single-line, as an owner/name and version
// or as a local filesystem path.
func parseThreeportReplace(gomod string) (repo, version string, kind threeportReplaceKind) {
	inBlock := false
	for _, line := range strings.Split(gomod, "\n") {
		// strip comments and skip blank lines
		fields := strings.Fields(stripComment(line))
		if len(fields) == 0 {
			continue
		}
		// enter a grouped replace block
		if !inBlock && fields[0] == "replace" && len(fields) == 2 && fields[1] == "(" {
			inBlock = true
			continue
		}
		if inBlock {
			// leave the grouped replace block
			if fields[0] == ")" {
				inBlock = false
				continue
			}
		} else {
			// skip lines that are not a single-line replace
			if fields[0] != "replace" {
				continue
			}
			// drop the replace keyword so the body starts at the module path
			fields = fields[1:]
		}
		// classify the replace body
		if repo, version, kind = parseReplaceBody(fields); kind != replaceNone {
			return repo, version, kind
		}
	}
	return "", "", replaceNone
}

// parseReplaceBody classifies a replace body of the form
// github.com/threeport/threeport => path [version], with the
// replace keyword already stripped.
func parseReplaceBody(fields []string) (repo, version string, kind threeportReplaceKind) {
	if len(fields) < 3 || fields[1] != "=>" || fields[0] != threeportModulePath {
		return "", "", replaceNone
	}
	newPath := fields[2]
	// a filesystem path has no published release
	if isLocalPath(newPath) {
		return "", "", replaceLocal
	}
	// require a version on a module replacement
	if len(fields) < 4 {
		return "", "", replaceNone
	}
	return shortModulePath(newPath), fields[3], replaceVersioned
}

// parseThreeportRequire returns the version of a threeport require in
// gomod, grouped or single-line.
func parseThreeportRequire(gomod string) (version string, ok bool) {
	inBlock := false
	for _, line := range strings.Split(gomod, "\n") {
		// strip comments and skip blank lines
		fields := strings.Fields(stripComment(line))
		if len(fields) == 0 {
			continue
		}
		// enter a grouped require block
		if !inBlock && fields[0] == "require" && len(fields) == 2 && fields[1] == "(" {
			inBlock = true
			continue
		}
		if inBlock {
			// leave the grouped require block
			if fields[0] == ")" {
				inBlock = false
				continue
			}
			// return a threeport require inside the block
			if len(fields) >= 2 && fields[0] == threeportModulePath {
				return fields[1], true
			}
			continue
		}
		// return a single-line threeport require
		if fields[0] == "require" && len(fields) >= 3 && fields[1] == threeportModulePath {
			return fields[2], true
		}
	}
	return "", false
}

// stripComment returns line with a trailing // comment removed, so a
// go.mod // indirect marker does not become a field.
func stripComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return line[:i]
	}
	return line
}

// isLocalPath reports whether path is a filesystem path in a go.mod
// replace: relative, rooted, or a Windows drive letter.
func isLocalPath(path string) bool {
	if path == "" {
		return false
	}
	// relative or rooted path
	if strings.HasPrefix(path, ".") || strings.HasPrefix(path, "/") {
		return true
	}
	if strings.HasPrefix(path, "..") {
		return true
	}
	// windows drive path such as C: has a colon in its second byte
	if len(path) >= 2 && path[1] == ':' {
		return true
	}
	return false
}

// shortModulePath returns the owner/name of a module path, dropping a
// leading hostname such as github.com and any deeper path segments.
func shortModulePath(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	// drop the host segment when it contains a dot
	if len(parts) >= 1 && strings.Contains(parts[0], ".") {
		parts = parts[1:]
	}
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return strings.Join(parts, "/")
}

// LatestMatchingTag returns the tag of the form base.N with the highest
// integer N, so .10 sorts above .9. The suffix after base. must be an
// integer, so v0.7.0 never matches a v0.7.0-dev.N tag.
func LatestMatchingTag(tags []string, base string) (string, bool) {
	prefix := base + "."
	highest := -1
	var best string
	for _, tag := range tags {
		// skip tags that are not base.N
		suffix := strings.TrimPrefix(tag, prefix)
		if suffix == tag {
			continue
		}
		// skip a non-integer suffix
		n, err := strconv.Atoi(suffix)
		if err != nil {
			continue
		}
		// keep the tag when N is higher
		if n > highest {
			highest = n
			best = tag
		}
	}
	if highest < 0 {
		return "", false
	}
	return best, true
}
