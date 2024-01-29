package handlers

import (
	"errors"
	"fmt"

	echo "github.com/labstack/echo/v4"
	gorm "gorm.io/gorm"

	iapi "github.com/threeport/threeport/pkg/api-server/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// AddWorkloadResourceDefinitions adds a new  set of workload resource definitions.
// @Summary adds a new set of workload resource definitions.
// @Description Add a set of new workload resource definition to the Threeport database.
// @ID add-workloadResourceDefinitions
// @Accept  json
// @Produce  json
// @Param   workloadResourceDefinitions	body	[]v0.WorkloadResourceDefinition	true	"WorkloadResourceDefinition object array"
// @Success 201 {object} v0.Response	"Created"
// @Failure 400 {object} v0.Response	"Bad Request"
// @Failure 500 {object} v0.Response	"Internal Server Error"
// @Router /v0/workload-resource-definition-sets [post]
func (h Handler) AddWorkloadResourceDefinitions(c echo.Context) error {
	objectType := v0.ObjectTypeWorkloadResourceDefinition
	var workloadResourceDefinitions []v0.WorkloadResourceDefinition

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType, v0.WorkloadResourceDefinition{}); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&workloadResourceDefinitions); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, workloadResourceDefinitions, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// create all workload resource definitions or none at all
	var createdWRDs []v0.WorkloadResourceDefinition
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		for _, wrd := range workloadResourceDefinitions {
			if result := h.DB.Create(&wrd); result.Error != nil {
				return result.Error
			}
			createdWRDs = append(createdWRDs, wrd)
		}

		return nil
	})
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	response, err := v0.CreateResponse(nil, createdWRDs, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// @Summary deletes workload events by query parameter.
// @Description Deletes workload events by query parameter from the database.
// @ID delete-workloadEvents
// @Accept json
// @Produce json
// @Param name query string false "workload event search by name"
// @Success 200 {object} v0.Response "OK"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/workload-events [DELETE]
func (h Handler) DeleteWorkloadEvents(c echo.Context) error {
	objectType := v0.ObjectTypeWorkloadEvent
	params, err := c.(*iapi.CustomContext).GetPaginationParams()
	if err != nil {
		return iapi.ResponseStatus400(c, &params, err, objectType)
	}

	// ensure query parameters are present to prevent client from deleting all
	// workload events by mistake
	queryParams := c.QueryParams()
	if len(queryParams) != 1 {
		err := errors.New("must provide one - and only one - query parameter when deleting multiple workload events")
		return iapi.ResponseStatus403(c, &params, err, objectType)
	}

	// ensure workload events are deleted by workload or helm workload instance
	// ID
	validQueryKeys := []string{"workloadinstanceid", "helmworkloadinstanceid"}
	for k, _ := range queryParams {
		if !util.StringSliceContains(validQueryKeys, k, false) {
			//if strings.ToLower(k) != "helmworkloadinstanceid" {
			err := fmt.Errorf("can only delete multiple workload events using query parameter keys %s", validQueryKeys)
			return iapi.ResponseStatus403(c, &params, err, objectType)
		}
	}

	var filter v0.WorkloadEvent
	if err := c.Bind(&filter); err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	var totalCount int64
	workloadEvents := &[]v0.WorkloadEvent{}
	if result := h.DB.Where(&filter).Find(workloadEvents).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	// return 404 if no matches found for query parameter
	if len(*workloadEvents) == 0 {
		return iapi.ResponseStatus404(c, nil, client.ErrorObjectNotFound, objectType)
	}

	if result := h.DB.Delete(workloadEvents); result.Error != nil {
		return iapi.ResponseStatus500(c, &params, result.Error, objectType)
	}

	response, err := v0.CreateResponse(v0.CreateMeta(params, totalCount), *workloadEvents, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, &params, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}
