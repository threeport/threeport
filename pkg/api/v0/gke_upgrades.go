package v0

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GkeReleaseChannels are the GKE release channels a cluster can be enrolled
// in.  Each offers every one of SupportedKubernetesVersions.  GKE upgrades a
// cluster's control plane and nodes to its channel's releases on its own.
var GkeReleaseChannels = []string{"REGULAR", "STABLE"}

// DefaultGkeReleaseChannel is the release channel of a cluster whose
// definition sets none.
const DefaultGkeReleaseChannel = "REGULAR"

// ProductionGkeReleaseChannel is the release channel of a production control
// plane's cluster: its releases have been through REGULAR first.
const ProductionGkeReleaseChannel = "STABLE"

// gkeMaintenanceWindowDays are the days a maintenance window can recur on, as
// an RRULE writes them.
var gkeMaintenanceWindowDays = []string{"MO", "TU", "WE", "TH", "FR", "SA", "SU"}

// The maintenance window of a cluster whose definition sets none: Saturday
// and Sunday, from 04:00 to 12:00 UTC.
const (
	DefaultGkeMaintenanceWindowDays  = "SA,SU"
	DefaultGkeMaintenanceWindowStart = "04:00"
	DefaultGkeMaintenanceWindowHours = 8
)

// GKE upgrades only in an occurrence of a maintenance window at least four
// hours long.  The window can't run past the next day's.
const (
	minGkeMaintenanceWindowHours = 4
	maxGkeMaintenanceWindowHours = 24
)

// ErrMsgGkeUpgradeSettingsImmutable is the error a change to a GKE
// definition's release channel or maintenance window returns.  Threeport
// can't change a cluster after creating it yet, so a changed setting would
// record one its clusters don't have.
const ErrMsgGkeUpgradeSettingsImmutable = "ReleaseChannel and the MaintenanceWindow fields cannot be changed after creation: changing a cluster is not supported yet"

// GkeReleaseChannelOrDefault returns channel, or DefaultGkeReleaseChannel when
// it is unset.
func GkeReleaseChannelOrDefault(channel *string) string {
	if channel == nil || *channel == "" {
		return DefaultGkeReleaseChannel
	}

	return *channel
}

// GkeMaintenanceWindowDaysOrDefault returns days, or
// DefaultGkeMaintenanceWindowDays when it is unset.
func GkeMaintenanceWindowDaysOrDefault(days *string) string {
	if days == nil || *days == "" {
		return DefaultGkeMaintenanceWindowDays
	}

	return *days
}

// GkeMaintenanceWindowStartOrDefault returns start, or
// DefaultGkeMaintenanceWindowStart when it is unset.
func GkeMaintenanceWindowStartOrDefault(start *string) string {
	if start == nil || *start == "" {
		return DefaultGkeMaintenanceWindowStart
	}

	return *start
}

// GkeMaintenanceWindowHoursOrDefault returns hours, or
// DefaultGkeMaintenanceWindowHours when it is unset.
func GkeMaintenanceWindowHoursOrDefault(hours *int) int {
	if hours == nil || *hours == 0 {
		return DefaultGkeMaintenanceWindowHours
	}

	return *hours
}

// ValidateGkeUpgradeSettings checks a GKE definition's release channel and
// maintenance window.  Each is optional, and an unset one takes its default.
func ValidateGkeUpgradeSettings(channel, days, start *string, hours *int) error {
	if channel != nil && !slices.Contains(GkeReleaseChannels, *channel) {
		return fmt.Errorf(
			"ReleaseChannel %q is not supported: use one of %s",
			*channel, strings.Join(GkeReleaseChannels, ", "),
		)
	}

	if days != nil {
		seen := map[string]bool{}
		for _, day := range strings.Split(*days, ",") {
			if !slices.Contains(gkeMaintenanceWindowDays, day) || seen[day] {
				return fmt.Errorf(
					"MaintenanceWindowDays %q is not a list of distinct days: separate %s with commas, such as %q",
					*days, strings.Join(gkeMaintenanceWindowDays, ", "), DefaultGkeMaintenanceWindowDays,
				)
			}
			seen[day] = true
		}
	}

	if start != nil {
		if _, err := time.Parse("15:04", *start); err != nil || len(*start) != len("15:04") {
			return fmt.Errorf(
				"MaintenanceWindowStart %q is not a time of day in UTC, such as %q",
				*start, DefaultGkeMaintenanceWindowStart,
			)
		}
	}

	if hours != nil && (*hours < minGkeMaintenanceWindowHours || *hours > maxGkeMaintenanceWindowHours) {
		return fmt.Errorf(
			"MaintenanceWindowHours %d is out of range: GKE upgrades only in a window of %d to %d hours",
			*hours, minGkeMaintenanceWindowHours, maxGkeMaintenanceWindowHours,
		)
	}

	return nil
}

// validateGkeUpgradeSettingsCreate checks the release channel and maintenance
// window of a GKE definition being created.
func validateGkeUpgradeSettingsCreate(g *GcpGkeKubernetesRuntimeDefinition) error {
	if err := ValidateGkeUpgradeSettings(
		g.ReleaseChannel,
		g.MaintenanceWindowDays,
		g.MaintenanceWindowStart,
		g.MaintenanceWindowHours,
	); err != nil {
		return util.NewBadRequestError(err.Error())
	}

	return nil
}

// validateGkeUpgradeSettingsUpdate refuses an update that changes a GKE
// definition's release channel or maintenance window.
func validateGkeUpgradeSettingsUpdate(tx *gorm.DB) error {
	for _, field := range []string{
		"ReleaseChannel",
		"MaintenanceWindowDays",
		"MaintenanceWindowStart",
		"MaintenanceWindowHours",
	} {
		changed, err := lib.IsFieldChanged(tx, field)
		if err != nil {
			return err
		}
		if changed {
			return util.NewBadRequestError(ErrMsgGkeUpgradeSettingsImmutable)
		}
	}

	return nil
}
