package v0

import "testing"

// TestParseThreeportDependencyAcceptsForkReplace asserts a replace directive to
// a fork yields that fork's owner/name repository and the replacement version.
func TestParseThreeportDependencyAcceptsForkReplace(t *testing.T) {
	// a module that requires upstream but replaces it with a versioned fork
	gomod := `module github.com/example/consumer

go 1.21

require github.com/threeport/threeport v0.0.0-00010101000000-000000000000

replace github.com/threeport/threeport => github.com/randalljohnson/threeport v0.0.0-dev.8b97a6a
`
	// parse prefers the replace target over the require version
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// assert the fork repository is returned as owner/name
	if !found || repo != "randalljohnson/threeport" {
		t.Fatalf("repo=%q found=%v, want randalljohnson/threeport found=true", repo, found)
	}
	// assert the replacement version is returned, not the require placeholder
	if version != "v0.0.0-dev.8b97a6a" {
		t.Fatalf("version=%q, want v0.0.0-dev.8b97a6a", version)
	}
}

// TestParseThreeportDependencyAcceptsRequireWithoutReplace asserts a bare
// require on the upstream module yields threeport/threeport at that version.
func TestParseThreeportDependencyAcceptsRequireWithoutReplace(t *testing.T) {
	// a module that requires upstream with no replace override
	gomod := `module github.com/example/consumer

go 1.21

require (
	github.com/threeport/threeport v0.7.0-dev.42
	github.com/spf13/cobra v1.8.0 // indirect
)
`
	// parse falls back to the require version when no replace is present
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// assert the upstream repository is returned
	if !found || repo != "threeport/threeport" {
		t.Fatalf("repo=%q found=%v, want threeport/threeport found=true", repo, found)
	}
	// assert the require version is returned
	if version != "v0.7.0-dev.42" {
		t.Fatalf("version=%q, want v0.7.0-dev.42", version)
	}
}

// TestParseThreeportDependencyAcceptsSingleLineRequire asserts the single-line
// require form (no grouped block) is parsed for the upstream version.
func TestParseThreeportDependencyAcceptsSingleLineRequire(t *testing.T) {
	// a module declaring the threeport require on one line
	gomod := `module github.com/example/consumer

go 1.21

require github.com/threeport/threeport v0.7.0-dev.5
`
	// parse reads the single-line require form
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// assert the upstream repository and version are returned
	if !found || repo != "threeport/threeport" || version != "v0.7.0-dev.5" {
		t.Fatalf("repo=%q version=%q found=%v, want threeport/threeport v0.7.0-dev.5 true", repo, version, found)
	}
}

// TestParseThreeportDependencyRejectsNoDependency asserts a go.mod with no
// threeport dependency reports found=false rather than guessing a repository.
func TestParseThreeportDependencyRejectsNoDependency(t *testing.T) {
	// a module that does not depend on threeport at all
	gomod := `module github.com/example/consumer

go 1.21

require github.com/spf13/cobra v1.8.0
`
	// parse finds no threeport dependency
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// assert nothing is found and no repository is invented
	if found || repo != "" || version != "" {
		t.Fatalf("repo=%q version=%q found=%v, want empty found=false", repo, version, found)
	}
}

// TestParseThreeportDependencyRejectsLocalPathReplace asserts a replace to a
// local filesystem path returns an error, since that checkout has no release
// to download.
func TestParseThreeportDependencyRejectsLocalPathReplace(t *testing.T) {
	// a module replacing threeport with a relative local checkout
	gomod := `module github.com/example/consumer

go 1.21

require github.com/threeport/threeport v0.0.0-00010101000000-000000000000

replace github.com/threeport/threeport => ../threeport
`
	// parse rejects the local replace rather than treating it as no dependency
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err == nil {
		t.Fatalf("ParseThreeportDependency repo=%q version=%q found=%v, want error", repo, version, found)
	}
	if found || repo != "" || version != "" {
		t.Fatalf("repo=%q version=%q found=%v, want empty found=false", repo, version, found)
	}
}

// TestParseThreeportDependencyReplaceWinsOverRequire asserts the replace target
// is preferred even when a non-local require precedes it in the file.
func TestParseThreeportDependencyReplaceWinsOverRequire(t *testing.T) {
	// a module with a real upstream require and a fork replace
	gomod := `module github.com/example/consumer

require github.com/threeport/threeport v0.7.0-dev.9

replace github.com/threeport/threeport => github.com/acme/threeport v0.7.0-dev.3
`
	// parse prefers the replace over the require
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// assert the replace target and its version win
	if !found || repo != "acme/threeport" || version != "v0.7.0-dev.3" {
		t.Fatalf("repo=%q version=%q found=%v, want acme/threeport v0.7.0-dev.3 true", repo, version, found)
	}
}

// TestParseThreeportDependencyAcceptsGroupedForkReplace asserts a versioned fork
// replace is found inside a grouped replace block, the form go mod edit writes
// once a go.mod carries more than one replace.
func TestParseThreeportDependencyAcceptsGroupedForkReplace(t *testing.T) {
	// a module whose replaces are grouped, with an unrelated entry ahead of the
	// threeport one
	gomod := `module github.com/example/consumer

require github.com/threeport/threeport v0.7.0-dev.9

replace (
	github.com/other/dep => github.com/acme/dep v1.2.3
	github.com/threeport/threeport => github.com/acme/threeport v0.7.0-dev.3
)
`
	// parse reads the grouped entry
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// assert the grouped replace target and its version win over the require
	if !found || repo != "acme/threeport" || version != "v0.7.0-dev.3" {
		t.Fatalf("repo=%q version=%q found=%v, want acme/threeport v0.7.0-dev.3 true", repo, version, found)
	}
}

// TestParseThreeportDependencyRejectsGroupedLocalPathReplace asserts a local-path
// replace inside a grouped block returns an error. Missing it would let the
// require version through, defeating local-path pairing silently.
func TestParseThreeportDependencyRejectsGroupedLocalPathReplace(t *testing.T) {
	// a module pairing against a local threeport checkout from a grouped block
	gomod := `module github.com/example/consumer

require github.com/threeport/threeport v0.7.0-dev.9

replace (
	github.com/other/dep => github.com/acme/dep v1.2.3
	github.com/threeport/threeport => ../threeport
)
`
	// parse rejects the local replace rather than treating it as no dependency
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err == nil {
		t.Fatalf("ParseThreeportDependency repo=%q version=%q found=%v, want error", repo, version, found)
	}
	// assert the require version does not leak through
	if found || repo != "" || version != "" {
		t.Fatalf("repo=%q version=%q found=%v, want empty found=false", repo, version, found)
	}
}

// TestParseThreeportDependencyIgnoresRequireBlockEntries asserts a grouped
// require block is not read as a replace, so an entry there cannot be mistaken
// for a replacement target.
func TestParseThreeportDependencyIgnoresRequireBlockEntries(t *testing.T) {
	// a module whose only threeport reference is a grouped require entry
	gomod := `module github.com/example/consumer

require (
	github.com/other/dep v1.2.3
	github.com/threeport/threeport v0.7.0-dev.9
)
`
	// parse falls through to the require and names the upstream repository
	repo, version, found, err := ParseThreeportDependency(gomod)
	if err != nil {
		t.Fatalf("ParseThreeportDependency error: %v", err)
	}
	// assert the upstream repository and the required version are reported
	if !found || repo != "threeport/threeport" || version != "v0.7.0-dev.9" {
		t.Fatalf("repo=%q version=%q found=%v, want threeport/threeport v0.7.0-dev.9 true", repo, version, found)
	}
}

// TestLatestMatchingTagPicksHighestNumeric asserts the highest N is chosen by
// numeric comparison so a double-digit N outranks a single-digit one.
func TestLatestMatchingTagPicksHighestNumeric(t *testing.T) {
	// tags whose lexical order disagrees with numeric order
	tags := []string{"v0.7.0-dev.2", "v0.7.0-dev.10", "v0.7.0-dev.9"}
	// pick the highest N for the base
	got, ok := LatestMatchingTag(tags, "v0.7.0-dev")
	// assert dev.10 wins over dev.9 and dev.2 by numeric N
	if !ok || got != "v0.7.0-dev.10" {
		t.Fatalf("got=%q ok=%v, want v0.7.0-dev.10 true", got, ok)
	}
}

// TestLatestMatchingTagIgnoresOtherBases asserts tags under a different base are
// ignored when selecting the highest N for the requested base.
func TestLatestMatchingTagIgnoresOtherBases(t *testing.T) {
	// a mix of bases plus a non-numeric suffix on the requested base
	tags := []string{
		"v0.6.0-dev.50",
		"v0.7.0-dev.3",
		"v0.7.0-dev.7",
		"v0.7.0-dev.beta",
	}
	// pick the highest numeric N for the requested base only
	got, ok := LatestMatchingTag(tags, "v0.7.0-dev")
	// assert the other base and the non-numeric suffix are excluded
	if !ok || got != "v0.7.0-dev.7" {
		t.Fatalf("got=%q ok=%v, want v0.7.0-dev.7 true", got, ok)
	}
}

// TestLatestMatchingTagRejectsPrefixCrossContamination asserts a base that is a
// prefix of a longer base does not match the longer base's tags.
func TestLatestMatchingTagRejectsPrefixCrossContamination(t *testing.T) {
	// release tags for the bare base alongside prerelease tags for the dev base
	tags := []string{
		"v0.7.0.1",
		"v0.7.0.2",
		"v0.7.0-dev.9",
		"v0.7.0-dev.10",
	}

	// the bare base must match only its own dotted-numeric tags
	got, ok := LatestMatchingTag(tags, "v0.7.0")
	// assert the dev tags do not bleed into the bare base selection
	if !ok || got != "v0.7.0.2" {
		t.Fatalf("got=%q ok=%v, want v0.7.0.2 true", got, ok)
	}

	// the dev base must match only its own prerelease tags
	gotDev, okDev := LatestMatchingTag(tags, "v0.7.0-dev")
	// assert the bare-base tags do not bleed into the dev selection
	if !okDev || gotDev != "v0.7.0-dev.10" {
		t.Fatalf("got=%q ok=%v, want v0.7.0-dev.10 true", gotDev, okDev)
	}
}

// TestLatestMatchingTagRejectsEmptyInput asserts an empty tag slice reports
// false rather than returning a zero-value tag as a match.
func TestLatestMatchingTagRejectsEmptyInput(t *testing.T) {
	// no tags to choose from
	got, ok := LatestMatchingTag(nil, "v0.7.0-dev")
	// assert no match is reported
	if ok || got != "" {
		t.Fatalf("got=%q ok=%v, want empty false", got, ok)
	}
}

// TestLatestMatchingTagRejectsNoMatchingBase asserts a base with no matching
// tags reports false even when other valid tags are present.
func TestLatestMatchingTagRejectsNoMatchingBase(t *testing.T) {
	// tags that all belong to a different base
	tags := []string{"v0.6.0-dev.1", "v0.6.0-dev.2"}
	// no tag matches the requested base
	got, ok := LatestMatchingTag(tags, "v0.7.0-dev")
	// assert no match is reported
	if ok || got != "" {
		t.Fatalf("got=%q ok=%v, want empty false", got, ok)
	}
}
