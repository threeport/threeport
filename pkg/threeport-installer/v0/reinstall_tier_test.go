package v0

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

// TestReinstallKeepsTheInstalledTier covers a reinstall of a production
// control plane.  tptctl reinstall is not given a tier, so one taken from the
// installer alone would bring the API server and controllers back with a
// single replica and remove their disruption budgets.
//
// The fake client cannot apply the database and message broker manifests, so
// the reinstall is stopped at its first delete, after it has read the tier,
// and the components are then installed with the same installer, as the
// reinstall goes on to do.
func TestReinstallKeepsTheInstalledTier(t *testing.T) {
	for tier, wantReplicas := range map[string]int64{
		ControlPlaneTierProd: productionComponentReplicas,
		ControlPlaneTierDev:  1,
	} {
		t.Run(tier, func(t *testing.T) {
			kubeClient := testKubeClient(testNamespace(ControlPlaneNamespace, tier))
			mapper := testComponentMapper()

			// stop the reinstall at its first delete
			errStopped := errors.New("reinstall stopped by the test")
			kubeClient.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, errStopped
			})

			// the installer a reinstall builds: no tier given
			cpi := testInstaller()
			require.ErrorIs(t, cpi.Reinstall(kubeClient, &mapper, nil), errStopped)
			assert.Equal(t, ControlPlaneTier(tier), cpi.Opts.Tier)

			for _, name := range componentsWithBudgets {
				componentClient := testKubeClient()
				installComponent(t, cpi, componentClient, name)

				assert.Equal(t, wantReplicas, replicas(t, getDeployment(t, componentClient, ControlPlaneNamespace, name)), name)

				_, err := componentClient.Resource(pdbGVR).Namespace(ControlPlaneNamespace).Get(context.Background(), name, metav1.GetOptions{})
				if wantReplicas > 1 {
					assert.NoError(t, err, "%s has no disruption budget after the reinstall", name)
				} else {
					assert.True(t, k8serrors.IsNotFound(err), "%s has a disruption budget with a single replica", name)
				}
			}
		})
	}
}

// TestReinstallOfAnUnrecordedTierKeepsTheGivenOne covers a namespace from
// before the tier was recorded: the reinstall goes ahead with the tier it was
// given rather than failing.
func TestReinstallOfAnUnrecordedTierKeepsTheGivenOne(t *testing.T) {
	kubeClient := testKubeClient(testNamespace(ControlPlaneNamespace, ""))
	mapper := testComponentMapper()

	cpi := testInstaller()
	require.NoError(t, cpi.useInstalledTier(kubeClient, &mapper))
	assert.Equal(t, ControlPlaneTier(""), cpi.Opts.Tier)
}
