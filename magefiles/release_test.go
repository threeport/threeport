package main

import "testing"

// TestValidateBaseAcceptsCleanVersions accepts a three-part version with or without a leading v.
func TestValidateBaseAcceptsCleanVersions(t *testing.T) {
	// three-part versions, with and without a leading v
	cases := []struct{ in, want string }{
		{"0.7.0", "0.7.0"},
		{"v0.7.0", "0.7.0"},
		{"10.20.30", "10.20.30"},
	}
	for _, c := range cases {
		// accept a three-part version
		got, err := validateBase(c.in)
		// a clean version returns no error
		if err != nil {
			t.Errorf("validateBase(%q) returned error: %v", c.in, err)
		}
		// a three-part base comes back, without a leading v
		if got != c.want {
			t.Errorf("validateBase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestValidateBaseRejectsMalformed rejects two-part, prerelease, four-part, non-numeric, and empty versions.
func TestValidateBaseRejectsMalformed(t *testing.T) {
	// two-part, prerelease, four-part, non-numeric, and empty
	for _, in := range []string{"0.7", "0.7.0-dev.1", "1.2.3.4", "x.y.z", ""} {
		// reject each as malformed
		if _, err := validateBase(in); err == nil {
			t.Errorf("validateBase(%q) accepted a malformed version", in)
		}
	}
}

// TestHighestCounterOrdersNumerically covers a two-digit counter beating a one-digit one.
func TestHighestCounterOrdersNumerically(t *testing.T) {
	// counters 1, 2, 9, and 10
	tags := []string{"v0.7.0-dev.1", "v0.7.0-dev.2", "v0.7.0-dev.9", "v0.7.0-dev.10"}
	// pick the highest counter
	got := highestCounter(tags, "v0.7.0-dev.")
	// 10 beats 9
	if got != 10 {
		t.Errorf("highestCounter = %d, want 10", got)
	}
}

// TestHighestCounterEmptyIsZero asserts an empty tag list yields zero.
func TestHighestCounterEmptyIsZero(t *testing.T) {
	// no tags listed
	if got := highestCounter(nil, "v0.7.0-dev."); got != 0 {
		t.Errorf("highestCounter(nil) = %d, want 0", got)
	}
}

// TestHighestCounterIgnoresOtherChannels covers a dev count ignoring rc tags and an rc count ignoring dev tags.
func TestHighestCounterIgnoresOtherChannels(t *testing.T) {
	// mixed dev and rc tags
	tags := []string{"v0.7.0-dev.3", "v0.7.0-rc.7", "v0.7.0-rc.8"}
	// the dev prefix counts only the dev tag
	if got := highestCounter(tags, "v0.7.0-dev."); got != 3 {
		t.Errorf("highestCounter(dev) = %d, want 3", got)
	}
	// the rc prefix counts only the rc tags
	if got := highestCounter(tags, "v0.7.0-rc."); got != 8 {
		t.Errorf("highestCounter(rc) = %d, want 8", got)
	}
}

// TestFormatVersionChannelAndGa covers a channel tag and a general-availability tag.
func TestFormatVersionChannelAndGa(t *testing.T) {
	// a dev channel tag
	if got := formatVersion("0.7.0", "dev", false, 3); got != "v0.7.0-dev.3" {
		t.Errorf("formatVersion dev = %q, want v0.7.0-dev.3", got)
	}
	// an rc channel tag
	if got := formatVersion("0.7.0", "rc", false, 1); got != "v0.7.0-rc.1" {
		t.Errorf("formatVersion rc = %q, want v0.7.0-rc.1", got)
	}
	// a general-availability tag is v plus the base
	if got := formatVersion("0.7.0", "", true, 0); got != "v0.7.0" {
		t.Errorf("formatVersion ga = %q, want v0.7.0", got)
	}
}

// TestJoinImageTagUsesRefNameOnTag covers a tag build using the ref name as the image tag.
func TestJoinImageTagUsesRefNameOnTag(t *testing.T) {
	// a tag ref, with a version and sha that must not appear
	got := joinImageTag("tag", "v0.7.0", "v0.7.0-dev", "abc1234")
	// return the ref name as the image tag
	if got != "v0.7.0" {
		t.Errorf("joinImageTag(tag) = %q, want v0.7.0", got)
	}
}

// TestJoinImageTagJoinsVersionAndSha covers a non-tag build tagging the image as version.sha.
func TestJoinImageTagJoinsVersionAndSha(t *testing.T) {
	// a branch ref
	got := joinImageTag("branch", "dev", "v0.7.0-dev", "abc1234")
	// join version and sha with a dot
	if got != "v0.7.0-dev.abc1234" {
		t.Errorf("joinImageTag(branch) = %q, want v0.7.0-dev.abc1234", got)
	}
	// an unset ref joins version and sha with a dot
	if got := joinImageTag("", "", "v0.7.0-dev", "abc1234"); got != "v0.7.0-dev.abc1234" {
		t.Errorf("joinImageTag(unset) = %q, want v0.7.0-dev.abc1234", got)
	}
}

// TestBaseFromVersionStripsPrefixAndSuffix covers reducing a version to its three-part base.
func TestBaseFromVersionStripsPrefixAndSuffix(t *testing.T) {
	// versions with a leading v, a channel suffix, or both
	cases := []struct{ in, want string }{
		{"v0.7.0-dev", "0.7.0"},
		{"v0.7.0", "0.7.0"},
		{"0.7.0-dev", "0.7.0"},
		{"v1.2.3-rc.4", "1.2.3"},
	}
	for _, c := range cases {
		// a three-part base remains
		if got := baseFromVersion(c.in); got != c.want {
			t.Errorf("baseFromVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestBaseFromVersionFeedsValidateBase covers a channel version accepted as a three-part base.
func TestBaseFromVersionFeedsValidateBase(t *testing.T) {
	// a channel version
	base := baseFromVersion("v0.7.0-dev")
	// accept the stripped value as a three-part base
	got, err := validateBase(base)
	if err != nil {
		t.Errorf("validateBase(%q) returned error: %v", base, err)
	}
	if got != "0.7.0" {
		t.Errorf("validateBase(%q) = %q, want 0.7.0", base, got)
	}
}
