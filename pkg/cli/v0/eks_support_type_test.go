package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestTptctlUpRecordsTheSupportTypeItCreatedTheClusterWith covers #796 for
// the EKS definition tptctl up registers: the support type it created the
// cluster with, and none for a cluster it didn't create or whose support
// type EKS chose, which the API would refuse as empty.
func TestTptctlUpRecordsTheSupportTypeItCreatedTheClusterWith(t *testing.T) {
	record := func(supportType string, controlPlaneOnly bool) *string {
		definition := v0.AwsEksKubernetesRuntimeDefinition{}
		recordEksSupportType(&definition, supportType, controlPlaneOnly)
		return definition.SupportType
	}

	assert.Equal(t, util.Ptr("STANDARD"), record((&provider.KubernetesRuntimeInfraEKS{}).CreatedSupportType(), false), "the default")
	assert.Equal(t, util.Ptr("EXTENDED"), record((&provider.KubernetesRuntimeInfraEKS{SupportType: "EXTENDED"}).CreatedSupportType(), false))
	assert.Nil(t, record("EXTENDED", true), "with --control-plane-only")
	assert.Nil(t, record("", false), "a cluster created before Threeport set it")
}
