package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestUpgradeRollTargetMatrix checks which control plane an upgrade rolls.
func TestUpgradeRollTargetMatrix(t *testing.T) {
	genesisID := uint(1)
	primaryID := uint(2)
	childID := uint(3)
	genesis := &api_v0.ControlPlaneInstance{
		Common:   api_v0.Common{ID: &genesisID},
		Instance: api_v0.Instance{Name: util.Ptr("genesis")},
		IsSelf:   util.Ptr(true),
	}
	primary := &api_v0.ControlPlaneInstance{
		Common:                       api_v0.Common{ID: &primaryID},
		Instance:                     api_v0.Instance{Name: util.Ptr("primary")},
		IsSelf:                       util.Ptr(false),
		ParentControlPlaneInstanceID: &genesisID,
	}
	child := &api_v0.ControlPlaneInstance{
		Common:                       api_v0.Common{ID: &childID},
		Instance:                     api_v0.Instance{Name: util.Ptr("child")},
		IsSelf:                       util.Ptr(false),
		ParentControlPlaneInstanceID: &primaryID,
	}
	byName := func(name string) (*api_v0.ControlPlaneInstance, error) {
		switch name {
		case "primary":
			return primary, nil
		case "child":
			return child, nil
		default:
			return nil, errors.New("not found")
		}
	}
	byID := func(id uint) (*api_v0.ControlPlaneInstance, error) {
		switch id {
		case genesisID:
			return genesis, nil
		case primaryID:
			return primary, nil
		default:
			return nil, errors.New("not found")
		}
	}

	// each row is the connection the command already resolved, plus --name
	rows := []struct {
		name    string
		self    *api_v0.ControlPlaneInstance
		roll    string
		want    string
		refused bool
	}{
		{name: "row 1 current genesis", self: genesis, want: "genesis"},
		{name: "row 2 flag genesis", self: genesis, want: "genesis"},
		{name: "row 3 current primary", self: primarySelf(primaryID, genesisID), refused: true},
		{name: "row 4 flag primary", self: primarySelf(primaryID, genesisID), refused: true},
		{name: "row 5 flag child", self: childSelf(childID, primaryID), refused: true},
		{name: "row 6 flag genesis no name", self: genesis, want: "genesis"},
		{name: "row 7 genesis name primary", self: genesis, roll: "primary", want: "primary"},
		{name: "row 7 genesis name child", self: genesis, roll: "child", want: "child"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			got, err := upgradeRollTarget(row.self, row.roll, byName, byID)
			if row.refused {
				require.ErrorIs(t, err, api_v0.ErrRequestMustBeMadeToParent)
				return
			}
			require.NoError(t, err)
			require.Equal(t, row.want, util.DerefString(got.Name))
		})
	}

	// a name that is not a descendant is refused
	_, err := upgradeRollTarget(genesis, "other", byName, byID)
	require.Error(t, err)
}

// childSelf is a child API's own row, so a direct connection is refused.
func childSelf(id, parent uint) *api_v0.ControlPlaneInstance {
	return &api_v0.ControlPlaneInstance{
		Common:                       api_v0.Common{ID: &id},
		Instance:                     api_v0.Instance{Name: util.Ptr("child")},
		IsSelf:                       util.Ptr(true),
		ParentControlPlaneInstanceID: &parent,
	}
}

func primarySelf(id, parent uint) *api_v0.ControlPlaneInstance {
	return &api_v0.ControlPlaneInstance{
		Common:                       api_v0.Common{ID: &id},
		Instance:                     api_v0.Instance{Name: util.Ptr("primary")},
		IsSelf:                       util.Ptr(true),
		ParentControlPlaneInstanceID: &parent,
	}
}
