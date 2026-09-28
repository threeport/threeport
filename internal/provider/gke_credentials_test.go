package provider

import (
	"os"
	"regexp"
	"testing"
)

// TestGCPClientsThreadServiceAccountCredentials is a regression test for the
// bug this file has now fixed twice: a GCP client constructed here that
// relies on ambient/ADC credential resolution instead of the explicit
// ServiceAccountCredentials tied to this GcpProvider, which silently uses
// the wrong identity in a cross-project setup (see #506, PR #519).
//
// The correctness of "does credential X actually get used by client Y" can
// only be verified against real GCP infrastructure, which isn't available
// in a unit test. What is checkable here, and would have caught the two
// call sites this PR was missing before this test was added, is structural:
// every GCP client construction in this file must pass a locally-built
// "...Opts" variadic (this file's established pattern for conditionally
// including explicit credentials) or a direct credentials option/call,
// rather than being called bare. A bare call - exactly what shipped for
// GetConnection and DeleteGCPResources before review - always falls back to
// ambient/ADC resolution regardless of ServiceAccountCredentials.
func TestGCPClientsThreadServiceAccountCredentials(t *testing.T) {
	src, err := os.ReadFile("gke.go")
	if err != nil {
		t.Fatalf("failed to read gke.go: %v", err)
	}
	text := string(src)

	// each pattern captures a GCP client constructor call together with its
	// argument list (every call in this file is single-line and well under
	// 200 chars). The call must include either a locally-built "...Opts"
	// variadic spread, or a direct credentials option/constructor - this
	// list should be extended whenever a new GCP client construction is
	// added to gke.go.
	type constructorCheck struct {
		name    string
		pattern string
	}
	credentialMarker := regexp.MustCompile(`\w*Opts\.\.\.|WithCredentialsJSON|CredentialsFromJSON`)
	constructors := []constructorCheck{
		{"container.NewClusterManagerClient", `container\.NewClusterManagerClient\(ctx[^)]{0,200}?\)`},
		{"iam.NewService", `\biam\.NewService\(ctx[^)]{0,200}?\)`},
		{"gcpiam.NewService", `gcpiam\.NewService\(ctx[^)]{0,200}?\)`},
		{"cloudresourcemanager.NewService", `cloudresourcemanager\.NewService\(ctx[^)]{0,200}?\)`},
		{"google.CredentialsFromJSON", `google\.CredentialsFromJSON\(ctx[^)]{0,200}?\)`},
	}

	foundAny := false
	for _, c := range constructors {
		calls := regexp.MustCompile(c.pattern).FindAllString(text, -1)
		if len(calls) == 0 {
			t.Errorf("expected to find at least one call to %s in gke.go - update this test's constructor list if it was removed or renamed", c.name)
			continue
		}
		foundAny = true
		for _, call := range calls {
			if !credentialMarker.MatchString(call) {
				t.Errorf(
					"call to %s does not pass a credentials-aware option in its argument list: %s\n"+
						"this looks like a GCP client built without threading explicit credentials through, "+
						"which silently falls back to ambient/ADC resolution (see #506)",
					c.name, call,
				)
			}
		}
	}
	if !foundAny {
		t.Fatal("no GCP client constructor calls found at all - test patterns are likely out of date")
	}

	// google.DefaultTokenSource has no credentials parameter (the reason
	// GetConnection uses google.CredentialsFromJSON instead when explicit
	// credentials are set), so it can't be checked by inspecting its own
	// call arguments the way the constructors above are. Instead, require
	// it to appear only in a branch explicitly gated on
	// ServiceAccountCredentials being empty, so it is never reachable when
	// explicit credentials were provided.
	tokenSourceCalls := regexp.MustCompile(`google\.DefaultTokenSource\(ctx[^)]{0,200}?\)`).FindAllStringIndex(text, -1)
	if len(tokenSourceCalls) == 0 {
		t.Error("expected to find at least one call to google.DefaultTokenSource in gke.go - update this test if it was removed or renamed")
	}
	// 600 bytes comfortably covers the guard-comment-call shape used here
	// (roughly a dozen lines) while remaining far short of the distance to
	// any other function's own ServiceAccountCredentials guard, so this
	// can't be satisfied by an unrelated check elsewhere in the file.
	const emptyCredsGuardWindow = 600
	emptyCredsGuard := regexp.MustCompile(`if i\.ServiceAccountCredentials != "" \{`)
	for _, loc := range tokenSourceCalls {
		start := loc[0] - emptyCredsGuardWindow
		if start < 0 {
			start = 0
		}
		window := text[start:loc[0]]
		if !emptyCredsGuard.MatchString(window) {
			t.Errorf(
				`call to google.DefaultTokenSource at byte offset %d is not preceded within %d bytes by `+
					`an "if i.ServiceAccountCredentials != \"\" {" guard - this looks reachable even when `+
					`explicit credentials were provided, which silently falls back to ambient/ADC resolution (see #506)`,
				loc[0], emptyCredsGuardWindow,
			)
		}
	}
}
