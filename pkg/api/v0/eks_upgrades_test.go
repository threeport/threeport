package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAnEksSupportTypeIsChecked covers #796: the support types an EKS
// definition can ask for, and its default.
func TestAnEksSupportTypeIsChecked(t *testing.T) {
	require.Contains(t, EksSupportTypes, DefaultEksSupportType)
	assert.NoError(t, ValidateEksSupportType(nil), "unset")
	for _, supportType := range []string{"STANDARD", "EXTENDED"} {
		assert.NoError(t, ValidateEksSupportType(util.Ptr(supportType)), supportType)
	}
	for _, supportType := range []string{"standard", "Extended", "", "REGULAR", "STANDARD "} {
		assert.EqualError(t, ValidateEksSupportType(util.Ptr(supportType)),
			"SupportType \""+supportType+"\" is not supported: use one of STANDARD, EXTENDED", "%q", supportType)
	}

	assert.Equal(t, "STANDARD", EksSupportTypeOrDefault(nil))
	assert.Equal(t, "STANDARD", EksSupportTypeOrDefault(util.Ptr("")))
	assert.Equal(t, "EXTENDED", EksSupportTypeOrDefault(util.Ptr("EXTENDED")))
}

// TestAnEksDefinitionsSupportTypeIsKept covers the API: an EKS definition's
// support type is checked on create, and a change to it is refused on
// either update shape.
func TestAnEksDefinitionsSupportTypeIsKept(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	runtimeDefinitions := uint(0)
	build := func(name string, supportType *string) *AwsEksKubernetesRuntimeDefinition {
		runtimeDefinitions++
		return &AwsEksKubernetesRuntimeDefinition{
			Definition: Definition{Name: util.Ptr(name)}, ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("t3.medium"),
			DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
			KubernetesRuntimeDefinitionID: util.Ptr(runtimeDefinitions),
			SupportType:                   supportType,
		}
	}

	assert.ErrorContains(t, db.Create(build("refused", util.Ptr("LONG"))).Error,
		"SupportType \"LONG\" is not supported: use one of STANDARD, EXTENDED", "a create")

	created := build("eks", util.Ptr("EXTENDED"))
	require.NoError(t, db.Create(created).Error)
	unset := build("eks-unset", nil)
	require.NoError(t, db.Create(unset).Error, "unset is the default")
	load := func(id uint) *AwsEksKubernetesRuntimeDefinition {
		var loaded AwsEksKubernetesRuntimeDefinition
		require.NoError(t, db.First(&loaded, id).Error)
		return &loaded
	}

	for _, supportType := range []*string{util.Ptr("STANDARD"), nil} {
		replaced := load(*created.ID)
		replaced.SupportType = supportType
		assert.ErrorContains(t, db.Session(&gorm.Session{}).Save(replaced).Error,
			ErrMsgEksSupportTypeImmutable, "a replace with %v", supportType)
	}
	assert.ErrorContains(t, db.Session(&gorm.Session{}).Model(load(*created.ID)).Updates(
		&AwsEksKubernetesRuntimeDefinition{SupportType: util.Ptr("STANDARD")},
	).Error, ErrMsgEksSupportTypeImmutable, "a patch")
	assert.ErrorContains(t, db.Session(&gorm.Session{}).Model(load(*unset.ID)).Updates(
		&AwsEksKubernetesRuntimeDefinition{SupportType: util.Ptr("EXTENDED")},
	).Error, ErrMsgEksSupportTypeImmutable, "setting one it was created without")

	require.NoError(t, db.Session(&gorm.Session{}).Model(load(*created.ID)).Updates(
		&AwsEksKubernetesRuntimeDefinition{SupportType: util.Ptr("EXTENDED")},
	).Error, "sending the one it holds is not a change")
	require.NoError(t, db.Session(&gorm.Session{}).Save(load(*created.ID)).Error, "nor is a replace that keeps it")
	require.NoError(t, db.Session(&gorm.Session{}).Model(load(*created.ID)).Updates(
		&AwsEksKubernetesRuntimeDefinition{DefaultNodeGroupMaximumSize: util.Ptr(5)},
	).Error, "another field")

	assert.Equal(t, util.Ptr("EXTENDED"), load(*created.ID).SupportType)
	assert.Nil(t, load(*unset.ID).SupportType)
}
