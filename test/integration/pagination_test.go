package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// paginationTestPrefix names the objects this test creates so its assertions
// can tell them apart from whatever else the control plane already holds.
const paginationTestPrefix = "pagination-filter-test"

// TestPaginatedListAppliesFilter covers the defect where a filtered list
// returned unfiltered rows once the result set outgrew the page limit. The
// count that decides whether to paginate applied the filter, and the page query
// that ran afterwards did not, so a caller filtering by name got every object
// back as soon as a second page was needed.
//
// The filter has to match more rows than the page limit, or the handler's
// count decides no pagination is needed and the plain filtered read answers
// instead, which passes with or without the fix. So the matching group shares
// one profile and the filter is that profile, giving five matches against a
// limit of two. A unit test cannot reach this: the page query runs AS OF SYSTEM
// TIME against a snapshot, which is CockroachDB-only syntax, and this is the
// only test that executes that query with bind parameters against a real
// CockroachDB.
func TestPaginatedListAppliesFilter(t *testing.T) {
	cli.InitConfig(nil, "")

	threeportConfig, _, err := cli.GetThreeportConfig("")
	require.Nil(t, err, "should have no error getting threeport config")
	apiClient, err := threeportConfig.GetHTTPClient(threeportConfig.CurrentControlPlane)
	require.Nil(t, err, "should have no error creating http client")
	controlPlaneConfig, err := threeportConfig.GetControlPlaneConfig(threeportConfig.CurrentControlPlane)
	require.Nil(t, err, "should not get an error looking up Threeport API endpoint")
	apiEndpoint := controlPlaneConfig.APIServer

	// arrange: a profile the matching group shares, so the filter selects
	// more rows than the page limit and the handler has to paginate
	const perGroup = 5
	const pageLimit = 2

	profile, err := client.CreateProfile(apiClient, apiEndpoint, &v0.Profile{
		Name: util.Ptr(fmt.Sprintf("%s-profile", paginationTestPrefix)),
	})
	require.Nil(t, err, "should have no error creating profile")
	defer retryDelete(t, fmt.Sprintf("profile %d", *profile.ID), func() error {
		_, err := client.DeleteProfile(apiClient, apiEndpoint, *profile.ID)
		return err
	})

	matchName := fmt.Sprintf("%s-match", paginationTestPrefix)
	otherName := fmt.Sprintf("%s-other", paginationTestPrefix)

	var created []uint
	defer func() {
		for _, id := range created {
			retryDelete(t, fmt.Sprintf("kubernetes workload definition %d", id), func() error {
				_, err := client.DeleteKubernetesWorkloadDefinition(apiClient, apiEndpoint, id)
				return err
			})
		}
	}()

	// the matching rows carry the profile, the others carry none, and the two
	// groups interleave by id so a page query that dropped the filter would
	// pick up a non-matching row rather than sorting past them
	for i := range perGroup {
		for _, name := range []string{matchName, otherName} {
			definition := v0.KubernetesWorkloadDefinition{
				Definition: v0.Definition{
					Name: util.Ptr(fmt.Sprintf("%s-%d", name, i)),
				},
				YAMLDocument: util.Ptr(paginationTestManifest),
			}
			if name == matchName {
				definition.ProfileID = profile.ID
			}
			result, err := client.CreateKubernetesWorkloadDefinition(apiClient, apiEndpoint, &definition)
			require.Nil(t, err, "should have no error creating kubernetes workload definition")
			created = append(created, *result.ID)
		}
	}

	// action: filter by the shared profile with a page limit below the number
	// of rows that match, so the handler dispatches to a pagination strategy
	results, err := client.GetKubernetesWorkloadDefinitionsByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("profileid=%d&limit=%d", *profile.ID, pageLimit),
	)
	require.Nil(t, err, "should have no error listing kubernetes workload definitions")

	// assert: every object on the page matches the filter. Before the fix the
	// page query dropped the filter and paged over the whole table
	require.NotNil(t, results)
	assert.Len(t, *results, pageLimit, "the filtered set outgrows the limit, so one page comes back full")
	for _, definition := range *results {
		require.NotNil(t, definition.ProfileID, "a paginated page must not return rows the filter excludes")
		assert.Equal(t, *profile.ID, *definition.ProfileID, "a paginated page must not return rows the filter excludes")
	}
}

// paginationTestManifest is the smallest valid workload document, since this
// test exercises the list query rather than anything the workload deploys.
const paginationTestManifest = `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: pagination-filter-test
data:
  key: value
`
