package v0

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// TestTheRestoreClientAsksForAFreshTokenOnEveryRequest covers how a restore
// authenticates to its cluster.  Each of its phases can run for half an
// hour, longer than one Google access token lasts, so a client that kept the
// token it connected with would start failing part way, with the control
// plane scaled down and the restore lock held.  Every request carries the
// token the generator returns when it is made.
func TestTheRestoreClientAsksForAFreshTokenOnEveryRequest(t *testing.T) {
	var mu sync.Mutex
	var presented []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		presented = append(presented, r.Header.Get("Authorization"))
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			fmt.Fprint(w, `{"kind":"APIVersions","versions":["v1"]}`)
		case "/apis":
			fmt.Fprint(w, `{"kind":"APIGroupList","apiVersion":"v1","groups":[]}`)
		case "/api/v1":
			fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","namespaced":true,"kind":"ConfigMap","verbs":["get","list"]}]}`)
		default:
			fmt.Fprint(w, `{"kind":"ConfigMapList","apiVersion":"v1","metadata":{},"items":[]}`)
		}
	}))
	defer server.Close()

	minted := 0
	tokenGenerator := func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		minted++
		return fmt.Sprintf("token-%d", minted), nil
	}
	connection := &kube.KubeConnectionInfo{
		APIEndpoint:   server.URL,
		CACertificate: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})),
		Token:         "the-token-it-connected-with",
	}

	kubeClient, err := refreshingKubeClient(connection, tokenGenerator)
	require.NoError(t, err)
	configMaps := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	for i := 0; i < 2; i++ {
		_, err := kubeClient.Resource(configMaps).Namespace("threeport-control-plane").List(context.Background(), metav1.ListOptions{})
		require.NoError(t, err)
	}

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(presented), 2)
	last, beforeLast := presented[len(presented)-1], presented[len(presented)-2]
	assert.NotEqual(t, beforeLast, last, "two requests carried the same token")
	assert.Equal(t, fmt.Sprintf("Bearer token-%d", minted), last, "a request did not carry the newest token")
	assert.NotContains(t, presented, "Bearer the-token-it-connected-with", "the token the connection was made with is kept")
}
