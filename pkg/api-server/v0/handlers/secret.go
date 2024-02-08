package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	echo "github.com/labstack/echo/v4"
	iapi "github.com/threeport/threeport/pkg/api-server/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	"gorm.io/datatypes"
	gorm "gorm.io/gorm"
)

func (h *Handler) AddSecretDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{
		h.CustomAddSecretDefinition,
	}
}

func (h *Handler) GetSecretDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{}
}

func (h *Handler) PatchSecretDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{}
}

func (h *Handler) PutSecretDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{}
}

func (h *Handler) DeleteSecretDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{}
}

// @Summary adds a new secret definition.
// @Description Add a new secret definition to the Threeport database.
// @ID add-secretDefinition
// @Accept json
// @Produce json
// @Param secretDefinition body v0.SecretDefinition true "SecretDefinition object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/secret-definitions [POST]
func (h Handler) CustomAddSecretDefinition(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		objectType := v0.ObjectTypeSecretDefinition
		var secretDefinition v0.SecretDefinition

		// check for empty payload, unsupported fields, GORM Model fields, optional associations, etc.
		if id, err := iapi.PayloadCheck(c, false, objectType, secretDefinition); err != nil {
			return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
		}

		if err := c.Bind(&secretDefinition); err != nil {
			return iapi.ResponseStatus500(c, nil, err, objectType)
		}

		// check for missing required fields
		if id, err := iapi.ValidateBoundData(c, secretDefinition, objectType); err != nil {
			return iapi.ResponseStatusErr(id, c, nil, errors.New(err.Error()), objectType)
		}

		// check for duplicate names
		var existingSecretDefinition v0.SecretDefinition
		nameUsed := true
		result := h.DB.Where("name = ?", secretDefinition.Name).First(&existingSecretDefinition)
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				nameUsed = false
			} else {
				return iapi.ResponseStatus500(c, nil, result.Error, objectType)
			}
		}
		if nameUsed {
			return iapi.ResponseStatus409(c, nil, errors.New("object with provided name already exists"), objectType)
		}

		// create copy of Data field in memory, as it will be
		// set to nil in the secretDefinition.BeforeCreate() method
		data := secretDefinition.Data

		// persist to DB
		if result := h.DB.Create(&secretDefinition); result.Error != nil {
			return iapi.ResponseStatus500(c, nil, result.Error, objectType)
		}

		// encrypt sensitive values
		var encryptionKey = os.Getenv("ENCRYPTION_KEY")
		if encryptionKey == "" {
			return errors.New("environment variable ENCRYPTION_KEY is not set")
		}

		// define the secret name and value
		var secretData map[string]string
		if err := json.Unmarshal([]byte(*data), &secretData); err != nil {
			return fmt.Errorf("failed to unmarshal secret data")
		}

		// encrypt the secret data's values
		encryptedDataMap, err := encryption.EncryptStringMap(encryptionKey, secretData)
		if err != nil {
			return fmt.Errorf("failed to encrypt secret data")
		}

		// marshal back to json and update secretDefinition
		marshaledJson, err := json.Marshal(encryptedDataMap)
		if err != nil {
			return fmt.Errorf("failed to marshal encrypted secret data")
		}
		encryptedJson := datatypes.JSON(marshaledJson)
		secretDefinition.Data = &encryptedJson

		// notify controller if reconciliation is required
		if !*secretDefinition.Reconciled {
			notifPayload, err := secretDefinition.NotificationPayload(
				notifications.NotificationOperationCreated,
				false,
				time.Now().Unix(),
			)
			if err != nil {
				return iapi.ResponseStatus500(c, nil, err, objectType)
			}
			h.JS.Publish(v0.SecretDefinitionCreateSubject, *notifPayload)
		}

		response, err := v0.CreateResponse(nil, secretDefinition, objectType)
		if err != nil {
			return iapi.ResponseStatus500(c, nil, err, objectType)
		}

		return iapi.ResponseStatus201(c, *response)
	}
}
