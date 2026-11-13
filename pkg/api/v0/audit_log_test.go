package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestADefinitionsAuditLogCantChange covers AuditLog on the EKS, GKE and OKE
// definitions, which is set on a cluster when it is created: a patch or a
// replace that changes it is refused, while one that leaves it, or a replace
// that doesn't send it on a definition without one, is accepted.
func TestADefinitionsAuditLogCantChange(t *testing.T) {
	krdID := util.Ptr(uint(1))
	cases := map[string]func(auditLog *bool) any{
		"EKS": func(auditLog *bool) any {
			return &AwsEksKubernetesRuntimeDefinition{
				Definition: Definition{Name: util.Ptr("eks")}, ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("m5.large"),
				DefaultNodeGroupInitialSize: util.Ptr(2), DefaultNodeGroupMinimumSize: util.Ptr(0), DefaultNodeGroupMaximumSize: util.Ptr(5),
				KubernetesRuntimeDefinitionID: krdID, AuditLog: auditLog,
			}
		},
		"GKE": func(auditLog *bool) any {
			return &GcpGkeKubernetesRuntimeDefinition{
				Definition: Definition{Name: util.Ptr("gke")}, ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("e2-standard-4"),
				DefaultNodeGroupInitialSize: util.Ptr(2), DefaultNodeGroupMinimumSize: util.Ptr(0), DefaultNodeGroupMaximumSize: util.Ptr(5),
				KubernetesRuntimeDefinitionID: krdID, AuditLog: auditLog,
			}
		},
		"OKE": func(auditLog *bool) any {
			return &OciOkeKubernetesRuntimeDefinition{
				Definition: Definition{Name: util.Ptr("oke")}, WorkerNodeShape: util.Ptr("VM.Standard.E4.Flex"), WorkerNodeInitialCount: util.Ptr(int32(2)),
				KubernetesRuntimeDefinitionID: krdID, AuditLog: auditLog,
			}
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			db := setupKubernetesApiAccessDB(t)
			session := func() *gorm.DB { return db.Session(&gorm.Session{}) }

			created := build(util.Ptr(true))
			require.NoError(t, session().Create(created).Error)
			assert.ErrorContains(t, session().Model(created).Updates(build(util.Ptr(false))).Error, ErrMsgAuditLogImmutable)
			assert.NoError(t, session().Model(created).Updates(build(util.Ptr(true))).Error, "sending the value it holds")
			assert.NoError(t, session().Model(created).Updates(build(nil)).Error, "a patch without it")

			replaced := build(nil)
			require.NoError(t, session().First(replaced, 1).Error)
			require.NoError(t, session().Save(replaced).Error, "a replace that keeps it")
			switch object := replaced.(type) {
			case *AwsEksKubernetesRuntimeDefinition:
				object.AuditLog = nil
			case *GcpGkeKubernetesRuntimeDefinition:
				object.AuditLog = nil
			case *OciOkeKubernetesRuntimeDefinition:
				object.AuditLog = nil
			}
			assert.ErrorContains(t, session().Save(replaced).Error, ErrMsgAuditLogImmutable, "a replace that clears it")

			// a definition without an audit log takes a replace that doesn't send it
			require.NoError(t, session().Model(created).UpdateColumn("audit_log", false).Error)
			require.NoError(t, session().First(replaced, 1).Error)
			switch object := replaced.(type) {
			case *AwsEksKubernetesRuntimeDefinition:
				object.AuditLog = nil
			case *GcpGkeKubernetesRuntimeDefinition:
				object.AuditLog = nil
			case *OciOkeKubernetesRuntimeDefinition:
				object.AuditLog = nil
			}
			assert.NoError(t, session().Save(replaced).Error, "unset is false")
		})
	}
}
