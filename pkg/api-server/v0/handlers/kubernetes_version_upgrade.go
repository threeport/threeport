package handlers

import (
	"net/http"
	"strconv"
	"time"

	echo "github.com/labstack/echo/v4"
	zap "go.uber.org/zap"
	gorm "gorm.io/gorm"

	awsnotif "github.com/threeport/threeport/internal/aws/notif"
	gcpnotif "github.com/threeport/threeport/internal/gcp/notif"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// The EKS and GKE definitions' updates ask their instances' clusters to be
// upgraded to the definition's KubernetesVersion.  The definitions have no
// controller of their own, so the request is made here, once the update is
// saved: each instance not yet asked for the version is marked unreconciled,
// with a refused or failed upgrade cleared, and its controller is notified.
// Each instance records the version it was asked for, so a request this
// failed to finish is finished by the next update of the definition.

func (h *Handler) AddAwsEksKubernetesRuntimeDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{}
}

func (h *Handler) GetAwsEksKubernetesRuntimeDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{}
}

func (h *Handler) PatchAwsEksKubernetesRuntimeDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{h.RequestAwsEksUpgrades}
}

func (h *Handler) PutAwsEksKubernetesRuntimeDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{h.RequestAwsEksUpgrades}
}

func (h *Handler) DeleteAwsEksKubernetesRuntimeDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{}
}

func (h *Handler) AddGcpGkeKubernetesRuntimeDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{}
}

func (h *Handler) GetGcpGkeKubernetesRuntimeDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{}
}

func (h *Handler) PatchGcpGkeKubernetesRuntimeDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{h.RequestGcpGkeUpgrades}
}

func (h *Handler) PutGcpGkeKubernetesRuntimeDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{h.RequestGcpGkeUpgrades}
}

func (h *Handler) DeleteGcpGkeKubernetesRuntimeDefinitionMiddleware() []echo.MiddlewareFunc {
	return []echo.MiddlewareFunc{}
}

// updatedDefinitionID returns the ID of the definition an update saved, and
// false when the update didn't save it.
func updatedDefinitionID(c echo.Context, err error) (uint, bool) {
	if err != nil || c.Response().Status != http.StatusOK {
		return 0, false
	}
	id, parseErr := strconv.ParseUint(c.Param("id"), 10, 0)
	if parseErr != nil {
		return 0, false
	}

	return uint(id), true
}

// RequestAwsEksUpgrades asks the instances of an updated EKS definition to
// upgrade their clusters to its KubernetesVersion.
func (h Handler) RequestAwsEksUpgrades(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		err := next(c)
		id, saved := updatedDefinitionID(c, err)
		if !saved {
			return err
		}

		var definition v0.AwsEksKubernetesRuntimeDefinition
		if result := h.RequestDB(c).First(&definition, id); result.Error != nil {
			h.Logger.Error("failed to read the updated EKS definition to request upgrades", zap.Error(result.Error))
			return nil
		}
		version := v0.KubernetesVersionOrDefault(definition.KubernetesVersion)

		var instances []v0.AwsEksKubernetesRuntimeInstance
		if result := h.RequestDB(c).Where("aws_eks_kubernetes_runtime_definition_id = ?", id).Find(&instances); result.Error != nil {
			h.Logger.Error("failed to list the EKS definition's instances to request upgrades", zap.Error(result.Error))
			return nil
		}
		for i := range instances {
			instance := &instances[i]
			if util.DerefString(instance.UpgradeRequestedVersion) == version {
				continue
			}
			request := v0.AwsEksKubernetesRuntimeInstance{
				Reconciliation:          v0.Reconciliation{Reconciled: util.Ptr(false)},
				UpgradeRequestedVersion: &version,
				UpgradeRefused:          util.Ptr(false),
				UpgradeFailed:           util.Ptr(false),
			}
			if result := h.Write(c, func(db *gorm.DB) *gorm.DB {
				return db.Model(instance).Updates(&request)
			}); result.Error != nil {
				h.Logger.Error("failed to request an EKS cluster upgrade", zap.Uint("instanceID", *instance.ID), zap.Error(result.Error))
				continue
			}
			instance.Reconciled, instance.UpgradeRequestedVersion = request.Reconciled, request.UpgradeRequestedVersion
			instance.UpgradeRefused, instance.UpgradeFailed = request.UpgradeRefused, request.UpgradeFailed
			h.publishUpgradeRequest(awsnotif.AwsEksKubernetesRuntimeInstanceUpdateSubject, instance)
		}

		return nil
	}
}

// RequestGcpGkeUpgrades asks the instances of an updated GKE definition to
// upgrade their clusters to its KubernetesVersion.
func (h Handler) RequestGcpGkeUpgrades(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		err := next(c)
		id, saved := updatedDefinitionID(c, err)
		if !saved {
			return err
		}

		var definition v0.GcpGkeKubernetesRuntimeDefinition
		if result := h.RequestDB(c).First(&definition, id); result.Error != nil {
			h.Logger.Error("failed to read the updated GKE definition to request upgrades", zap.Error(result.Error))
			return nil
		}
		version := v0.KubernetesVersionOrDefault(definition.KubernetesVersion)

		var instances []v0.GcpGkeKubernetesRuntimeInstance
		if result := h.RequestDB(c).Where("gcp_gke_kubernetes_runtime_definition_id = ?", id).Find(&instances); result.Error != nil {
			h.Logger.Error("failed to list the GKE definition's instances to request upgrades", zap.Error(result.Error))
			return nil
		}
		for i := range instances {
			instance := &instances[i]
			if util.DerefString(instance.UpgradeRequestedVersion) == version {
				continue
			}
			request := v0.GcpGkeKubernetesRuntimeInstance{
				Reconciliation:          v0.Reconciliation{Reconciled: util.Ptr(false)},
				UpgradeRequestedVersion: &version,
				UpgradeRefused:          util.Ptr(false),
				UpgradeFailed:           util.Ptr(false),
			}
			if result := h.Write(c, func(db *gorm.DB) *gorm.DB {
				return db.Model(instance).Updates(&request)
			}); result.Error != nil {
				h.Logger.Error("failed to request a GKE cluster upgrade", zap.Uint("instanceID", *instance.ID), zap.Error(result.Error))
				continue
			}
			instance.Reconciled, instance.UpgradeRequestedVersion = request.Reconciled, request.UpgradeRequestedVersion
			instance.UpgradeRefused, instance.UpgradeFailed = request.UpgradeRefused, request.UpgradeFailed
			h.publishUpgradeRequest(gcpnotif.GcpGkeKubernetesRuntimeInstanceUpdateSubject, instance)
		}

		return nil
	}
}

// upgradeRequestNotifier is an instance whose controller is notified of an
// upgrade request.
type upgradeRequestNotifier interface {
	NotificationPayload(operation notifications.NotificationOperation, requeue bool, lastDelay int64) (*[]byte, error)
}

// publishUpgradeRequest notifies an instance's controller that its cluster
// was asked to be upgraded.
func (h Handler) publishUpgradeRequest(subject string, instance upgradeRequestNotifier) {
	payload, err := instance.NotificationPayload(notifications.NotificationOperationUpdated, false, time.Now().Unix())
	if err != nil {
		h.Logger.Error("failed to create an upgrade request notification", zap.Error(err))
		return
	}
	if _, err := h.JS.Publish(subject, *payload); err != nil {
		h.Logger.Error("failed to publish an upgrade request notification", zap.Error(err))
	}
}
