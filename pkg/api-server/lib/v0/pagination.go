package v0

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	PaginationViewPrefix = "paginated"
)

// CleanupMaterializedViews runs a cleanup routine for old materialized views every interval seconds.
// It will drop views older than ttlMinutes.
func CleanupMaterializedViews(db *gorm.DB, logger *zap.Logger, interval, ttlMinutes int) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	go func() {
		for range ticker.C {
			if err := dropPaginationViews(db, logger, ttlMinutes); err != nil {
				logger.Error("failed to drop materialized views", zap.Error(err))
			}
		}
	}()
}

// dropPaginationViews drops old materialized views for pagination.  It uses the timestamp
// in the view name to determine if the view has passed its TTL.
func dropPaginationViews(db *gorm.DB, logger *zap.Logger, ttlMinutes int) error {
	// list materialized views for pagination
	query := fmt.Sprintf(`
        SELECT table_name
        FROM information_schema.tables
        WHERE table_type = 'VIEW'
        AND table_name LIKE '%s_%%';
    `, PaginationViewPrefix)
	var viewNames []string
	if result := db.Raw(query).Scan(&viewNames); result.Error != nil {
		return fmt.Errorf("failed to query materialized view names: %w", result.Error)
	}

	// regex to extract timestamp from view name
	re := regexp.MustCompile(fmt.Sprintf(`%s_(\d{14})_.*`, PaginationViewPrefix))
	now := time.Now()
	ttl := time.Duration(ttlMinutes) * time.Minute

	var viewsToDrop []string
	for _, viewName := range viewNames {
		// extract timestamp from view name
		matches := re.FindStringSubmatch(viewName)
		if len(matches) != 2 {
			logger.Warn("skipping view with invalid name format", zap.String("viewName", viewName))
			continue
		}

		// parse timestamp (YYYYMMDDHHMMSS)
		timestamp, err := time.Parse("20060102150405", matches[1])
		if err != nil {
			logger.Warn("skipping view with invalid timestamp", zap.String("viewName", viewName), zap.Error(err))
			continue
		}

		// check if view is older than TTL
		if now.Sub(timestamp) > ttl {
			viewsToDrop = append(viewsToDrop, viewName)
		}
	}

	// drop old views
	for _, viewName := range viewsToDrop {
		dropQuery := fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", viewName)
		if result := db.Exec(dropQuery); result.Error != nil {
			return fmt.Errorf("failed to drop view %s: %w", viewName, result.Error)
		}
		logger.Info("dropped materialized view", zap.String("viewName", viewName))
	}

	return nil
}

// hlcTokenPattern matches a CRDB cluster_logical_timestamp() rendering:
// digits, one dot, more digits. Anchored to guard against injection when
// the token gets interpolated into an AS OF SYSTEM TIME clause.
var hlcTokenPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// ValidHLCToken reports whether s looks like a CRDB HLC decimal, so a
// caller-supplied pagination queryid can be safely interpolated into an
// AS OF SYSTEM TIME clause.
func ValidHLCToken(s string) bool {
	return hlcTokenPattern.MatchString(s)
}

// queryIdPattern matches the queryid GenerateMaterializedViewName() hands out:
// sixteen characters drawn from the lowercase alphanumeric set. Anchored so a
// caller-supplied value cannot carry SQL into the view lookup.
var queryIdPattern = regexp.MustCompile(`^[a-z0-9]{16}$`)

// ValidPaginationQueryId reports whether s has the shape of a queryid the
// server issues for a materialized-view pagination window.
func ValidPaginationQueryId(s string) bool {
	return queryIdPattern.MatchString(s)
}

// ErrInvalidPaginationQueryId reports that a caller-supplied queryid is not one
// the server could have issued, so it names no snapshot and reaches no query.
// Both pagination modes wrap it: an as-of-system-time token that is not a bare
// HLC decimal, and a materialized-view id that is not the shape
// GenerateMaterializedViewName() hands out. Handlers match on it to answer a
// client error rather than reporting a server fault.
var ErrInvalidPaginationQueryId = errors.New("invalid queryid")

// ErrPaginationSessionExpired reports that the snapshot a queryid names is
// gone, so the client has to start the result set over with no queryid. Both
// pagination modes reach it: an as-of-system-time snapshot that has passed the
// garbage-collection threshold, and a materialized view that has been dropped.
var ErrPaginationSessionExpired = errors.New("pagination session expired, restart pagination with no queryid to obtain a fresh snapshot")

// TranslatePaginationSessionError maps a CRDB "batch timestamp below GC
// threshold" failure onto ErrPaginationSessionExpired, keeping the original
// wrapped for the logs. Any other error is returned unchanged.
func TranslatePaginationSessionError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "batch timestamp") && strings.Contains(msg, "replica GC threshold") {
		return fmt.Errorf("%w: %w", ErrPaginationSessionExpired, err)
	}
	return err
}

// undefinedTableSQLState is the SQLSTATE a read against a table or view that is
// not there comes back with, which the driver carries in the error text
// alongside the server's message.
const undefinedTableSQLState = "42P01"

// TranslateDroppedViewError maps an undefined-table failure naming viewName
// onto ErrPaginationSessionExpired, keeping the original wrapped for the logs.
// The TTL sweeper can drop a pagination view between the lookup that resolves
// its name and the read that pages from it, and the client's recourse there is
// the same as for an expired snapshot: restart with no queryid.
//
// The match is scoped twice, to a name carrying the pagination view prefix and
// to that name appearing in the message, so a base table that is genuinely
// missing stays a server fault. Any other error is returned unchanged.
func TranslateDroppedViewError(err error, viewName string) error {
	if err == nil {
		return nil
	}
	if !strings.HasPrefix(viewName, PaginationViewPrefix+"_") {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, viewName) {
		return err
	}
	if strings.Contains(msg, "does not exist") || strings.Contains(msg, undefinedTableSQLState) {
		return fmt.Errorf("%w: %w", ErrPaginationSessionExpired, err)
	}
	return err
}
