package v0

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// TestGkeScopesIncludeEmail covers the scopes a GKE token is minted with.
//
// The identity a managed cluster is told to authorize is the service account's
// email. Without the email scope a token can be resolved by the account's
// numeric id instead, and a binding naming the email would authorize nobody -
// an install that reads as correct while leaving the controllers locked out.
func TestGkeScopesIncludeEmail(t *testing.T) {
	assert.Contains(t, auth.GcpOAuthScopes, "https://www.googleapis.com/auth/userinfo.email")
	assert.Contains(t, auth.GcpOAuthScopes, "https://www.googleapis.com/auth/cloud-platform")
}

// TestGkeTokenAndSubjectShareOneDecision covers the property the two halves of
// this path rest on.
//
// The token source and the identity a cluster is told to authorize have to come
// from the same decision. Worked out separately they can disagree - a control
// plane minting tokens from ambient credentials while the cluster authorizes a
// stored service account, or the reverse - and either way the controllers
// authenticate as someone the cluster has never been told about.
func TestGkeTokenAndSubjectShareOneDecision(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "cluster.go"))
	require.NoError(t, err)
	text := string(src)

	// both entry points defer to gkeAuthSource rather than deciding themselves
	for _, caller := range []string{"buildGKETokenGenerator", "GkeAuthSubject"} {
		body := regexp.MustCompile(`func ` + caller + `\([\s\S]{0,700}?\n}`).FindString(text)
		require.NotEmpty(t, body, "%s not found", caller)
		assert.Contains(t, body, "gkeAuthSource(", "%s must not decide the identity itself", caller)
	}

	// and only that one function reads ambient credentials
	defaultTokenSource := regexp.MustCompile(`google\.DefaultTokenSource\(`)
	assert.Len(t, defaultTokenSource.FindAllString(text, -1), 1,
		"ambient credentials are read in one place, so the two cannot diverge")
}
