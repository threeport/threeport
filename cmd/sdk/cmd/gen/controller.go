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
	"github.com/threeport/threeport/internal/sdk/mod"
)

// func ControllerGen(controllerDomain string, apiObjects []*sdk.ApiObject) error {
func ControllerGen(controllerDomain string, apiObjectGroup *sdk.ApiObjectGroup) error {
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

	extension, modulePath, err := mod.IsExtension()
	if err != nil {
		return fmt.Errorf("could not determine if running for an extension: %w", err)
	}

	controllerConfig.ReconciledObjects = make([]controller.ReconciledObject, 0)
	for _, apiObject := range apiObjectGroup.Objects {
		var versions []string
		for _, version := range apiObject.Versions {
			versions = append(versions, *version)
		}
		if apiObject.Reconcilable != nil && *apiObject.Reconcilable {
			disableNotificationPersistense := false
			if apiObject.DisableNotificationPersistence != nil && *apiObject.DisableNotificationPersistence {
				disableNotificationPersistense = true
			}

			controllerConfig.ReconciledObjects = append(controllerConfig.ReconciledObjects, controller.ReconciledObject{
				Name:                           *apiObject.Name,
				Versions:                       versions,
				DisableNotificationPersistence: disableNotificationPersistense,
			})
		}
		//}
	}
	// generate the controllers' internal package general source code
	if err := controllerConfig.InternalPackage(); err != nil {
		return fmt.Errorf("failed to generate code for controller's internal package source file: %w", err)
	}

	// generate the controller's reconcile function boilerplate
	if extension {
		if err := controllerConfig.ExtensionReconcilers(modulePath); err != nil {
			return fmt.Errorf("failed to generate code for controller's reconcilers for extension: %w", err)
		}
	} else {
		if err := controllerConfig.Reconcilers(); err != nil {
			return fmt.Errorf("failed to generate code for controller's reconcilers: %w", err)
		}
	}

	// generate the controllers notifcation streams and subjects
	if extension {
		if err := controllerConfig.Notifications(); err != nil {
			return fmt.Errorf("failed to generate code for controller notification streams and subjects: %w", err)
		}
	}

	// generate controllers' internal package CUD operation scaffolding
	if extension {
		if err := controllerConfig.ExtensionReconcilerCudFuncs(modulePath); err != nil {
			return fmt.Errorf("failed to generate code for controller's reconciler CUD operation functions: %w", err)
		}
	} else {
		if err := controllerConfig.ReconcilerCudFuncs(); err != nil {
			return fmt.Errorf("failed to generate code for controller's reconciler CUD operation functions: %w", err)
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
