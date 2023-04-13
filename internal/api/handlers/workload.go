package handlers

import (
	"errors"

	echo "github.com/labstack/echo/v4"
	gorm "gorm.io/gorm"

	iapi "github.com/threeport/threeport/internal/api"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// AddWorkloadResourceDefinitions adds a new set of workload resource definitions.
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
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
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

	response, err := v0.CreateResponse(nil, createdWRDs)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus201(c, *response)
}

// UpdateWorkloadResourceDefinitions updates a set of workload resource definitions.
// @Summary updates a set of workload resource definitions.
// @Description Update a set of existing workload resource definitions in the Threeport database.
// @ID update-workloadResourceDefinitions
// @Accept  json
// @Produce  json
// @Param   workloadResourceDefinitions	body	[]v0.WorkloadResourceDefinition	true	"WorkloadResourceDefinition object array"
// @Success 200 {object} v0.Response	"OK"
// @Failure 400 {object} v0.Response	"Bad Request"
// @Failure 500 {object} v0.Response	"Internal Server Error"
// @Router /v0/workload-resource-definition-sets [put]
func (h Handler) UpdateWorkloadResourceDefinitions(c echo.Context) error {
	objectType := v0.ObjectTypeWorkloadResourceDefinition
	var workloadResourceDefinitions []v0.WorkloadResourceDefinition

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&workloadResourceDefinitions); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, workloadResourceDefinitions, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// update all workload resource definitions or none at all
	var updatedWRDs []v0.WorkloadResourceDefinition
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		for _, wrd := range workloadResourceDefinitions {
			if result := h.DB.Updates(&wrd); result.Error != nil {
				return result.Error
			}
			updatedWRDs = append(updatedWRDs, wrd)
		}

		return nil
	})
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	response, err := v0.CreateResponse(nil, updatedWRDs)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus200(c, *response)
}

// DeleteWorkloadResourceDefinitions deletes a set of workload resource definitions.
// @Summary deletes a set of workload resource definitions.
// @Description Delete a set of existing workload resource definitions in the Threeport database.
// @ID update-workloadResourceDefinitions
// @Accept  json
// @Produce  json
// @Param   workloadResourceDefinitions	body	[]v0.WorkloadResourceDefinition	true	"WorkloadResourceDefinition object array"
// @Success 204 {object} v0.Response	"No Content"
// @Failure 400 {object} v0.Response	"Bad Request"
// @Failure 500 {object} v0.Response	"Internal Server Error"
// @Router /v0/workload-resource-definition-sets [delete]
func (h Handler) DeleteWorkloadResourceDefinitions(c echo.Context) error {
	objectType := v0.ObjectTypeWorkloadResourceDefinition
	var workloadResourceDefinitions []v0.WorkloadResourceDefinition

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := iapi.PayloadCheck(c, false, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&workloadResourceDefinitions); err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := iapi.ValidateBoundData(c, workloadResourceDefinitions, objectType); err != nil {
		return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// delete all workload resource definitions or none at all
	var deletedWRDs []v0.WorkloadResourceDefinition
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		for _, wrd := range workloadResourceDefinitions {
			if result := h.DB.Delete(&wrd); result.Error != nil {
				return result.Error
			}
			deletedWRDs = append(deletedWRDs, wrd)
		}

		return nil
	})
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	response, err := v0.CreateResponse(nil, deletedWRDs)
	if err != nil {
		return iapi.ResponseStatus500(c, nil, err, objectType)
	}

	return iapi.ResponseStatus204(c, *response)
}
