package handlers

import (
	"fmt"
	"reflect"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Handler is the main handler for the API server.  It contains the database connection,
// NATS connection, JetStream context, and logger.
type Handler struct {
	DB     *gorm.DB
	NC     *nats.Conn
	JS     nats.JetStreamContext
	Logger *zap.Logger
}

// New returns a new Handler.
func New(db *gorm.DB, nc *nats.Conn, rc nats.JetStreamContext, logger *zap.Logger) Handler {
	return Handler{db, nc, rc, logger}
}

// CreateMaterializedView creates a materialized view for a given object type and returns the
// view name and query ID. The views are used for result pagination in handlers that support pagination.
func (h Handler) CreateMaterializedView(queryTable string) (string, string, error) {
	// create the materialized view name
	viewName, queryId := GenerateMaterializedViewName()

	// create the materialized view
	createView := fmt.Sprintf("CREATE MATERIALIZED VIEW %s AS SELECT * FROM %s ORDER BY ID ASC", viewName, queryTable)
	if result := h.DB.Exec(createView); result.Error != nil {
		return "", "", fmt.Errorf("handler error: error creating materialized view with name %s: %w", viewName, result.Error)
	}

	// create an ID index on the materialized view
	createIdIndex := fmt.Sprintf("CREATE INDEX ON %s (ID)", viewName)
	if result := h.DB.Exec(createIdIndex); result.Error != nil {
		return "", "", fmt.Errorf("handler error: error creating ID index: %w", result.Error)
	}

	return viewName, queryId, nil
}

// GetMaterializedViewName finds the name of the materialized view created for a given pagination query ID.
func (h Handler) GetMaterializedViewName(queryId string) (string, error) {
	// find the materialized view name by query ID
	viewQuery := fmt.Sprintf("SELECT table_name FROM information_schema.tables WHERE table_type = 'VIEW' AND table_name LIKE 'paginated_%%_%s'", queryId)
	var viewName string
	if result := h.DB.Raw(viewQuery).Scan(&viewName); result.Error != nil {
		return "", fmt.Errorf("handler error: error finding materialized view: %w", result.Error)
	}

	return viewName, nil
}

// GetMaterializedViewRecords fetches records from a materialized view based on a cursor
// for pagination.  This function uses reflection to work with any slice type.  It will
// use a cursor to fetch the next page of results if a cursor is included in the page
// request parameters.
func (h Handler) GetMaterializedViewRecords(
	records interface{},
	viewName string,
	pageParams *apiserver_lib.PageRequestParams,
) (int64, error) {
	var recordsQuery string
	if pageParams.Cursor == 0 {
		recordsQuery = fmt.Sprintf("SELECT * FROM %s ORDER BY ID ASC LIMIT %d", viewName, pageParams.Limit)
	} else {
		recordsQuery = fmt.Sprintf("SELECT * FROM %s WHERE ID > %d ORDER BY ID ASC LIMIT %d", viewName, pageParams.Cursor, pageParams.Limit)
	}
	if result := h.DB.Raw(recordsQuery).Find(records); result.Error != nil {
		return 0, fmt.Errorf("handler error: error finding records: %w", result.Error)
	}

	// Use reflection to get the length of the slice
	recordsValue := reflect.ValueOf(records)
	if recordsValue.Kind() == reflect.Ptr {
		recordsValue = recordsValue.Elem()
	}
	if recordsValue.Kind() != reflect.Slice {
		return 0, fmt.Errorf("records must be a slice")
	}

	return int64(recordsValue.Len()), nil
}

// GenerateMaterializedViewName generates a standardized, unique materialized view name.
// The name contains the current timestamp for that is used to clean up materialized views
// that have passed a TTL. The query ID is used by the client to identify the view for
// subsequent pages of results.
func GenerateMaterializedViewName() (string, string) {
	queryId := util.RandomAlphaNumericString(16)
	viewName := fmt.Sprintf(
		"%s_%s_%s",
		apiserver_lib.PaginationViewPrefix,
		time.Now().Format("20060102150405"),
		queryId,
	)

	return viewName, queryId
}
