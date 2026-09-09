package v0

import "testing"

// TestParseThreeportDependencyAcceptsForkReplace covers a versioned fork
// replace yielding that fork's owner/name and replacement version.
func TestParseThreeportDependencyAcceptsForkReplace(t *testing.T) {
	// a placeholder require plus a versioned fork replace
	gomod := `module github.com/example/consumer

go 1.21

require github.com/threeport/threeport v0.0.0-00010101000000-000000000000

replace github.com/threeport/threeport => github.com/randalljohnson/threeport v0.0.0-dev.8b97a6a
`
	// parse the go.mod
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// the replace supplies the fork owner/name
	if !found || repo != "randalljohnson/threeport" {
		t.Fatalf("repo=%q found=%v, want randalljohnson/threeport found=true", repo, found)
	}
	// the replace supplies the version, not the placeholder
	if version != "v0.0.0-dev.8b97a6a" {
		t.Fatalf("version=%q, want v0.0.0-dev.8b97a6a", version)
	}
}

// TestParseThreeportDependencyAcceptsRequireWithoutReplace covers a
// grouped require of threeport with no replace.
func TestParseThreeportDependencyAcceptsRequireWithoutReplace(t *testing.T) {
	// a grouped require with threeport and an unrelated indirect module
	gomod := `module github.com/example/consumer

go 1.21

require (
	github.com/threeport/threeport v0.7.0-dev.42
	github.com/spf13/cobra v1.8.0 // indirect
)
`
	// parse the go.mod
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// the require supplies the upstream owner/name
	if !found || repo != "threeport/threeport" {
		t.Fatalf("repo=%q found=%v, want threeport/threeport found=true", repo, found)
	}
	// the require supplies the version
	if version != "v0.7.0-dev.42" {
		t.Fatalf("version=%q, want v0.7.0-dev.42", version)
	}
}

// TestParseThreeportDependencyAcceptsSingleLineRequire covers a
// single-line require of threeport with no replace.
func TestParseThreeportDependencyAcceptsSingleLineRequire(t *testing.T) {
	// a single-line require and no replace
	gomod := `module github.com/example/consumer

go 1.21

require github.com/threeport/threeport v0.7.0-dev.5
`
	// parse the go.mod
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// the require supplies the upstream owner/name and version
	if !found || repo != "threeport/threeport" || version != "v0.7.0-dev.5" {
		t.Fatalf("repo=%q version=%q found=%v, want threeport/threeport v0.7.0-dev.5 true", repo, version, found)
	}
}

// TestParseThreeportDependencyRejectsNoDependency covers a go.mod with
// no threeport module.
func TestParseThreeportDependencyRejectsNoDependency(t *testing.T) {
	// a go.mod whose only require is an unrelated module
	gomod := `module github.com/example/consumer

go 1.21

require github.com/spf13/cobra v1.8.0
`
	// parse the go.mod
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// not found is empty, not an invented repository
	if found || repo != "" || version != "" {
		t.Fatalf("repo=%q version=%q found=%v, want empty found=false", repo, version, found)
	}
}

// TestParseThreeportDependencyRejectsLocalPathReplace covers a replace
// that points at a local filesystem path.
func TestParseThreeportDependencyRejectsLocalPathReplace(t *testing.T) {
	// a replace pointing at a relative path
	gomod := `module github.com/example/consumer

go 1.21

require github.com/threeport/threeport v0.0.0-00010101000000-000000000000

replace github.com/threeport/threeport => ../threeport
`
	// parse the go.mod
	repo, version, found, err := ParseThreeportDependency(gomod)
	// a local path replace is an error, not a missing dependency
	if err == nil {
		t.Fatalf("ParseThreeportDependency repo=%q version=%q found=%v, want error", repo, version, found)
	}
	// found stays false and the repo and version stay empty
	if found || repo != "" || version != "" {
		t.Fatalf("repo=%q version=%q found=%v, want empty found=false", repo, version, found)
	}
}

// TestParseThreeportDependencyReplaceWinsOverRequire covers a published
// require and a versioned fork replace of a different version.
func TestParseThreeportDependencyReplaceWinsOverRequire(t *testing.T) {
	// a real upstream require and a fork replace of a different version
	gomod := `module github.com/example/consumer

require github.com/threeport/threeport v0.7.0-dev.9

replace github.com/threeport/threeport => github.com/acme/threeport v0.7.0-dev.3
`
	// parse the go.mod
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// the replace wins over the require
	if !found || repo != "acme/threeport" || version != "v0.7.0-dev.3" {
		t.Fatalf("repo=%q version=%q found=%v, want acme/threeport v0.7.0-dev.3 true", repo, version, found)
	}
}

// TestParseThreeportDependencyAcceptsGroupedForkReplace covers a grouped
// replace that points threeport at a versioned fork.
func TestParseThreeportDependencyAcceptsGroupedForkReplace(t *testing.T) {
	// a grouped replace with an unrelated module listed first
	gomod := `module github.com/example/consumer

require github.com/threeport/threeport v0.7.0-dev.9

replace (
	github.com/other/dep => github.com/acme/dep v1.2.3
	github.com/threeport/threeport => github.com/acme/threeport v0.7.0-dev.3
)
`
	// parse the go.mod
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// the grouped replace wins over the require
	if !found || repo != "acme/threeport" || version != "v0.7.0-dev.3" {
		t.Fatalf("repo=%q version=%q found=%v, want acme/threeport v0.7.0-dev.3 true", repo, version, found)
	}
}

// TestParseThreeportDependencyRejectsGroupedLocalPathReplace covers a
// grouped replace that points threeport at a local filesystem path.
func TestParseThreeportDependencyRejectsGroupedLocalPathReplace(t *testing.T) {
	// a grouped replace pointing threeport at a relative path
	gomod := `module github.com/example/consumer

require github.com/threeport/threeport v0.7.0-dev.9

replace (
	github.com/other/dep => github.com/acme/dep v1.2.3
	github.com/threeport/threeport => ../threeport
)
`
	// parse the go.mod
	repo, version, found, err := ParseThreeportDependency(gomod)
	// a local path replace is an error, not a missing dependency
	if err == nil {
		t.Fatalf("ParseThreeportDependency repo=%q version=%q found=%v, want error", repo, version, found)
	}
	// the require version does not leak through
	if found || repo != "" || version != "" {
		t.Fatalf("repo=%q version=%q found=%v, want empty found=false", repo, version, found)
	}
}

// TestParseThreeportDependencyIgnoresRequireBlockEntries covers finding
// threeport among other entries in a grouped require.
func TestParseThreeportDependencyIgnoresRequireBlockEntries(t *testing.T) {
	// a grouped require as the only threeport reference
	gomod := `module github.com/example/consumer

require (
	github.com/other/dep v1.2.3
	github.com/threeport/threeport v0.7.0-dev.9
)
`
	// parse the go.mod
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// the upstream owner/name and require version are reported
	if !found || repo != "threeport/threeport" || version != "v0.7.0-dev.9" {
		t.Fatalf("repo=%q version=%q found=%v, want threeport/threeport v0.7.0-dev.9 true", repo, version, found)
	}
}

// TestLatestMatchingTagPicksHighestNumeric covers tags whose lexical
// order disagrees with numeric order.
func TestLatestMatchingTagPicksHighestNumeric(t *testing.T) {
	// tags whose lexical order disagrees with numeric order
	tags := []string{"v0.7.0-dev.2", "v0.7.0-dev.10", "v0.7.0-dev.9"}
	// pick the latest matching v0.7.0-dev
	got, ok := LatestMatchingTag(tags, "v0.7.0-dev")
	// a double-digit suffix outranks a single-digit suffix
	if !ok || got != "v0.7.0-dev.10" {
		t.Fatalf("got=%q ok=%v, want v0.7.0-dev.10 true", got, ok)
	}
}

// TestLatestMatchingTagIgnoresOtherBases covers skipping tags from
// another series and a non-numeric suffix.
func TestLatestMatchingTagIgnoresOtherBases(t *testing.T) {
	// tags from another series, a matching series, and a non-numeric suffix
	tags := []string{
		"v0.6.0-dev.50",
		"v0.7.0-dev.3",
		"v0.7.0-dev.7",
		"v0.7.0-dev.beta",
	}
	// pick the latest matching v0.7.0-dev
	got, ok := LatestMatchingTag(tags, "v0.7.0-dev")
	// only the numeric v0.7.0-dev suffixes count
	if !ok || got != "v0.7.0-dev.7" {
		t.Fatalf("got=%q ok=%v, want v0.7.0-dev.7 true", got, ok)
	}
}

// TestLatestMatchingTagRejectsPrefixCrossContamination covers v0.7.0 and
// v0.7.0-dev selections on a mixed tag list.
func TestLatestMatchingTagRejectsPrefixCrossContamination(t *testing.T) {
	// v0.7.0.N tags alongside v0.7.0-dev.N tags
	tags := []string{
		"v0.7.0.1",
		"v0.7.0.2",
		"v0.7.0-dev.9",
		"v0.7.0-dev.10",
	}

	// pick the latest matching v0.7.0
	got, ok := LatestMatchingTag(tags, "v0.7.0")
	// the longer v0.7.0-dev tags do not match
	if !ok || got != "v0.7.0.2" {
		t.Fatalf("got=%q ok=%v, want v0.7.0.2 true", got, ok)
	}

	// pick the latest matching v0.7.0-dev
	gotDev, okDev := LatestMatchingTag(tags, "v0.7.0-dev")
	// the shorter v0.7.0 tags do not match
	if !okDev || gotDev != "v0.7.0-dev.10" {
		t.Fatalf("got=%q ok=%v, want v0.7.0-dev.10 true", gotDev, okDev)
	}
}

// TestLatestMatchingTagRejectsEmptyInput covers a nil tag list.
func TestLatestMatchingTagRejectsEmptyInput(t *testing.T) {
	// pick from a nil tag list
	got, ok := LatestMatchingTag(nil, "v0.7.0-dev")
	// no match is reported
	if ok || got != "" {
		t.Fatalf("got=%q ok=%v, want empty false", got, ok)
	}
}

// TestLatestMatchingTagRejectsNoMatchingBase covers tags from a different
// series.
func TestLatestMatchingTagRejectsNoMatchingBase(t *testing.T) {
	// tags that all belong to a different series
	tags := []string{"v0.6.0-dev.1", "v0.6.0-dev.2"}
	// pick the latest matching v0.7.0-dev
	got, ok := LatestMatchingTag(tags, "v0.7.0-dev")
	// no match is reported
	if ok || got != "" {
		t.Fatalf("got=%q ok=%v, want empty false", got, ok)
	}
}
