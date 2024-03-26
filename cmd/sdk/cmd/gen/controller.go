/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package gen

import (
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/controller"
)

func ControllerGen(controllerDomain string, apiObjects []*sdk.ApiObject) error {
	baseName := controllerDomain
	controllerConfig := controller.ControllerConfig{
		Name: strings.ReplaceAll(
			fmt.Sprintf("%s-controller", baseName),
			"_",
			"-",
		),
		ShortName:   strings.ReplaceAll(baseName, "_", "-"),
		PackageName: strings.ReplaceAll(baseName, "_", ""),
		StreamName: fmt.Sprintf(
			"%sStreamName", strcase.ToCamel(baseName),
		),
	}

	extension, modulePath, err := isExtension()
	if err != nil {
		return fmt.Errorf("could not determine if running for an extension: %w", err)
	}

	// Assemble all api objects in this controller domain according to there version
	versionObjMap := make(map[string][]*sdk.ApiObject, 0)
	for _, obj := range apiObjects {
		for _, v := range obj.Versions {
			if _, exists := versionObjMap[*v]; exists {
				versionObjMap[*v] = append(versionObjMap[*v], obj)
			} else {
				versionObjMap[*v] = []*sdk.ApiObject{obj}
			}
		}
	}

	for version, objects := range versionObjMap {

		controllerConfig.ReconciledObjects = make([]controller.ReconciledObject, 0)
		for _, obj := range objects {
			if obj.Reconcilable != nil && *obj.Reconcilable {
				disableNotificationPersistense := false
				if obj.DisableNotificationPersistence != nil && *obj.DisableNotificationPersistence {
					disableNotificationPersistense = true
				}

				controllerConfig.ReconciledObjects = append(controllerConfig.ReconciledObjects, controller.ReconciledObject{
					Name:                           *obj.Name,
					Version:                        version,
					DisableNotificationPersistence: disableNotificationPersistense,
				})
			}
		}

		// generate the controller's internal package general source code
		if err := controllerConfig.InternalPackage(); err != nil {
			return fmt.Errorf("failed to generate code for controller's internal package source file: %w", err)
		}

		// generate the controller's reconcile functions
		if extension {
			if err := controllerConfig.ExtensionReconcilers(modulePath); err != nil {
				return fmt.Errorf("failed to generate code for controller's reconcilers for extension: %w", err)
			}
		} else {
			if err := controllerConfig.Reconcilers(); err != nil {
				return fmt.Errorf("failed to generate code for controller's reconcilers: %w", err)
			}
		}

		//// generate the controller's reconcile functions
		//if err := controllerConfig.NotificationHelper(); err != nil {
		//	return fmt.Errorf("failed to generate notification helper for controller's reconcilers: %w", err)
		//}

	}

	controllerConfig.ReconciledObjects = make([]controller.ReconciledObject, 0)
	for _, obj := range apiObjects {
		if obj.Reconcilable != nil && *obj.Reconcilable {
			disableNotificationPersistense := false
			if obj.DisableNotificationPersistence != nil && *obj.DisableNotificationPersistence {
				disableNotificationPersistense = true
			}

			for _, v := range obj.Versions {
				controllerConfig.ReconciledObjects = append(controllerConfig.ReconciledObjects, controller.ReconciledObject{
					Name:                           *obj.Name,
					Version:                        *v,
					DisableNotificationPersistence: disableNotificationPersistense,
				})
			}
		}
	}

	// generate the controller's main package
	if extension {
		if err := controllerConfig.ExtensionMainPackage(modulePath); err != nil {
			return fmt.Errorf("failed to generate code for controller's main package for extension: %w", err)
		}
	} else {
		if err := controllerConfig.MainPackage(); err != nil {
			return fmt.Errorf("failed to generate code for controller's main package: %w", err)
		}
	}

	return nil
}
