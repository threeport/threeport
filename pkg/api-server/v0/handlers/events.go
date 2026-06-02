package handlers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	echo "github.com/labstack/echo/v4"
	zap "go.uber.org/zap"
	"gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// eventJoinAttachedObjectReferenceClause is the inner join from
// v0_events to v0_attached_object_references on the polymorphic
// columns the reference table uses to point at events. The `?`
// placeholder stands in for the event's fully qualified type. Used
// directly by gorm .Joins() chains; raw-SQL paths substitute the
// literal in via strings.Replace.
const eventJoinAttachedObjectReferenceClause = `INNER JOIN v0_attached_object_references
	ON v0_attached_object_references.attached_object_type = ?
	AND v0_attached_object_references.attached_object_id = v0_events.id`

// JoinEventsToAttachedObjectReferences chains the join above plus the
// soft-delete predicate on the reference rows, so live events and live
// references come back together.
func JoinEventsToAttachedObjectReferences(query *gorm.DB, fullyQualifiedEventType string) *gorm.DB {
	return query.
		Joins(eventJoinAttachedObjectReferenceClause, fullyQualifiedEventType).
		Where(apiserver_lib.LiveRowsFilter("v0_attached_object_references"))
}

// @Summary gets all events joined with attached object references.
// @Description Get all events joined with attached object references from the Threeport database.
// @ID get-v0-events-join-attached-object-references
// @Accept json
// @Produce json
// @Param objectid query string false "filter events by object ID"
// @Param objecttypename query string false "filter events by object type name (with objectname); CamelCase Go TypeName like 'KubernetesWorkloadInstance'"
// @Param objectversion query string false "narrow objecttypename match to one version (e.g. 'v0')"
// @Param objectnamespace query string false "narrow objecttypename match to one api namespace (e.g. 'threeport.io')"
// @Param objectname query string false "filter events by object name (with objecttypename)"
// @Success 200 {object} v0.Response "OK"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/events-join-attached-object-references [GET]
func (h Handler) GetEventsJoinAttachedObjectReferences(c echo.Context) error {
	objectType := v0.ObjectTypeEvent

	// fully qualified type of Event - written to AOR.AttachedObjectType when an event
	// is recorded, and used here as the join key between v0_events
	// and v0_attached_object_references
	fullyQualifiedEventType := (&v0.Event{}).GetFullyQualifiedType()

	// get pagination parameters
	pageParams, err := c.(*apiserver_lib.CustomContext).GetPaginationParams()
	if err != nil {
		return apiserver_lib.ResponseStatus400(c, pageParams, err, objectType)
	}

	// bind filter
	var filter v0.Event
	if err := c.Bind(&filter); err != nil {
		h.Logger.Error("handler error: error binding filter", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, pageParams, err, objectType)
	}

	// collect object IDs to filter on. The accepted shapes are:
	//   - nothing supplied                  -> return every event
	//   - objecttypename + objectid         -> filter by id under type
	//   - objecttypename + objectname       -> resolve name to id(s) under type
	// Any other combination is rejected: type is always required when
	// filtering, and id+name together is ambiguous.
	targetTypeName := c.QueryParam("objecttypename")
	targetVersion := c.QueryParam("objectversion")
	targetNamespace := c.QueryParam("objectnamespace")
	targetName := c.QueryParam("objectname")
	directObjectId := c.QueryParam("objectid")

	var ids []uint
	var fullyQualifiedTypes []string

	// resolveQualifiedTypes turns the targetTypeName into the set of
	// fully qualified types that match it, optionally narrowed by namespace/version.
	// shared by the type+id and type+name branches below so both
	// constrain the AOR subject filter to the right type set.
	resolveQualifiedTypes := func() ([]string, error) {
		types, err := apiserver_lib.GetObjectTypes(h.DB, targetTypeName)
		if err != nil {
			return nil, err
		}
		return apiserver_lib.FilterQualifiedTypes(types, targetNamespace, targetVersion), nil
	}
	switch {
	case targetTypeName == "" && targetName == "" && directObjectId == "":
		// no filter supplied; fall through to the unfiltered query

	case targetTypeName == "":
		// caller supplied a name or id but no type. type is the only
		// disambiguator for a polymorphic lookup, so this is rejected.
		return apiserver_lib.ResponseStatus400(
			c, pageParams,
			errors.New("objecttypename is required when filtering by objectid or objectname"),
			objectType,
		)

	case directObjectId != "" && targetName != "":
		// type + id + name is ambiguous - which filter wins?
		return apiserver_lib.ResponseStatus400(
			c, pageParams,
			errors.New("provide either objectid or objectname, not both"),
			objectType,
		)

	case directObjectId != "":
		// type + id. The type still has to resolve so the subject
		// filter can pin object_type; without it, an unrelated type
		// that happens to share the id leaks in. Multi-type bare
		// kinds surface every (resolved type, id) pair - narrow with
		// objectnamespace / objectversion.
		parsed, err := strconv.ParseUint(directObjectId, 10, 64)
		if err != nil {
			return apiserver_lib.ResponseStatus400(c, pageParams,
				fmt.Errorf("invalid objectid %q: %w", directObjectId, err), objectType)
		}
		ids = []uint{uint(parsed)}

		types, lookupErr := resolveQualifiedTypes()
		if lookupErr != nil {
			h.Logger.Error("handler error: error looking up object types", zap.Error(lookupErr))
			return apiserver_lib.ResponseStatus400(c, pageParams, lookupErr, objectType)
		}
		if len(types) == 0 {
			return apiserver_lib.ResponseStatus404(c, pageParams,
				fmt.Errorf("kind %q is not registered (or no version/namespace match)", targetTypeName), objectType)
		}
		fullyQualifiedTypes = types

	case targetName != "":
		// type + name - look up every fully qualified type that matches the type name,
		// then look up the named object under each
		types, lookupErr := resolveQualifiedTypes()
		if lookupErr != nil {
			h.Logger.Error("handler error: error looking up object types", zap.Error(lookupErr))
			return apiserver_lib.ResponseStatus400(c, pageParams, lookupErr, objectType)
		}
		if len(types) == 0 {
			return apiserver_lib.ResponseStatus404(c, pageParams,
				fmt.Errorf("kind %q is not registered (or no version/namespace match)", targetTypeName), objectType)
		}
		fullyQualifiedTypes = types

		// look up the named object across every matched fully qualified type; each
		// fully qualified type may yield zero or more ids - name uniqueness is not
		// enforced at the database level
		for _, fqt := range fullyQualifiedTypes {
			moreIds, lookupErr := GetObjectIDsByName(h.DB, fqt, targetName)
			if lookupErr == nil {
				ids = append(ids, moreIds...)
			}
		}
		if len(ids) == 0 {
			return apiserver_lib.ResponseStatus404(c, pageParams,
				fmt.Errorf("no object found with name %q for kind %q", targetName, targetTypeName), objectType)
		}

	default:
		// caller supplied type alone (no id, no name) - we don't
		// support type-only filtering, so report the supported shapes
		return apiserver_lib.ResponseStatus400(
			c, pageParams,
			errors.New("must provide either objecttypename + objectid, or objecttypename + objectname"),
			objectType,
		)
	}

	// pagination state is built up across the branches below and read
	// into the final response Meta
	pagination := new(apiserver_lib.Pagination)
	pagination.Limit = pageParams.Limit

	records := &[]v0.Event{}
	var returnedCount int64

	// apply the subject filter only when ids were supplied. The
	// filter is the Cartesian product (object_type IN types AND
	// object_id IN ids) - intentional, so a multi-type bare kind
	// surfaces every (resolved type, id) pair instead of forcing
	// namespace disambiguation up front.
	applyObjectIdFilter := func(query *gorm.DB) *gorm.DB {
		if len(ids) == 0 {
			return query
		}
		return query.
			Where("v0_attached_object_references.object_type IN ?", fullyQualifiedTypes).
			Where("v0_attached_object_references.object_id IN ?", ids)
	}

	switch {
	case pageParams.QueryId == "":
		// first-page request: no QueryId means the client is asking
		// for the start of a fresh result set, not a continuation

		// count total objects to decide whether pagination is needed.
		// The JOIN matches each event row to its reference row on
		// attached_object_type + attached_object_id. The soft-delete
		// predicate is explicit; gorm's automatic deleted_at filter
		// does not apply to raw .Joins() clauses.
		var totalCount int64
		countQuery := JoinEventsToAttachedObjectReferences(
			h.DB.Model(&v0.Event{}),
			fullyQualifiedEventType,
		)
		if result := applyObjectIdFilter(countQuery).Where(&filter).Count(&totalCount); result.Error != nil {
			h.Logger.Error("handler error: error counting objects", zap.Error(result.Error))
			return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
		}

		// total greater than the limit means the client will need to
		// page through the result set; HasMore signals that
		pagination.HasMore = totalCount > pagination.Limit

		switch pagination.HasMore {
		case false:
			// small result set: skip the materialized view machinery
			// and return everything in one shot. Same JOIN shape and
			// soft-delete predicate as the count query above.
			findQuery := JoinEventsToAttachedObjectReferences(
				h.DB.Order("ID asc"),
				fullyQualifiedEventType,
			)
			if result := applyObjectIdFilter(findQuery).Where(&filter).Find(records); result.Error != nil {
				h.Logger.Error("handler error: error finding objects", zap.Error(result.Error))
				return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
			}
			returnedCount = int64(len(*records))

		case true:
			// large result set: materialize the join so subsequent
			// cursor-based page requests can scan a stable view
			// instead of re-running the join each time
			viewName, queryId := GenerateMaterializedViewName()

			// base WHERE excludes soft-deleted events and reference
			// rows; raw SQL doesn't pick up gorm's deleted_at
			// scoping. Subject filters (when supplied) AND on after.
			whereClause := " WHERE " + apiserver_lib.LiveRowsFilter("v0_events", "v0_attached_object_references")
			if len(ids) > 0 {
				// constrain both object_type and object_id; id alone
				// would let unrelated types with the same id leak in
				typeStrs := make([]string, len(fullyQualifiedTypes))
				for i, t := range fullyQualifiedTypes {
					typeStrs[i] = fmt.Sprintf("'%s'", t)
				}
				idStrs := make([]string, len(ids))
				for i, id := range ids {
					idStrs[i] = fmt.Sprintf("'%d'", id)
				}
				whereClause += fmt.Sprintf(
					" AND v0_attached_object_references.object_type IN (%s) AND v0_attached_object_references.object_id IN (%s)",
					strings.Join(typeStrs, ", "),
					strings.Join(idStrs, ", "),
				)
			}

			// build and execute the CREATE MATERIALIZED VIEW. Raw SQL,
			// so substitute the literal type into the shared join
			// clause instead of binding via a gorm placeholder.
			joinClause := strings.Replace(
				eventJoinAttachedObjectReferenceClause,
				"?",
				fmt.Sprintf("'%s'", fullyQualifiedEventType),
				1,
			)
			createView := fmt.Sprintf(`
				CREATE MATERIALIZED VIEW %s AS
				SELECT v0_events.*
				FROM v0_events
				%s
				%s
				ORDER BY v0_events.id ASC
			`,
				viewName,
				joinClause,
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

			// fetch the first page off the new materialized view
			query := fmt.Sprintf("SELECT * FROM %s ORDER BY ID ASC LIMIT %d", viewName, pageParams.Limit)
			if result := h.DB.Raw(query).Find(records); result.Error != nil {
				h.Logger.Error("handler error: error finding objects", zap.Error(result.Error))
				return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
			}
			returnedCount = int64(len(*records))

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
		// continuation request: client gave a QueryId+Cursor pair, so
		// resume from the materialized view we created in a prior call

		// use the query ID to find the materialized view name (the view
		// name is deterministic from the queryId)
		viewName, err := h.GetMaterializedViewName(pageParams.QueryId)
		if err != nil {
			h.Logger.Error("handler error: error finding materialized view", zap.Error(err))
			return apiserver_lib.ResponseStatus500(c, pageParams, err, objectType)
		}

		// preserve the queryId across pages so the client keeps using
		// the same view for subsequent continuation requests
		pagination.QueryId = pageParams.QueryId

		// fetch the next page from the view starting just past the
		// previous cursor. the ID index built at create-time keeps
		// this O(limit) rather than O(view size)
		recordsQuery := fmt.Sprintf("SELECT * FROM %s WHERE ID > %d ORDER BY ID ASC LIMIT %d", viewName, pageParams.Cursor, pageParams.Limit)
		if result := h.DB.Raw(recordsQuery).Find(records); result.Error != nil {
			h.Logger.Error("handler error: error finding records", zap.Error(result.Error))
			return apiserver_lib.ResponseStatus500(c, pageParams, result.Error, objectType)
		}

		returnedCount = int64(len(*records))

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
	}

	// enrich records with attached object reference fields and resolved
	// object names; failures are logged so events still come back when
	// resolution can't fully complete
	if err := enrichEventsWithObjectInfo(h.DB, *records, h.Logger); err != nil {
		h.Logger.Error("handler error: error enriching events with object info", zap.Error(err))
	}

	// construct response
	response, err := apiserver_lib.CreateResponse(
		&apiserver_lib.Meta{
			Pagination:  *pagination,
			ObjectCount: returnedCount,
		},
		*records,
		objectType,
	)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, pageParams, err, objectType)
	}

	return apiserver_lib.ResponseStatus200(c, *response)
}

// enrichEventsWithObjectInfo populates ObjectType, ObjectID, and ObjectName
// on each event from the joined attached object reference and a per-type
// batched name lookup.
func enrichEventsWithObjectInfo(db *gorm.DB, events []v0.Event, log *zap.Logger) error {
	// no events to enrich - nothing to do
	if len(events) == 0 {
		return nil
	}

	// collect distinct event ids to look up their AOR rows via the
	// (attached_object_type, attached_object_id) composite index
	eventIDs := make([]uint, 0, len(events))
	seen := map[uint]struct{}{}
	for _, e := range events {
		// skip events without an ID (shouldn't happen for persisted rows
		// but avoids a nil deref if it does)
		if e.ID == nil {
			continue
		}
		// dedupe: an event id may appear more than once in events when
		// pagination retries land overlapping pages
		if _, ok := seen[*e.ID]; ok {
			continue
		}
		seen[*e.ID] = struct{}{}
		eventIDs = append(eventIDs, *e.ID)
	}

	// nothing left after the ID dedupe pass; bail before issuing the AOR query
	if len(eventIDs) == 0 {
		return nil
	}

	// load every AOR row where this event is the attached side. The
	// (attached_object_type, attached_object_id) composite uniquely
	// identifies one AOR per event id.
	fullyQualifiedEventType := (&v0.Event{}).GetFullyQualifiedType()
	var aors []v0.AttachedObjectReference
	if err := db.
		Where("attached_object_type = ? AND attached_object_id IN ?", fullyQualifiedEventType, eventIDs).
		Find(&aors).Error; err != nil {
		return fmt.Errorf("failed to load attached object references: %w", err)
	}

	// build an event-id -> AOR map so the per-event projection step
	// below is O(1) per lookup
	aorByEventID := make(map[uint]v0.AttachedObjectReference, len(aors))
	for _, a := range aors {
		if a.AttachedObjectID != nil {
			aorByEventID[*a.AttachedObjectID] = a
		}
	}

	// project AOR.ObjectType and AOR.ObjectID onto each event row
	// (these are gorm:"-" projection-only fields on the Event struct)
	for i := range events {
		e := &events[i]
		if e.ID == nil {
			continue
		}
		a, ok := aorByEventID[*e.ID]
		if !ok {
			// event has no matching AOR; leave the projection fields nil
			// and let the response render id-only when shown
			continue
		}
		e.ObjectType = a.ObjectType
		e.ObjectID = a.ObjectID
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

	// dispatch each type to core sql or module http and collect
	// resolved names; failures are logged so events still come back
	// (rendered id-only) when name resolution fails for some types
	namesByType := make(map[string]map[uint]string, len(idsByType))
	for typ, idSet := range idsByType {
		ids := make([]uint, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		names, err := GetObjectNames(db, typ, ids, true)
		if err != nil {
			log.Error("failed to resolve object names", zap.String("objectType", typ), zap.Error(err))
			continue
		}
		namesByType[typ] = names
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
