package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// definitionCreateService is a resource a refused definition declares.
const definitionCreateService = `apiVersion: v1
kind: Service
metadata:
  name: refused
spec:
  ports:
  - port: 80
`

// TestAWorkloadDefinitionAnUpdateWouldRefuseIsRefusedOnCreate covers a
// kubernetes workload definition created with a YAML document the update
// path refuses: its controller refuses it too, with a DefinitionCreateError
// warning on the definition, and writes no resource definitions for it.
func TestAWorkloadDefinitionAnUpdateWouldRefuseIsRefusedOnCreate(t *testing.T) {
	apiClient, endpoint, _ := threeportClient(t)

	for _, testCase := range []struct {
		name string
		yaml string
		note string
	}{
		{
			name: "duplicate-resource",
			yaml: definitionCreateService + "---\n" + definitionCreateService,
			note: "declares /Service/refused more than once",
		},
		{
			name: "no-resources",
			yaml: "# nothing to deploy\n",
			note: "declares no resources",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			created, err := client.CreateKubernetesWorkloadDefinition(apiClient, endpoint, &v0.KubernetesWorkloadDefinition{
				Definition:   v0.Definition{Name: util.Ptr(fmt.Sprintf("refused-%s-%d", testCase.name, time.Now().Unix()))},
				YAMLDocument: util.Ptr(testCase.yaml),
			})
			require.NoError(t, err)
			defer retryDelete(t, "kubernetes workload definition "+*created.Name, func() error {
				_, err := client.DeleteKubernetesWorkloadDefinition(apiClient, endpoint, *created.ID)
				return err
			})

			var refusal *v0.Event
			require.NoError(t, util.Retry(reconcileAttempts, 2, func() error {
				events, err := client.GetEventsFilteredByQueryString(
					apiClient,
					endpoint,
					fmt.Sprintf(
						"objectid=%d&objecttypename=KubernetesWorkloadDefinition&objectnamespace=threeport.io&objectversion=v0",
						*created.ID,
					),
					0,
				)
				if err != nil {
					return err
				}
				for _, event := range *events {
					if util.DerefString(event.Reason) == "DefinitionCreateError" {
						found := event
						refusal = &found
						return nil
					}
				}
				return fmt.Errorf("no DefinitionCreateError event on definition %d yet", *created.ID)
			}), "the definition is refused")
			assert.Equal(t, "Warning", util.DerefString(refusal.Type))
			assert.Contains(t, util.DerefString(refusal.Note), testCase.note)

			resourceDefinitions, err := client.GetKubernetesWorkloadResourceDefinitionsByID(apiClient, endpoint, *created.ID)
			require.NoError(t, err)
			assert.Empty(t, *resourceDefinitions, "no resource definitions written")

			definition, err := client.GetKubernetesWorkloadDefinitionByID(apiClient, endpoint, *created.ID)
			require.NoError(t, err)
			assert.False(t, util.Deref(definition.Reconciled), "not reconciled")
		})
	}
}
