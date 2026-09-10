package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	echo "github.com/labstack/echo/v4"
	zap "go.uber.org/zap"
	"gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// objectNamespacePattern matches a DNS-like API namespace, e.g. threeport.io.
// Anchored so the value can be interpolated into a LIKE clause on object_type.
var objectNamespacePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*$`)

// objectVersionPattern matches an alphanumeric version token, e.g. v0 or v1alpha1.
// Anchored so the value can be interpolated into a LIKE clause on object_type.
var objectVersionPattern = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

// reasonPattern matches a reason token of letters, digits, or _, e.g. SuccessfulCreate or Reconcile_Fail.
// Anchored so the value can be interpolated into equality and LIKE predicates on reason.
var reasonPattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// objectNamePattern matches a DNS-like object name, e.g. my-widget.
// Holds objectname and objectnameprefix to that shape before they reach the name resolvers.
var objectNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// qualifiedTypePattern matches a fully qualified type of the form
// <namespace>/<version>.<TypeName>, e.g. threeport.io/v0.Profile.
// Event-row object_type values are held to this shape before interpolation into SQL.
var qualifiedTypePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*/[a-zA-Z0-9]+\.[a-zA-Z0-9]+$`)

// materializedViewThresholdFloor is the floor of the first-page probe,
// max(Limit*10, this value). Under that probe, the listing returns the
// whole result set and skips the snapshot.
const materializedViewThresholdFloor = 5000

// boundEventFilterClause returns parameterized AND clauses for Event filter
// fields that bind onto the row. Reason is left out because the reason and
// reasonprefix query params own that column. Time columns are left out
// because the query binder rejects time values, so they never arrive on a
// list request.
func boundEventFilterClause(filter *v0.Event) (string, []interface{}) {
	var fragments []string
	var values []interface{}

	add := func(column string, value interface{}) {
		fragments = append(fragments, fmt.Sprintf(" AND v0_events.%s = ?", column))
		values = append(values, value)
	}

	if filter.ID != nil {
		add("id", *filter.ID)
	}
	if filter.Note != nil {
		add("note", *filter.Note)
	}
	if filter.Count != nil {
		add("count", *filter.Count)
	}
	if filter.Type != nil {
		add("type", *filter.Type)
	}
	if filter.ReportingController != nil {
		add("reporting_controller", *filter.ReportingController)
	}
	if filter.ObjectType != nil {
		add("object_type", *filter.ObjectType)
	}
	if filter.ObjectID != nil {
		add("object_id", *filter.ObjectID)
	}

	return strings.Join(fragments, ""), values
}

// GetEventsFiltered lists events filtered by subject type, id, name, and reason, and fills in each event's object name.
// @Summary gets all events, filtered by subject.
// @Description Get events from the Threeport database, narrowed by the object_type and object_id columns each event row carries.
// @ID get-v0-events-filtered
// @Accept json
// @Produce json
// @Param objectid query string false "filter events by object ID"
// @Param objecttypename query string false "filter events by object type name; CamelCase Go TypeName like 'KubernetesWorkloadInstance'. Filters on its own, and narrows objectid, objectname, or objectnameprefix to one kind"
// @Param objectversion query string false "narrow objecttypename match to one version (e.g. 'v0')"
// @Param objectnamespace query string false "narrow objecttypename match to one api namespace (e.g. 'threeport.io')"
// @Param objectname query string false "filter events by exact object name; matches every subject type carrying that name, deleted subjects included, unless objecttypename narrows it"
// @Param objectnameprefix query string false "filter events by object name prefix; matches every subject whose name starts with this token, deleted subjects included, across every subject type unless objecttypename narrows it"
// @Param reason query string false "filter events by exact Reason match (case-sensitive CamelCase, e.g. 'SuccessfulCreate')"
// @Param reasonprefix query string false "filter events by Reason prefix (case-sensitive CamelCase, matches Reason values starting with this token)"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/events-filtered [GET]
func (h Handler) GetEventsFiltered(c echo.Context) error {
	objectType := v0.ObjectTypeEvent

	// get pagination parameters
	pageParams, err := c.(*apiserver_lib.CustomContext).GetPaginationParams()
	if err != nil {
		return apiserver_lib.ResponseStatus400(c, pageParams, err, objectType)
	}

	// bind filter
	var filter v0.Event
	if err := c.Bind(&filter); err != nil {
		h.Logger.Error("handler error: error binding filter", zap.Error(err))
		return apiserver_lib.ResponseStatus400(c, pageParams, err, objectType)
	}

	// collect the subject filter. objecttypename, objectid, objectname,
	// and objectnameprefix each narrow on their own; objecttypename
	// combines with any one of the other three. objectid, objectname,
	// and objectnameprefix are mutually exclusive.
	targetTypeName := c.QueryParam("objecttypename")
	targetVersion := c.QueryParam("objectversion")
	targetNamespace := c.QueryParam("objectnamespace")
	targetName := c.QueryParam("objectname")
	targetNamePrefix := c.QueryParam("objectnameprefix")
	directObjectId := c.QueryParam("objectid")
	targetReason := c.QueryParam("reason")
	targetReasonPrefix := c.QueryParam("reasonprefix")

	// reject tokens that would not be safe to interpolate into the object_type LIKE clause
	if targetNamespace != "" && !objectNamespacePattern.MatchString(targetNamespace) {
		return apiserver_lib.ResponseStatus400(c, pageParams,
			fmt.Errorf("invalid objectnamespace %q: expected DNS-like value", targetNamespace),
			objectType)
	}
	if targetVersion != "" && !objectVersionPattern.MatchString(targetVersion) {
		return apiserver_lib.ResponseStatus400(c, pageParams,
			fmt.Errorf("invalid objectversion %q: expected alphanumeric token", targetVersion),
			objectType)
	}
	// reject pairing reason with reasonprefix; both interpolate into predicates on reason
	if targetReason != "" && targetReasonPrefix != "" {
		return apiserver_lib.ResponseStatus400(c, pageParams,
			errors.New("provide either reason or reasonprefix, not both"),
			objectType)
	}
	if targetReason != "" && !reasonPattern.MatchString(targetReason) {
		return apiserver_lib.ResponseStatus400(c, pageParams,
			fmt.Errorf("invalid reason %q: expected CamelCase token", targetReason),
			objectType)
	}
	if targetReasonPrefix != "" && !reasonPattern.MatchString(targetReasonPrefix) {
		return apiserver_lib.ResponseStatus400(c, pageParams,
			fmt.Errorf("invalid reasonprefix %q: expected CamelCase token", targetReasonPrefix),
			objectType)
	}
	// reject pairing objectname, objectnameprefix, and objectid; each names the subject
	if targetName != "" && targetNamePrefix != "" {
		return apiserver_lib.ResponseStatus400(c, pageParams,
			errors.New("provide either objectname or objectnameprefix, not both"),
			objectType)
	}
	if directObjectId != "" && targetNamePrefix != "" {
		return apiserver_lib.ResponseStatus400(c, pageParams,
			errors.New("provide either objectid or objectnameprefix, not both"),
			objectType)
	}
	if directObjectId != "" && targetName != "" {
		return apiserver_lib.ResponseStatus400(c, pageParams,
			errors.New("provide either objectid or objectname, not both"),
			objectType)
	}
	if targetNamePrefix != "" && !objectNamePattern.MatchString(targetNamePrefix) {
		return apiserver_lib.ResponseStatus400(c, pageParams,
			fmt.Errorf("invalid objectnameprefix %q: expected DNS-like name token", targetNamePrefix),
			objectType)
	}
	if targetName != "" && !objectNamePattern.MatchString(targetName) {
		return apiserver_lib.ResponseStatus400(c, pageParams,
			fmt.Errorf("invalid objectname %q: expected DNS-like name token", targetName),
			objectType)
	}

	var ids []uint
	var fullyQualifiedTypes []string

	// resolve the bare kind to fully qualified types, then apply namespace and version
	resolveQualifiedTypes := func() ([]string, error) {
		types, err := apiserver_lib.GetObjectTypes(h.DB, targetTypeName)
		if err != nil {
			return nil, err
		}
		return apiserver_lib.FilterQualifiedTypes(types, targetNamespace, targetVersion), nil
	}

	// build a LIKE pattern for object_type from namespace and version.
	// object_type is stored as namespace/version.TypeName, so the
	// patterns anchor on the slash and the dot
	buildNamespaceVersionPattern := func() (pattern string, active bool) {
		switch {
		case targetNamespace != "" && targetVersion != "":
			return fmt.Sprintf("%s/%s.%%", targetNamespace, targetVersion), true
		case targetNamespace != "":
			return fmt.Sprintf("%s/%%", targetNamespace), true
		case targetVersion != "":
			return fmt.Sprintf("%%/%s.%%", targetVersion), true
		default:
			return "", false
		}
	}

	// subjects resolved from an objectname or objectnameprefix filter,
	// each type paired with its own ids so a shared id on another type stays out
	var nameMatchedSubjects []eventSubjectGroup

	// return types to search: types present on events when no kind is given,
	// otherwise the registered types for that kind. the status is the
	// response code to answer when the error is non-nil
	candidateSubjectTypes := func() ([]string, int, error) {
		if targetTypeName == "" {
			presentTypes, err := eventSubjectTypes(h.DB)
			if err != nil {
				return nil, http.StatusInternalServerError, err
			}

			return apiserver_lib.FilterQualifiedTypes(presentTypes, targetNamespace, targetVersion), 0, nil
		}

		resolvedTypes, err := resolveQualifiedTypes()
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
		if len(resolvedTypes) == 0 {
			return nil, http.StatusNotFound,
				fmt.Errorf("kind %q is not registered (or no version/namespace match)", targetTypeName)
		}

		return resolvedTypes, 0, nil
	}

	// map a type-lookup failure to the matching HTTP status
	respondTypeLookup := func(status int, err error) error {
		switch status {
		case http.StatusInternalServerError:
			h.Logger.Error("handler error: error reading event subject types", zap.Error(err))
			return apiserver_lib.ResponseStatus500(c, pageParams, err, objectType)
		case http.StatusNotFound:
			return apiserver_lib.ResponseStatus404(c, pageParams, err, objectType)
		default:
			h.Logger.Error("handler error: error looking up object types", zap.Error(err))
			return apiserver_lib.ResponseStatus400(c, pageParams, err, objectType)
		}
	}

	switch {
	case targetNamePrefix != "":
		// resolve every subject whose name starts with the prefix; a
		// prefix spans types unless objecttypename narrows the set
		candidateTypes, status, lookupErr := candidateSubjectTypes()
		if lookupErr != nil {
			return respondTypeLookup(status, lookupErr)
		}

		matched, lookupErr := resolveSubjectsByNamePrefix(
			c.Request().Context(), h.DB, candidateTypes, targetNamePrefix, h.Logger,
		)
		if lookupErr != nil {
			h.Logger.Error("handler error: error resolving object name prefix", zap.Error(lookupErr))
			return apiserver_lib.ResponseStatus500(c, pageParams, lookupErr, objectType)
		}
		if len(matched) == 0 {
			return apiserver_lib.ResponseStatus404(c, pageParams,
				fmt.Errorf("no object found with name prefix %q", targetNamePrefix), objectType)
		}
		nameMatchedSubjects = matched

	case targetName != "":
		// resolve every subject whose name matches exactly; a name is
		// unique only within a type, so each type keeps the ids it yielded
		candidateTypes, status, lookupErr := candidateSubjectTypes()
		if lookupErr != nil {
			return respondTypeLookup(status, lookupErr)
		}

		matched := resolveSubjectsByName(h.DB, candidateTypes, targetName, h.Logger)
		if len(matched) == 0 {
			if targetTypeName != "" {
				return apiserver_lib.ResponseStatus404(c, pageParams,
					fmt.Errorf("no object found with name %q for kind %q", targetName, targetTypeName), objectType)
			}

			return apiserver_lib.ResponseStatus404(c, pageParams,
				fmt.Errorf("no object found with name %q", targetName), objectType)
		}
		nameMatchedSubjects = matched

	case directObjectId != "":
		// parse the id; a type alongside it pins object_type so an
		// unrelated type that shares the id stays out
		parsed, err := strconv.ParseUint(directObjectId, 10, 64)
		if err != nil {
			return apiserver_lib.ResponseStatus400(c, pageParams,
				fmt.Errorf("invalid objectid %q: %w", directObjectId, err), objectType)
		}
		ids = []uint{uint(parsed)}

		if targetTypeName != "" {
			types, status, lookupErr := candidateSubjectTypes()
			if lookupErr != nil {
				return respondTypeLookup(status, lookupErr)
			}
			fullyQualifiedTypes = types
		}

	case targetTypeName != "":
		// filter on the kind's fully qualified types; every id under
		// those types is in the answer
		types, status, lookupErr := candidateSubjectTypes()
		if lookupErr != nil {
			return respondTypeLookup(status, lookupErr)
		}
		fullyQualifiedTypes = types

	default:
		// no subject selected. a namespace or version on its own still
		// narrows object_type through the LIKE predicate below
	}

	// pagination state is built up across the branches below and read
	// into the final response Meta
	pagination := new(apiserver_lib.Pagination)
	pagination.Limit = pageParams.Limit

	records := &[]v0.Event{}
	var returnedCount int64

	// build a raw SQL fragment for an exact or prefix reason filter.
	// values already match reasonPattern, so they interpolate as literals
	buildReasonRawWhere := func() (string, bool) {
		switch {
		case targetReason != "":
			return fmt.Sprintf("v0_events.reason = '%s'", targetReason), true
		case targetReasonPrefix != "":
			return fmt.Sprintf("v0_events.reason LIKE '%s%%'", targetReasonPrefix), true
		default:
			return "", false
		}
	}

	// build the WHERE clause for raw SQL pagination queries. raw SQL
	// does not pick up gorm's deleted_at scoping, so the live-rows
	// predicate is explicit
	buildRawWhere := func() string {
		whereClause := " WHERE " + apiserver_lib.LiveRowsFilter("v0_events")
		if len(fullyQualifiedTypes) > 0 {
			typeStrs := make([]string, len(fullyQualifiedTypes))
			for i, t := range fullyQualifiedTypes {
				typeStrs[i] = fmt.Sprintf("'%s'", t)
			}
			whereClause += fmt.Sprintf(
				" AND v0_events.object_type IN (%s)",
				strings.Join(typeStrs, ", "),
			)
		}
		if len(ids) > 0 {
			idStrs := make([]string, len(ids))
			for i, id := range ids {
				idStrs[i] = fmt.Sprintf("%d", id)
			}
			whereClause += fmt.Sprintf(
				" AND v0_events.object_id IN (%s)",
				strings.Join(idStrs, ", "),
			)
		}
		if len(nameMatchedSubjects) > 0 {
			// pair each resolved type with its ids so a name match on two
			// types does not cross-product ids onto the wrong type
			groups := make([]string, 0, len(nameMatchedSubjects))
			for _, group := range nameMatchedSubjects {
				idStrs := make([]string, len(group.IDs))
				for i, id := range group.IDs {
					idStrs[i] = fmt.Sprintf("%d", id)
				}
				groups = append(groups, fmt.Sprintf(
					"(v0_events.object_type = '%s' AND v0_events.object_id IN (%s))",
					group.QualifiedType,
					strings.Join(idStrs, ", "),
				))
			}
			whereClause += " AND (" + strings.Join(groups, " OR ") + ")"
		}
		if pattern, active := buildNamespaceVersionPattern(); active {
			// narrow object_type by namespace, version, or both when no kind was resolved
			whereClause += fmt.Sprintf(
				" AND v0_events.object_type LIKE '%s'",
				pattern,
			)
		}
		if reasonFrag, active := buildReasonRawWhere(); active {
			whereClause += " AND " + reasonFrag
		}

		return whereClause
	}

	// apply subject and reason filters on a gorm query. a type set and
	// ids together match every (type, id) pair
	applyObjectIdFilter := func(query *gorm.DB) *gorm.DB {
		if len(fullyQualifiedTypes) > 0 {
			query = query.Where("v0_events.object_type IN ?", fullyQualifiedTypes)
		}
		if len(ids) > 0 {
			query = query.Where("v0_events.object_id IN ?", ids)
		}
		if len(nameMatchedSubjects) > 0 {
			// pair each resolved type with its ids so a name match on two
			// types does not cross-product ids onto the wrong type
			clauses := make([]string, 0, len(nameMatchedSubjects))
			values := make([]interface{}, 0, len(nameMatchedSubjects)*2)
			for _, group := range nameMatchedSubjects {
				clauses = append(clauses, "(v0_events.object_type = ? AND v0_events.object_id IN ?)")
				values = append(values, group.QualifiedType, group.IDs)
			}
			query = query.Where(strings.Join(clauses, " OR "), values...)
		}
		if pattern, active := buildNamespaceVersionPattern(); active {
			query = query.Where("v0_events.object_type LIKE ?", pattern)
		}
		if targetReason != "" {
			query = query.Where("v0_events.reason = ?", targetReason)
		}
		if targetReasonPrefix != "" {
			query = query.Where("v0_events.reason LIKE ?", targetReasonPrefix+"%")
		}
		return query
	}

	// bind remaining Event fields. these stay off the view definition
	// because a view takes no placeholders; a view read applies them
	// with the table prefix stripped, because the view columns are unqualified
	boundClause, boundValues := boundEventFilterClause(&filter)
	viewBoundClause := strings.ReplaceAll(boundClause, "v0_events.", "")

	switch {
	case pageParams.QueryId == "":
		// first-page request: no QueryId means the client is asking
		// for the start of a fresh result set, not a continuation

		// probe max(Limit*10, floor) rows so the pagination decision
		// comes from returned row count instead of a separate Count query
		threshold := pagination.Limit * 10
		if threshold < materializedViewThresholdFloor {
			threshold = materializedViewThresholdFloor
		}

		// fetch threshold+1 rows; under the threshold, serve them and skip the snapshot
		findQuery := h.DB.Order("event_time ASC, id ASC").Limit(int(threshold) + 1)
		if result := applyObjectIdFilter(findQuery).Where(&filter).Find(records); result.Error != nil {
			h.Logger.Error("handler error: error finding objects", zap.Error(result.Error))
			return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
		}
		pagination.HasMore = int64(len(*records)) > threshold

		switch pagination.HasMore {
		case false:
			// small enough to return in one page
			returnedCount = int64(len(*records))

		case true:
			// discard the probe rows and pin a snapshot so later pages
			// see the same rows under concurrent writes
			*records = (*records)[:0]

			whereClause := buildRawWhere()

			switch h.paginationMode() {
			case apiserver_lib.PaginationModeAsOfSystemTime:
				// capture the HLC once; the client echoes it back on
				// every continuation so all pages read the same snapshot
				hlc, err := h.resolveHLCSnapshot("")
				if err != nil {
					h.Logger.Error("handler error: error capturing HLC snapshot", zap.Error(err))
					return apiserver_lib.ResponseStatus500(c, pageParams, err, objectType)
				}
				pagination.QueryId = hlc

				// page the snapshot in id order. AS OF SYSTEM TIME sits
				// between FROM and WHERE
				query := fmt.Sprintf(`
					SELECT v0_events.*
					FROM v0_events
					AS OF SYSTEM TIME '%s'
					%s%s
					ORDER BY v0_events.id ASC
					LIMIT %d
				`,
					hlc,
					whereClause,
					boundClause,
					pageParams.Limit,
				)
				if result := h.DB.Raw(query, boundValues...).Find(records); result.Error != nil {
					h.Logger.Error("handler error: error finding objects", zap.Error(result.Error))
					return apiserver_lib.ResponseStatus500(c, pageParams, apiserver_lib.TranslatePaginationSessionError(result.Error), objectType)
				}
				returnedCount = int64(len(*records))

			default:
				// materialize the filtered set so continuation pages share one snapshot
				viewName, queryId := GenerateMaterializedViewName()

				// persist the filtered rows in event_time order, id breaking ties
				createView := fmt.Sprintf(`
					CREATE MATERIALIZED VIEW %s AS
					SELECT v0_events.*
					FROM v0_events
					%s
					ORDER BY v0_events.event_time ASC, v0_events.id ASC
				`,
					viewName,
					whereClause,
				)
				if result := h.DB.Exec(createView); result.Error != nil {
					h.Logger.Error("handler error: error creating materialized view", zap.Error(result.Error))
					return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
				}

				// index on ID so subsequent cursor pagination (WHERE ID > cursor)
				// doesn't full-scan the view
				createIdIndex := fmt.Sprintf("CREATE INDEX ON %s (ID)", viewName)
				if result := h.DB.Exec(createIdIndex); result.Error != nil {
					h.Logger.Error("handler error: error creating ID index", zap.Error(result.Error))
					return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
				}

				// expose the queryId so the client can request subsequent pages
				pagination.QueryId = queryId

				// fetch the first page from the view; subject filters are
				// in the view definition, filter-struct predicates bind on the read
				query := fmt.Sprintf(
					"SELECT * FROM %s WHERE TRUE%s ORDER BY ID ASC LIMIT %d",
					viewName,
					viewBoundClause,
					pageParams.Limit,
				)
				if result := h.DB.Raw(query, boundValues...).Find(records); result.Error != nil {
					h.Logger.Error("handler error: error finding objects", zap.Error(result.Error))
					return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
				}
				returnedCount = int64(len(*records))
			}

			// set NextCursor to the last record's ID so the client's
			// next request resumes at the row right after this one
			if len(*records) > 0 {
				pagination.NextCursor = *(*records)[len(*records)-1].ID
			} else {
				pagination.NextCursor = 0
			}
		}

	case pageParams.QueryId != "" && pageParams.Cursor == 0:
		// QueryId without Cursor is incoherent - we can't know which
		// page to return without a cursor position
		return apiserver_lib.ResponseStatus400(c, pageParams, errors.New("cursor is required when query ID is provided"), objectType)

	case pageParams.QueryId != "" && pageParams.Cursor != 0:
		// continuation request: client gave a QueryId+Cursor pair,
		// resume from the snapshot the first-page call anchored. the
		// queryId stays opaque to the client: materialized-view mode
		// reads it as a view suffix, as-of-system-time mode reads it as
		// an HLC.

		// preserve the queryId across pages so the client keeps using
		// the same snapshot for subsequent continuation requests
		pagination.QueryId = pageParams.QueryId

		// viewName is set only in materialized-view mode, so the last-page drop is a no-op otherwise
		var viewName string

		switch h.paginationMode() {
		case apiserver_lib.PaginationModeAsOfSystemTime:
			// treat the caller queryId as an HLC token; validate to
			// reject anything that would smuggle SQL into AS OF SYSTEM
			// TIME
			if !apiserver_lib.ValidHLCToken(pageParams.QueryId) {
				return apiserver_lib.ResponseStatus400(c, pageParams,
					errors.New("invalid queryid: not a valid HLC token, restart pagination with no queryid to obtain a fresh snapshot"),
					objectType)
			}

			// resume after the cursor on the same snapshot
			whereClause := buildRawWhere()
			whereClause += fmt.Sprintf(" AND v0_events.id > %d", pageParams.Cursor)

			recordsQuery := fmt.Sprintf(`
				SELECT v0_events.*
				FROM v0_events
				AS OF SYSTEM TIME '%s'
				%s%s
				ORDER BY v0_events.id ASC
				LIMIT %d
			`,
				pageParams.QueryId,
				whereClause,
				boundClause,
				pageParams.Limit,
			)
			// a snapshot past the garbage-collection threshold is the
			// client's to recover from by restarting pagination, so it
			// answers 400 the way the generated list handlers do
			if result := h.DB.Raw(recordsQuery, boundValues...).Find(records); result.Error != nil {
				pageErr := apiserver_lib.TranslatePaginationSessionError(result.Error)
				if errors.Is(pageErr, apiserver_lib.ErrPaginationSessionExpired) {
					return apiserver_lib.ResponseStatus400(c, pageParams, pageErr, objectType)
				}
				h.Logger.Error("handler error: error finding records", zap.Error(result.Error))
				return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
			}
			returnedCount = int64(len(*records))

		default:
			// treat the caller queryId as a view suffix; validate to
			// reject anything that would smuggle SQL into the view lookup
			if !apiserver_lib.ValidPaginationQueryId(pageParams.QueryId) {
				return apiserver_lib.ResponseStatus400(c, pageParams,
					errors.New("invalid queryid: not a server-issued pagination query id, restart pagination with no queryid to obtain a fresh snapshot"),
					objectType)
			}

			// use the query ID to find the materialized view name (the view
			// name is deterministic from the queryId)
			var err error
			viewName, err = h.GetMaterializedViewName(pageParams.QueryId)
			if err != nil {
				h.Logger.Error("handler error: error finding materialized view", zap.Error(err))
				return apiserver_lib.ResponseStatus500(c, pageParams, err, objectType)
			}

			// a queryid naming no live view means the snapshot is gone,
			// either dropped with the tail page or swept by the TTL. An
			// empty name would otherwise build SQL with no table and fail
			// as a syntax error.
			if viewName == "" {
				return apiserver_lib.ResponseStatus400(c, pageParams,
					apiserver_lib.ErrPaginationSessionExpired, objectType)
			}

			recordsQuery := fmt.Sprintf(
				"SELECT * FROM %s WHERE ID > %d%s ORDER BY ID ASC LIMIT %d",
				viewName,
				pageParams.Cursor,
				viewBoundClause,
				pageParams.Limit,
			)
			// the TTL sweeper can drop the view between the lookup above
			// and this read, which leaves the client in the same place an
			// expired snapshot does: restart pagination with no queryid
			if result := h.DB.Raw(recordsQuery, boundValues...).Find(records); result.Error != nil {
				pageErr := apiserver_lib.TranslateDroppedViewError(result.Error, viewName)
				if errors.Is(pageErr, apiserver_lib.ErrPaginationSessionExpired) {
					return apiserver_lib.ResponseStatus400(c, pageParams, pageErr, objectType)
				}
				h.Logger.Error("handler error: error finding records", zap.Error(result.Error))
				return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
			}
			returnedCount = int64(len(*records))
		}

		// set the next cursor to the last record's ID, or 0 when the
		// page came back empty (caller can treat 0 as "no more")
		if len(*records) > 0 {
			pagination.NextCursor = *(*records)[len(*records)-1].ID
		} else {
			pagination.NextCursor = 0
		}

		// returnedCount >= limit means there's likely another page; a
		// smaller-than-limit page means we hit the tail
		pagination.HasMore = returnedCount >= pagination.Limit

		// drop the view once the tail page is returned. a drop failure
		// is logged, not returned; the TTL sweeper still drops the view
		if !pagination.HasMore && viewName != "" {
			dropQuery := fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", viewName)
			if result := h.DB.Exec(dropQuery); result.Error != nil {
				h.Logger.Error("handler error: error dropping materialized view on last page", zap.String("viewName", viewName), zap.Error(result.Error))
			}
		}
	}

	// fill in ObjectName; a lookup failure is logged and the events still return
	if err := enrichEventsWithObjectInfo(c.Request().Context(), h.DB, *records, h.Logger); err != nil {
		h.Logger.Error("handler error: error enriching events with object info", zap.Error(err))
	}

	// encode the concrete []Event. CreateResponse copies each event into
	// an interface{} slice, and json then re-reflects every element
	w := c.Response()
	w.Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(struct {
		Meta   apiserver_lib.Meta
		Type   string
		Data   []v0.Event
		Status apiserver_lib.Status
	}{
		Meta:   apiserver_lib.Meta{Pagination: *pagination, ObjectCount: returnedCount},
		Type:   objectType,
		Data:   *records,
		Status: apiserver_lib.Status{Code: http.StatusOK, Message: http.StatusText(http.StatusOK)},
	})
}

// enrichEventsWithObjectInfo sets ObjectName on each event from the subject's
// current name. Events whose lookup fails are left with a nil ObjectName.
func enrichEventsWithObjectInfo(ctx context.Context, db *gorm.DB, events []v0.Event, log *zap.Logger) error {
	// no events to enrich - nothing to do
	if len(events) == 0 {
		return nil
	}

	// group object ids by their qualified type so the name lookup can
	// fan out one batch per type (each batch hits either core SQL or
	// one module HTTP endpoint - see GetObjectNames)
	idsByType := map[string]map[uint]struct{}{}
	for _, e := range events {
		if e.ObjectType == nil || e.ObjectID == nil {
			continue
		}
		if idsByType[*e.ObjectType] == nil {
			idsByType[*e.ObjectType] = map[uint]struct{}{}
		}
		idsByType[*e.ObjectType][*e.ObjectID] = struct{}{}
	}

	// resolve names one qualified type at a time
	namesByType := make(map[string]map[uint]string, len(idsByType))
	for typ, idSet := range idsByType {
		ids := make([]uint, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}

		resolved, err := resolveNamesWithCache(ctx, db, typ, ids)
		if err != nil {
			log.Error("failed to resolve object names", zap.String("objectType", typ), zap.Error(err))
			// keep names already found in cache when the remaining lookup fails
			if len(resolved) > 0 {
				namesByType[typ] = resolved
			}
			continue
		}
		namesByType[typ] = resolved
	}

	// project the resolved name onto each event row when available;
	// events whose subject lookup failed keep ObjectName=nil
	for i := range events {
		e := &events[i]
		if e.ObjectType == nil || e.ObjectID == nil {
			continue
		}
		names, ok := namesByType[*e.ObjectType]
		if !ok {
			continue
		}
		if name, ok := names[*e.ObjectID]; ok {
			e.ObjectName = util.Ptr(name)
		}
	}

	return nil
}

// resolveNamesWithCache returns names for ids of one object type, using the
// in-process cache and fetching only the misses. A fetch error still returns cache hits.
func resolveNamesWithCache(ctx context.Context, db *gorm.DB, objectType string, ids []uint) (map[uint]string, error) {
	resolved := make(map[uint]string, len(ids))
	misses := make([]uint, 0, len(ids))
	for _, id := range ids {
		if cached, ok := moduleNameCache.Get(objectType, id); ok {
			resolved[id] = cached
			continue
		}
		misses = append(misses, id)
	}

	if len(misses) == 0 {
		return resolved, nil
	}

	fetched, err := GetObjectNames(ctx, db, objectType, misses, true)
	if err != nil {
		return resolved, err
	}
	for id, name := range fetched {
		resolved[id] = name
		moduleNameCache.Put(objectType, id, name)
	}

	return resolved, nil
}

// eventSubjectGroup is a set of object ids that share one fully qualified type.
type eventSubjectGroup struct {
	QualifiedType string
	IDs           []uint
}

// eventSubjectTypes returns the distinct fully qualified object types present
// on live event rows, in name order. Values come off the event row, so each
// one is held to qualifiedTypePattern before it is returned.
func eventSubjectTypes(db *gorm.DB) ([]string, error) {
	var rawTypes []string
	if err := db.Model(&v0.Event{}).
		Distinct().
		Order("object_type ASC").
		Pluck("object_type", &rawTypes).Error; err != nil {
		return nil, fmt.Errorf("failed to read event subject types: %w", err)
	}

	types := make([]string, 0, len(rawTypes))
	for _, rawType := range rawTypes {
		if qualifiedTypePattern.MatchString(rawType) {
			types = append(types, rawType)
		}
	}

	return types, nil
}

// resolveSubjectsByName returns the (type, ids) groups whose subject name
// equals name. A name is unique only within a type. A type that fails
// lookup is skipped so an unreachable owner narrows the answer rather
// than failing the request.
func resolveSubjectsByName(
	db *gorm.DB,
	candidateTypes []string,
	name string,
	log *zap.Logger,
) []eventSubjectGroup {
	groups := make([]eventSubjectGroup, 0, len(candidateTypes))

	for _, qualifiedType := range candidateTypes {
		ids, err := GetObjectIDsByName(db, qualifiedType, name)
		if err != nil {
			log.Error(
				"failed to resolve object name for name filter",
				zap.String("objectType", qualifiedType),
				zap.String("objectName", name),
				zap.Error(err),
			)

			continue
		}
		if len(ids) == 0 {
			continue
		}

		groups = append(groups, eventSubjectGroup{QualifiedType: qualifiedType, IDs: ids})
	}

	return groups
}

// resolveSubjectsByNamePrefix returns the (type, ids) groups whose subject
// name starts with prefix. Event rows hold type and id only, so the match
// reads those ids, resolves names, and compares the prefix in Go. A type
// whose names cannot be resolved is skipped.
func resolveSubjectsByNamePrefix(
	ctx context.Context,
	db *gorm.DB,
	candidateTypes []string,
	prefix string,
	log *zap.Logger,
) ([]eventSubjectGroup, error) {
	groups := make([]eventSubjectGroup, 0, len(candidateTypes))

	for _, qualifiedType := range candidateTypes {
		var ids []uint
		if err := db.Model(&v0.Event{}).
			Where("object_type = ?", qualifiedType).
			Distinct().
			Order("object_id ASC").
			Pluck("object_id", &ids).Error; err != nil {
			return nil, fmt.Errorf("failed to read subject ids for %s: %w", qualifiedType, err)
		}
		if len(ids) == 0 {
			continue
		}

		names, err := resolveNamesWithCache(ctx, db, qualifiedType, ids)
		if err != nil {
			log.Error(
				"failed to resolve object names for name prefix filter",
				zap.String("objectType", qualifiedType),
				zap.Error(err),
			)
		}

		matched := make([]uint, 0, len(ids))
		for _, id := range ids {
			if strings.HasPrefix(names[id], prefix) {
				matched = append(matched, id)
			}
		}
		if len(matched) == 0 {
			continue
		}

		groups = append(groups, eventSubjectGroup{QualifiedType: qualifiedType, IDs: matched})
	}

	return groups, nil
}
