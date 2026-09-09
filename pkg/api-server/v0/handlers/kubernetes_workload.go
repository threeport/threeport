package handlers

import (
	"errors"

	echo "github.com/labstack/echo/v4"
	zap "go.uber.org/zap"
	gorm "gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// AddKubernetesWorkloadResourceDefinitions adds a new set of kubernetes workload resource definitions.
// @Summary adds a new set of kubernetes workload resource definitions.
// @Description Add a set of new kubernetes workload resource definitions to the Threeport database.
// @ID add-kubernetesWorkloadResourceDefinitions
// @Accept  json
// @Produce  json
// @Param   kubernetesWorkloadResourceDefinitions	body	[]v0.KubernetesWorkloadResourceDefinition	true	"KubernetesWorkloadResourceDefinition object array"
// @Success 201 {object} v0.Response	"Created"
// @Failure 400 {object} v0.Response	"Bad Request"
// @Failure 500 {object} v0.Response	"Internal Server Error"
// @Router /v0/kubernetes-workload-resource-definition-sets [post]
func (h Handler) AddKubernetesWorkloadResourceDefinitions(c echo.Context) error {
	objectType := v0.ObjectTypeKubernetesWorkloadResourceDefinition
	var k8sWorkloadResourceDefinitions []v0.KubernetesWorkloadResourceDefinition

	// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
	if id, err := apiserver_lib.PayloadCheck(c, false, false, objectType, v0.KubernetesWorkloadResourceDefinition{}); err != nil {
		h.Logger.Error("handler error: error performing payload check", zap.Error(err))
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	if err := c.Bind(&k8sWorkloadResourceDefinitions); err != nil {
		h.Logger.Error("handler error: error binding object", zap.Error(err))
		return apiserver_lib.ResponseStatusBindErr(c, nil, err, objectType)
	}

	// check for missing required fields
	if id, err := apiserver_lib.ValidateBoundData(c, k8sWorkloadResourceDefinitions, objectType); err != nil {
		h.Logger.Error("handler error: error validating bound data", zap.Error(err))
		return apiserver_lib.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
	}

	// create all kubernetes workload resource definitions or none at all
	var createdWRDs []v0.KubernetesWorkloadResourceDefinition
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		for _, wrd := range k8sWorkloadResourceDefinitions {
			if result := h.DB.Create(&wrd); result.Error != nil {
				return result.Error
			}
			createdWRDs = append(createdWRDs, wrd)
		}

		return nil
	})
	if err != nil {
		h.Logger.Error("handler error: error creating kubernetes workload resource definitions", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	response, err := apiserver_lib.CreateResponse(
		apiserver_lib.SingleObjectMeta(),
		createdWRDs,
		objectType,
	)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, err, objectType)
	}

	return apiserver_lib.ResponseStatus201(c, *response)
}

