package v0

import (
	"fmt"
	"regexp"
	"slices"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GkeBootDiskTypes are the boot disk types a GKE definition's nodes can have.
var GkeBootDiskTypes = []string{"pd-standard", "pd-balanced", "pd-ssd"}

// GkeKmsKeyBootDiskTypes are the boot disk types GKE encrypts with a Cloud
// KMS key of the caller's.
var GkeKmsKeyBootDiskTypes = []string{"pd-standard", "pd-ssd"}

// GkeDefaultBootDiskType is the boot disk type a new node pool's nodes get
// when a GKE definition gives none.
const GkeDefaultBootDiskType = "pd-balanced"

// gkeMinBootDiskSizeGb and gkeMaxBootDiskSizeGb are the smallest and largest
// boot disks a GKE node can have.
const (
	gkeMinBootDiskSizeGb = 10
	gkeMaxBootDiskSizeGb = 65536
)

// kmsKeyNamePattern matches a Cloud KMS key's full resource name.
var kmsKeyNamePattern = regexp.MustCompile(`^projects/[^/]+/locations/[^/]+/keyRings/[^/]+/cryptoKeys/[^/]+$`)

// ErrMsgSecretsEncryptionKeyCleared is the error an update that clears a GKE
// definition's SecretsEncryptionKey is refused with.
const ErrMsgSecretsEncryptionKeyCleared = "SecretsEncryptionKey can't be cleared once it is set: GKE keeps encrypting " +
	"the clusters' Secrets with a key of yours once it has. Set it to another key to rotate it"

// gkeNodeSecurity is the part of a GKE definition that sets how its clusters
// encrypt and boot.
type gkeNodeSecurity struct {
	secretsEncryptionKey *string
	bootDiskType         *string
	bootDiskSizeGb       *int
	bootDiskKmsKey       *string
	secureBoot           *bool
	intranodeVisibility  *bool
	deletionProtection   *bool
	id                   *uint
}

// gkeNodeSecurityOf reads a GKE definition's encryption and boot settings.
func gkeNodeSecurityOf(value any) (gkeNodeSecurity, bool) {
	definition, ok := value.(*GcpGkeKubernetesRuntimeDefinition)
	if !ok {
		return gkeNodeSecurity{}, false
	}

	return gkeNodeSecurity{
		secretsEncryptionKey: definition.SecretsEncryptionKey,
		bootDiskType:         definition.BootDiskType,
		bootDiskSizeGb:       definition.BootDiskSizeGb,
		bootDiskKmsKey:       definition.BootDiskKmsKey,
		secureBoot:           definition.SecureBoot,
		intranodeVisibility:  definition.IntranodeVisibility,
		deletionProtection:   definition.DeletionProtection,
		id:                   definition.ID,
	}, true
}

// validateGkeNodeSecurity checks a GKE definition's encryption and boot
// settings: keys that are Cloud KMS key names, a boot disk type and size GKE
// gives nodes, and a boot disk key only on a disk type GKE encrypts with one.
// A boot disk type that isn't set is the one a new node pool gets.
func validateGkeNodeSecurity(settings gkeNodeSecurity) error {
	for field, key := range map[string]*string{
		"SecretsEncryptionKey": settings.secretsEncryptionKey,
		"BootDiskKmsKey":       settings.bootDiskKmsKey,
	} {
		if util.DerefString(key) != "" && !kmsKeyNamePattern.MatchString(*key) {
			return util.NewBadRequestError(fmt.Sprintf(
				"%s %q is not a Cloud KMS key's resource name, such as projects/my-project/locations/us-east1/keyRings/my-ring/cryptoKeys/my-key",
				field, *key,
			))
		}
	}
	if settings.bootDiskType != nil && !slices.Contains(GkeBootDiskTypes, *settings.bootDiskType) {
		return util.NewBadRequestError(fmt.Sprintf(
			"BootDiskType %q is not one of %v", *settings.bootDiskType, GkeBootDiskTypes,
		))
	}
	if settings.bootDiskSizeGb != nil &&
		(*settings.bootDiskSizeGb < gkeMinBootDiskSizeGb || *settings.bootDiskSizeGb > gkeMaxBootDiskSizeGb) {
		return util.NewBadRequestError(fmt.Sprintf(
			"BootDiskSizeGb %d is not from %d to %d", *settings.bootDiskSizeGb, gkeMinBootDiskSizeGb, gkeMaxBootDiskSizeGb,
		))
	}
	if util.DerefString(settings.bootDiskKmsKey) != "" {
		diskType := GkeDefaultBootDiskType
		if settings.bootDiskType != nil {
			diskType = *settings.bootDiskType
		}
		if !slices.Contains(GkeKmsKeyBootDiskTypes, diskType) {
			return util.NewBadRequestError(fmt.Sprintf(
				"BootDiskKmsKey needs a BootDiskType of %v: GKE doesn't encrypt a %s boot disk with a key of yours",
				GkeKmsKeyBootDiskTypes, diskType,
			))
		}
	}

	return nil
}

// validateGkeNodeSecurityCreate checks a new GKE definition's encryption and
// boot settings.
func validateGkeNodeSecurityCreate(g *GcpGkeKubernetesRuntimeDefinition) error {
	settings, _ := gkeNodeSecurityOf(g)

	return validateGkeNodeSecurity(settings)
}

// validateGkeNodeSecurityUpdate checks the encryption and boot settings a
// GKE definition has after an update that changes any of them: those it
// sets, with the stored ones a PATCH leaves as they are.  It refuses one
// that clears SecretsEncryptionKey, and one that would replace the node
// pools of a definition whose clusters are protected from deletion: a
// change of BootDiskKmsKey or SecureBoot, which a node pool can't change.
func validateGkeNodeSecurityUpdate(tx *gorm.DB) error {
	incoming, ok := gkeNodeSecurityOf(lib.IncomingValues(tx))
	if !ok {
		return fmt.Errorf("cannot validate the node security settings: update carries %T", lib.IncomingValues(tx))
	}
	if !lib.IsFullReplace(tx) && incoming.secretsEncryptionKey == nil && incoming.bootDiskType == nil &&
		incoming.bootDiskSizeGb == nil && incoming.bootDiskKmsKey == nil && incoming.secureBoot == nil &&
		incoming.intranodeVisibility == nil {
		return nil
	}

	var current any
	var err error
	if lib.IsFullReplace(tx) {
		current, err = lib.LoadObjectFromDB(tx)
	} else {
		current, err = lib.LockObjectInDB(tx)
	}
	if err != nil {
		return err
	}
	stored, ok := gkeNodeSecurityOf(current)
	if !ok {
		return fmt.Errorf("cannot validate the node security settings: loaded %T", current)
	}

	updated := incoming
	if !lib.IsFullReplace(tx) {
		updated = stored
		if incoming.secretsEncryptionKey != nil {
			updated.secretsEncryptionKey = incoming.secretsEncryptionKey
		}
		if incoming.bootDiskType != nil {
			updated.bootDiskType = incoming.bootDiskType
		}
		if incoming.bootDiskSizeGb != nil {
			updated.bootDiskSizeGb = incoming.bootDiskSizeGb
		}
		if incoming.bootDiskKmsKey != nil {
			updated.bootDiskKmsKey = incoming.bootDiskKmsKey
		}
		if incoming.secureBoot != nil {
			updated.secureBoot = incoming.secureBoot
		}
		if incoming.intranodeVisibility != nil {
			updated.intranodeVisibility = incoming.intranodeVisibility
		}
		if incoming.deletionProtection != nil {
			updated.deletionProtection = incoming.deletionProtection
		}
	}

	if util.DerefString(stored.secretsEncryptionKey) != "" && util.DerefString(updated.secretsEncryptionKey) == "" {
		return util.NewBadRequestError(ErrMsgSecretsEncryptionKeyCleared)
	}
	if err := validateGkeNodeSecurity(updated); err != nil {
		return err
	}

	replacesNodePools := util.DerefString(stored.bootDiskKmsKey) != util.DerefString(updated.bootDiskKmsKey) ||
		util.Deref(stored.secureBoot) != util.Deref(updated.secureBoot)
	if !replacesNodePools || !util.Deref(updated.deletionProtection) || stored.id == nil {
		return nil
	}
	var instances int64
	if result := lib.NewCleanSession(tx).Model(&GcpGkeKubernetesRuntimeInstance{}).
		Where("gcp_gke_kubernetes_runtime_definition_id = ?", *stored.id).Count(&instances); result.Error != nil {
		return fmt.Errorf("failed to count the definition's runtime instances: %w", result.Error)
	}
	if instances > 0 {
		return util.NewBadRequestError(
			"changing BootDiskKmsKey or SecureBoot replaces the node pools of the definition's clusters, " +
				"which DeletionProtection protects: set DeletionProtection to false, in this update or before it, " +
				"to replace them",
		)
	}

	return nil
}

// GkePendingSettings returns the names of the secrets encryption, boot disk,
// Secure Boot and intranode visibility settings a GKE definition asks for
// that its instance's cluster doesn't run, as the instance records them.  A
// setting the definition leaves unset asks for nothing.  One the instance
// doesn't record, as on a cluster created before it was, runs GKE's default
// where the definition asks for that, Google-managed keys and Secure Boot and
// intranode visibility off, and is otherwise pending, so applying it records
// it.
func GkePendingSettings(definition *GcpGkeKubernetesRuntimeDefinition, instance *GcpGkeKubernetesRuntimeInstance) []string {
	var pending []string
	add := func(name string, asked bool) {
		if asked {
			pending = append(pending, name)
		}
	}
	add("SecretsEncryptionKey", pendingSetting(definition.SecretsEncryptionKey, instance.SecretsEncryptionKey))
	add("BootDiskType", pendingSetting(definition.BootDiskType, instance.BootDiskType))
	add("BootDiskSizeGb", pendingSetting(definition.BootDiskSizeGb, instance.BootDiskSizeGb))
	add("BootDiskKmsKey", pendingSetting(definition.BootDiskKmsKey, instance.BootDiskKmsKey))
	add("SecureBoot", pendingSetting(definition.SecureBoot, instance.SecureBoot))
	add("IntranodeVisibility", pendingSetting(definition.IntranodeVisibility, instance.IntranodeVisibility))

	return pending
}

// GkeSettingOrApplied returns the setting a GKE definition asks for, or,
// when it asks for none, the one its cluster runs, unless that is its zero
// value's default.  A field left out of a definition's update keeps what the
// cluster runs: the node pool a later upgrade builds keeps its boot disk key
// and Secure Boot rather than being replaced by one without them.  An
// explicit false or "" turns one off.
func GkeSettingOrApplied[T comparable](asked, applied *T) *T {
	if asked != nil {
		return asked
	}
	var zero T
	if applied == nil || *applied == zero {
		return nil
	}

	return applied
}

// pendingSetting reports whether a setting a definition asks for differs
// from the one applied, an unrecorded one being its zero value's default.
func pendingSetting[T comparable](asked, applied *T) bool {
	if asked == nil {
		return false
	}
	if applied == nil {
		var zero T
		return *asked != zero
	}

	return *asked != *applied
}
