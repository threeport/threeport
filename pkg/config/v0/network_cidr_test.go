package v0

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAConfigReplaceKeepsARuntimesNetworkRange covers a runtime instance
// config replace and NetworkCidr, which is set once: a config that leaves it
// out keeps the recorded range, which the API would refuse to clear, and one
// that names it sends it.
func TestAConfigReplaceKeepsARuntimesNetworkRange(t *testing.T) {
	for name, test := range map[string]struct {
		stored, config, want *string
	}{
		"left out of the config":          {stored: util.Ptr("10.16.0.0/16"), want: util.Ptr("10.16.0.0/16")},
		"named for a runtime without one": {config: util.Ptr("10.17.0.0/16"), want: util.Ptr("10.17.0.0/16")},
		"named as recorded":               {stored: util.Ptr("10.16.0.0/16"), config: util.Ptr("10.16.0.0/16"), want: util.Ptr("10.16.0.0/16")},
	} {
		t.Run(name, func(t *testing.T) {
			runtimeName := "runtime"
			api := &fakeApi{t: t, objects: map[string]interface{}{
				api_v0.PathKubernetesRuntimeInstances: api_v0.KubernetesRuntimeInstance{
					Common:                        api_v0.Common{ID: util.Ptr(uint(1))},
					Instance:                      api_v0.Instance{Name: &runtimeName},
					KubernetesRuntimeDefinitionID: util.Ptr(uint(2)),
					NetworkCidr:                   test.stored,
				},
				api_v0.PathKubernetesRuntimeDefinitions: api_v0.KubernetesRuntimeDefinition{Common: api_v0.Common{ID: util.Ptr(uint(2))}},
			}}
			config := &KubernetesRuntimeInstanceConfig{KubernetesRuntimeInstance: KubernetesRuntimeInstanceValues{
				Name:                        &runtimeName,
				Location:                    util.Ptr("us-east-1"),
				NetworkCidr:                 test.config,
				KubernetesRuntimeDefinition: &KubernetesRuntimeDefinitionValues{Name: util.Ptr("definition")},
			}}
			_, err := config.Replace(&http.Client{}, api.serve(), runtimeName)
			require.NoError(t, err)

			var sent api_v0.KubernetesRuntimeInstance
			api.decodeSent(&sent)
			assert.Equal(t, test.want, sent.NetworkCidr)
		})
	}
}

// TestAControlPlaneConfigSendsItsNetworkCidrPool covers NetworkCidrPool in a
// control plane instance config: a create and a replace send it as the JSON
// array the API stores, and a replace that leaves it out clears it, like
// every other field the config sets.
func TestAControlPlaneConfigSendsItsNetworkCidrPool(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			raw, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			bodies = append(bodies, string(raw))
		}
		status := http.StatusOK
		if r.Method == http.MethodPost {
			status = http.StatusCreated
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"Status":{"code":` + strconv.Itoa(status) + `},"Data":[{"ID":1,"Name":"child","NetworkCidrPool":["10.32.0.0/12"]}]}`))
	}))
	defer server.Close()
	addr := strings.TrimPrefix(server.URL, "http://")

	withPool := instanceConfig("none")
	withPool.ControlPlaneInstance.NetworkCidrPool = &[]string{"10.32.0.0/12", "192.168.0.0/16"}
	created, err := withPool.Create(http.DefaultClient, addr)
	require.NoError(t, err)
	assert.Equal(t, &[]string{"10.32.0.0/12"}, created.ControlPlaneInstance.NetworkCidrPool, "read back from the API")

	_, err = withPool.Replace(http.DefaultClient, addr, "child")
	require.NoError(t, err)
	without := instanceConfig("none")
	_, err = without.Replace(http.DefaultClient, addr, "child")
	require.NoError(t, err)

	require.Len(t, bodies, 3)
	assert.Contains(t, bodies[0], `"NetworkCidrPool":["10.32.0.0/12","192.168.0.0/16"]`)
	assert.Contains(t, bodies[1], `"NetworkCidrPool":["10.32.0.0/12","192.168.0.0/16"]`)
	assert.NotContains(t, bodies[2], "10.32.0.0/12")
}

// TestAReplaceReleasesOnlyTheReservationsItNames covers ReservedNetworkCidrs
// in a control plane instance config: a replace that leaves it out keeps the
// stored reservations, one that names blocks keeps only those, and an empty
// list releases them all.
func TestAReplaceReleasesOnlyTheReservationsItNames(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			raw, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			bodies = append(bodies, string(raw))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Status":{"code":200},"Data":[{"ID":1,"Name":"self","ReservedNetworkCidrs":["10.32.0.0/12","10.64.0.0/12"]}]}`))
	}))
	defer server.Close()
	addr := strings.TrimPrefix(server.URL, "http://")

	// get shows them, so a config written from it keeps them
	got, err := (&ControlPlaneInstanceConfig{ControlPlaneInstance: ControlPlaneInstanceValues{Name: util.Ptr("self")}}).Get(http.DefaultClient, addr)
	require.NoError(t, err)
	require.Len(t, *got, 1)
	assert.Equal(t, &[]string{"10.32.0.0/12", "10.64.0.0/12"}, (*got)[0].ControlPlaneInstance.ReservedNetworkCidrs)

	without := instanceConfig("none")
	replaced, err := without.Replace(http.DefaultClient, addr, "self")
	require.NoError(t, err)
	assert.Equal(t, &[]string{"10.32.0.0/12", "10.64.0.0/12"}, replaced.ControlPlaneInstance.ReservedNetworkCidrs, "read back from the API")
	one := instanceConfig("none")
	one.ControlPlaneInstance.ReservedNetworkCidrs = &[]string{"10.64.0.0/12"}
	_, err = one.Replace(http.DefaultClient, addr, "self")
	require.NoError(t, err)
	none := instanceConfig("none")
	none.ControlPlaneInstance.ReservedNetworkCidrs = &[]string{}
	_, err = none.Replace(http.DefaultClient, addr, "self")
	require.NoError(t, err)

	require.Len(t, bodies, 3)
	assert.Contains(t, bodies[0], `"ReservedNetworkCidrs":["10.32.0.0/12","10.64.0.0/12"]`, "kept")
	assert.Contains(t, bodies[1], `"ReservedNetworkCidrs":["10.64.0.0/12"]`)
	assert.Contains(t, bodies[2], `"ReservedNetworkCidrs":[]`)
}

// TestNetworkCidrPoolJSONRoundTrips covers the conversion the config uses.
func TestNetworkCidrPoolJSONRoundTrips(t *testing.T) {
	pool := &[]string{"10.32.0.0/12"}
	assert.Equal(t, datatypes.JSON(`["10.32.0.0/12"]`), *cidrsToJSON(pool))
	assert.Equal(t, pool, cidrsFromJSON(cidrsToJSON(pool)))
	assert.Nil(t, cidrsToJSON(nil))
}
