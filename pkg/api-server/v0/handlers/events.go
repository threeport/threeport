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

// objectNamespacePattern matches DNS-like api namespaces such as
// "sxalable.io" or "threeport.io". Anchored so the value can be safely
// interpolated into a LIKE clause on v0_events.object_type.
var objectNamespacePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*$`)

// objectVersionPattern matches api version tokens such as "v0" or
// "v1alpha1". Anchored so the value can be safely interpolated into a
// LIKE clause on v0_events.object_type.
var objectVersionPattern = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

// reasonPattern matches event Reason values, which are Go-identifier
// CamelCase tokens (e.g. "SuccessfulCreate", "Reconcile_Fail"). Anchored
// so the value can be safely interpolated into equality and LIKE
// predicates on v0_events.reason.
var reasonPattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// objectNamePattern matches object Name values, which are DNS-like
// tokens such as "myfleet2-fleet2-host2". Anchored so an objectnameprefix
// is held to the shape of a name fragment before it reaches the name
// resolvers.
var objectNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// qualifiedTypePattern matches a fully qualified object type in
// "<namespace>/<version>.<TypeName>" form. The subject-type scan behind
// objectnameprefix reads object_type off the event row, so every value
// it returns is held to this shape before any of them is interpolated
// into SQL text.
var qualifiedTypePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]*/[a-zA-Z0-9]+\.[a-zA-Z0-9]+$`)

// materializedViewThresholdFloor is the minimum total-count above which
// the event listing spins up a materialized view for cursor pagination.
// Below the floor (or below limit*10 when the caller asks for larger
// pages), return the whole result set in a single query and skip the
// CREATE MATERIALIZED VIEW / DROP MATERIALIZED VIEW round trip.
const materializedViewThresholdFloor = 5000

// boundEventFilterClause renders the event columns a client bound onto the
// filter struct as a raw SQL fragment plus its bind values, for the paginated
// branches that build their query as a string rather than through gorm. Each
// value stays a placeholder, so no caller input reaches the SQL text.
//
// Reason is left out: the handler reads the reason and reasonprefix query
// params directly and builds its own predicate for them. The time columns are
// left out because the query binder rejects time values, so they never arrive
// on a list request.
//
// The subject columns object_type and object_id are included because a client
// can bind either one straight onto the list request, alongside the
// objecttypename / objectid query params the handler resolves itself.
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

// GetEventsJoinAttachedObjectReferences lists events, filtered by the
// subject columns object_type and object_id on the event row. The exported
// name and the route path both name the attached object reference table and
// keep that spelling for compatibility, because clients call
// /v0/events-join-attached-object-references today.
//
// @Summary gets all events, filtered by subject.
// @Description Get events from the Threeport database, narrowed by the object_type and object_id columns each event row carries.
// @ID get-v0-events-join-attached-object-references
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
// @Router /v0/events-join-attached-object-references [GET]
func (h Handler) GetEventsJoinAttachedObjectReferences(c echo.Context) error {
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

	// collect the subject filter. Each of objecttypename, objectid,
	// objectname, and objectnameprefix narrows the listing on its own,
	// and objecttypename combines with any one of the other three:
	//   - nothing supplied         -> return every event
	//   - objecttypename           -> every event whose subject is that kind
	//   - objectid                 -> filter by id across every subject type
	//   - objectname               -> resolve the name across every subject type
	//   - objectnameprefix         -> resolve the prefix across every subject type
	//   - objecttypename + one     -> any of the three above, narrowed to that kind
	// objectid, objectname, and objectnameprefix are mutually exclusive:
	// an id names the subject directly and each name form resolves it, so
	// a request carrying two of them has no single answer.
	targetTypeName := c.QueryParam("objecttypename")
	targetVersion := c.QueryParam("objectversion")
	targetNamespace := c.QueryParam("objectnamespace")
	targetName := c.QueryParam("objectname")
	targetNamePrefix := c.QueryParam("objectnameprefix")
	directObjectId := c.QueryParam("objectid")
	targetReason := c.QueryParam("reason")
	targetReasonPrefix := c.QueryParam("reasonprefix")

	// validate the narrow-filter tokens that get interpolated into the
	// object_type LIKE clause below. The regexes reject anything
	// outside the DNS-like namespace / alphanumeric-version shape so
	// caller-supplied text cannot inject SQL.
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
	// reason and reasonprefix flow into equality / LIKE predicates on
	// v0_events.reason. The regex restricts the accepted alphabet to
	// Go-identifier CamelCase tokens so caller text cannot inject SQL
	// when interpolated into the raw-SQL pagination paths below.
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
	// objectname and objectnameprefix each resolve the subject by name,
	// and an id names the subject directly, so a request carries at most
	// one of the three.
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

	var ids []uint
	var fullyQualifiedTypes []string

	// resolveQualifiedTypes turns the targetTypeName into the set of
	// fully qualified types that match it, optionally narrowed by namespace/version.
	// shared by the type+id and type+name branches below so both
	// constrain the subject filter to the right type set.
	resolveQualifiedTypes := func() ([]string, error) {
		types, err := apiserver_lib.GetObjectTypes(h.DB, targetTypeName)
		if err != nil {
			return nil, err
		}
		return apiserver_lib.FilterQualifiedTypes(types, targetNamespace, targetVersion), nil
	}

	// buildNamespaceVersionPattern returns the LIKE pattern that narrows
	// object_type to the caller-supplied namespace and version.
	// Types are stored as "<namespace>/<version>.<TypeName>", so patterns
	// anchor on the slash and dot separators. Returns active=false when
	// neither filter is set so callers can skip the extra predicate.
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

	// nameMatchedSubjects pairs a fully qualified type with the ids under
	// it that a name filter selected. Pairing each type with its own ids
	// keeps an unrelated type that happens to share an id out of the
	// listing.
	var nameMatchedSubjects []eventSubjectGroup

	// candidateSubjectTypes returns the fully qualified types a filter
	// resolves against: the ones objecttypename names when it is
	// supplied, and otherwise every subject type the live event rows
	// carry. Both sets are narrowed by objectnamespace and
	// objectversion. The returned status is the response code to answer
	// with when the error is non-nil.
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

	// respondTypeLookup answers a candidateSubjectTypes failure, logging
	// the statuses that report a server-side fault rather than a bad
	// request.
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
		// a prefix spans types by design: a fleet, the instances
		// derived from it, and their workloads share a name prefix and
		// answer one query. objecttypename narrows the candidate set to
		// one kind when it is supplied.
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
		// a name is unique only within a type, so it resolves one type
		// at a time and each candidate type keeps the ids it yielded.
		// Without objecttypename the candidates are every subject type
		// the event rows carry, which is what lets a caller holding
		// only the name find its events.
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
		// object_id is a column on the event row, so an id filters the
		// row set on its own. A type alongside it resolves as well, so
		// the filter also pins object_type and an unrelated type that
		// happens to share the id stays out. Multi-type bare kinds
		// surface every (resolved type, id) pair - narrow with
		// objectnamespace / objectversion.
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
		// a bare kind on its own narrows the listing to every event
		// whose subject is one of the types it resolves to. Nothing
		// selects a subject within the kind, so every id under it is in
		// the answer.
		types, status, lookupErr := candidateSubjectTypes()
		if lookupErr != nil {
			return respondTypeLookup(status, lookupErr)
		}
		fullyQualifiedTypes = types

	default:
		// nothing selects a subject. A namespace or version supplied on
		// its own still narrows the row set through the object_type
		// LIKE predicate below; with neither, the listing is unfiltered.
	}

	// pagination state is built up across the branches below and read
	// into the final response Meta
	pagination := new(apiserver_lib.Pagination)
	pagination.Limit = pageParams.Limit

	records := &[]v0.Event{}
	var returnedCount int64

	// buildReasonRawWhere returns a raw-SQL predicate fragment for the
	// reason / reasonprefix filter, or ("", false) when neither is set.
	// Values are pre-validated against reasonPattern above, so they are
	// safe to interpolate into the returned literal.
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

	// buildRawWhere renders the WHERE that every raw-SQL pagination
	// branch below shares. The first page and its continuations have to
	// select from the same row set, so they build it from one place
	// rather than each assembling its own copy.
	//
	// The base WHERE excludes soft-deleted events, because raw SQL does
	// not pick up gorm's deleted_at scoping. Every value interpolated
	// here is either a fully qualified type this handler resolved or a
	// regex-validated pattern; values a client supplies travel
	// separately as bind parameters.
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
			// one OR group per matched type, each holding only the ids
			// resolved under that type. Both halves are handler-side
			// values: the type passed qualifiedTypePattern and the ids
			// are integers.
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
			// narrow object_type by qualified-type prefix so a
			// namespace-only or version-only filter still constrains
			// the row set
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

	// apply the subject filter when ids, a resolved type set, or a
	// namespace/version filter were supplied. A type set and ids
	// together form the Cartesian product
	// (object_type IN types AND object_id IN ids) - intentional, so a
	// multi-type bare kind surfaces every (resolved type, id) pair
	// instead of forcing namespace disambiguation up front. An id
	// supplied without a type constrains object_id alone. The
	// namespace/version half narrows object_type via a LIKE prefix so an
	// --api-group / --object-version call without a bare kind still
	// constrains the row set.
	applyObjectIdFilter := func(query *gorm.DB) *gorm.DB {
		if len(fullyQualifiedTypes) > 0 {
			query = query.Where("v0_events.object_type IN ?", fullyQualifiedTypes)
		}
		if len(ids) > 0 {
			query = query.Where("v0_events.object_id IN ?", ids)
		}
		if len(nameMatchedSubjects) > 0 {
			// one OR group per matched type, so the ids resolved under
			// one type match only that type's rows
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

	// boundClause holds the filter-struct predicates the raw-SQL
	// pagination branches share, with each caller value traveling
	// beside it as a bind parameter. Every read of a snapshot applies
	// it, so each page of one result set is drawn from the same rows.
	//
	// These predicates stay out of buildRawWhere, and so out of the
	// CREATE MATERIALIZED VIEW: a view definition takes no placeholders,
	// and interpolating caller text into it would put that text in the
	// SQL. A view read carries them instead, with the table prefix
	// stripped because the view exposes its columns unqualified.
	boundClause, boundValues := boundEventFilterClause(&filter)
	viewBoundClause := strings.ReplaceAll(boundClause, "v0_events.", "")

	switch {
	case pageParams.QueryId == "":
		// first-page request: no QueryId means the client is asking
		// for the start of a fresh result set, not a continuation

		// only spin up a materialized view when the result set is large
		// enough that keyset paging over a stable snapshot is worth the
		// CREATE MATERIALIZED VIEW cost. Under the threshold, return
		// everything in one shot and skip the view machinery entirely.
		// Threshold is max(limit*10, 5000): scales with client-requested
		// limit so a caller asking for larger pages still gets multi-page
		// behavior, and a hard floor keeps small result sets on the
		// single-shot path even when limit is low.
		threshold := pagination.Limit * 10
		if threshold < materializedViewThresholdFloor {
			threshold = materializedViewThresholdFloor
		}

		// probe the result set with a LIMIT threshold+1 fetch so the
		// pagination decision is made from returned row count instead
		// of a separate Count query. When the row count fits under the
		// threshold, serve the fetched records directly and skip the
		// materialized view path.
		findQuery := h.DB.Order("event_time ASC, id ASC").Limit(int(threshold) + 1)
		if result := applyObjectIdFilter(findQuery).Where(&filter).Find(records); result.Error != nil {
			h.Logger.Error("handler error: error finding objects", zap.Error(result.Error))
			return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
		}
		pagination.HasMore = int64(len(*records)) > threshold

		switch pagination.HasMore {
		case false:
			// small result set: everything already loaded in the probe
			// fetch above; return it in one shot
			returnedCount = int64(len(*records))

		case true:
			// large result set: pin a snapshot so subsequent cursor
			// pages see the same rows even under concurrent writes.
			// the two modes are peers: materialized-view mode
			// materializes the filtered rows into a fresh view;
			// as-of-system-time mode captures an HLC and re-runs the
			// query at that timestamp on every page.

			// the probe fetch above already loaded threshold+1 rows;
			// discard them so the snapshot path below refills from its
			// own query rather than appending to a partial result
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

				// re-run the query at the captured snapshot for the
				// first page. AS OF SYSTEM TIME sits between FROM and
				// WHERE (CRDB syntax); id ordering matches MV mode.
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
				// materialize the filtered rows so subsequent
				// cursor-based page requests can scan a stable view
				// instead of re-running the query each time
				viewName, queryId := GenerateMaterializedViewName()

				// build and execute the CREATE MATERIALIZED VIEW.
				// materialize in causal order so the view reads as the
				// sequence the events actually happened in, with id
				// breaking intra-second ties.
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

				// fetch the first page off the new materialized view.
				// The view definition carries the subject filters this
				// handler resolved; the filter-struct predicates apply
				// on the read, as bind values.
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

		// MV mode pages over a named view and drops it once the client
		// walks off the end. AOST mode has no view to drop, so this
		// stays empty and the drop below is skipped.
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

			// rebuild what the first page used so the tail of the
			// result set is scanned over the same row set
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

			// fetch the next page from the view starting just past the
			// previous cursor. the ID index built at create-time keeps
			// this O(limit) rather than O(view size). The filter-struct
			// predicates apply on this read the same way they apply on
			// the first-page read.
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

		// drop the materialized view inline the moment the client walks
		// off the end of the result set so the backing storage is freed
		// immediately, not deferred to the TTL sweeper. Failures here are
		// logged, not returned: the response body is already correct, and
		// the TTL sweeper still drops the view on its next pass. Only MV
		// mode reaches this; AOST mode leaves viewName empty.
		if !pagination.HasMore && viewName != "" {
			dropQuery := fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", viewName)
			if result := h.DB.Exec(dropQuery); result.Error != nil {
				h.Logger.Error("handler error: error dropping materialized view on last page", zap.String("viewName", viewName), zap.Error(result.Error))
			}
		}
	}

	// resolve each event's object name from its subject columns;
	// failures are logged so events still come back when resolution
	// can't fully complete.
	if err := enrichEventsWithObjectInfo(c.Request().Context(), h.DB, *records, h.Logger); err != nil {
		h.Logger.Error("handler error: error enriching events with object info", zap.Error(err))
	}

	// stream the response directly to the wire instead of round-tripping
	// through CreateResponse. CreateResponse builds a parallel []Object
	// slice by reflect-copying every event into an interface{}; on large
	// pages that boxing loop plus the follow-on per-element type reflection
	// inside encoding/json dominates the handler's tail latency.
	// Marshalling the concrete []v0.Event lets json cache the type once and
	// avoids the intermediate slice entirely.
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

// enrichEventsWithObjectInfo populates ObjectName on each event, resolving
// it from the ObjectType and ObjectID columns the event row carries through
// a per-type batched name lookup.
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

	// resolve one batch per type through the cache-backed lookup;
	// failures are logged so events still come back (rendered id-only)
	// when name resolution fails for some types.
	namesByType := make(map[string]map[uint]string, len(idsByType))
	for typ, idSet := range idsByType {
		ids := make([]uint, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}

		resolved, err := resolveNamesWithCache(ctx, db, typ, ids)
		if err != nil {
			log.Error("failed to resolve object names", zap.String("objectType", typ), zap.Error(err))
			// keep any cache-hit names for this type so partial
			// resolution still degrades better than id-only
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

// resolveNamesWithCache returns id->name for one object type, serving
// what the process-wide cache holds and dispatching the rest to core
// SQL or the owning module. Cache hits skip the resolver round trip, so
// a batch whose names are all cached issues no query at all. A resolver
// error returns the cache hits alongside it, so the caller keeps the
// names that did resolve.
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

// eventSubjectGroup pairs a fully qualified object type with the subject
// ids under it that a filter selected.
type eventSubjectGroup struct {
	QualifiedType string
	IDs           []uint
}

// eventSubjectTypes returns every distinct object_type the live event
// rows carry, in name order. An objectnameprefix that arrives without
// objecttypename resolves against this set, so one query reaches a
// fleet object and the children whose names extend its name.
//
// Values come off the event row, so each one is held to
// qualifiedTypePattern before it is returned.
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

// resolveSubjectsByName returns, per candidate type, the ids of the
// objects under it carrying name. A name is unique only within a type,
// so the lookup runs once per candidate type and each type keeps its own
// ids, which holds an unrelated type that happens to share an id out of
// the listing.
//
// A type whose lookup fails contributes no ids and the failure is
// logged, so an unreachable or deregistered owner narrows the answer
// rather than failing the request.
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

// resolveSubjectsByNamePrefix returns, per candidate type, the subject
// ids under it whose object name starts with prefix.
//
// An event row holds object_type and object_id, and the name comes from
// each type's resolver, so the match runs over the ids the event table
// already carries: one distinct-id read per type, then the same batched
// name lookup the read path uses, compared in Go. A type whose names
// cannot be resolved contributes no ids and the failure is logged, so an
// unreachable module narrows the answer rather than failing the request.
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
