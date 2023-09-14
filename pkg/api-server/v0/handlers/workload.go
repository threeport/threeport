package handlers

import (
	"errors"

	echo "github.com/labstack/echo/v4"
	gorm "gorm.io/gorm"
	"gorm.io/gorm/clause"

	iapi "github.com/threeport/threeport/pkg/api-server/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
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

// @Summary gets a workload event set by workload instance ID.
// @Description Gets a set of workload events by workload instance ID from the database.
// @ID get-workloadEventSet
// @Accept json
// @Produce json
// @Param workloadInstanceID path int true "workloadInstanceID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/workload-event-sets/{workloadInstanceID} [get]
func (h Handler) GetWorkloadEventSet(c echo.Context) error {
	objectType := v0.ObjectTypeWorkloadEvent
	workloadInstanceID := c.Param("workloadInstanceID")

	var totalCount int64
	if result := h.DB.Model(&v0.WorkloadEvent{}).Where("workload_instance_id = ?", workloadInstanceID).Count(&totalCount); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	records := &[]v0.WorkloadEvent{}
	if result := h.DB.Order("ID asc").Where("workload_instance_id = ?", workloadInstanceID).Find(records); result.Error != nil {
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, *records, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// @Summary deletes a workload event set by workload instance ID.
// @Description Deletes a set of workload events by workload instance ID from the database.
// @ID delete-workloadEventSet
// @Accept json
// @Produce json
// @Param workloadInstanceID path int true "workloadInstanceID"
// @Success 200 {object} v0.Response "OK"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/workload-event-sets/{workloadInstanceID} [delete]
func (h Handler) DeleteWorkloadEventSet(c echo.Context) error {
	objectType := v0.ObjectTypeWorkloadEvent
	workloadInstanceID := c.Param("workloadInstanceID")
	var workloadEvents []v0.WorkloadEvent
	if result := h.DB.Clauses(clause.Returning{}).Where(
		"workload_instance_id = ?",
		workloadInstanceID,
	).Delete(&workloadEvents); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return iapi.ResponseStatus404(c, nil, result.Error, objectType)
		}
		return iapi.ResponseStatus500(c, nil, result.Error, objectType)
	}

	response, err := v0.CreateResponse(nil, workloadEvents, objectType)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}
