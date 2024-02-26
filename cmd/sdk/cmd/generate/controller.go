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

func ControllerGen(controllerDomain string, apiObjects []*sdk.APIObject) error {
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

	// Assemble all api objects in this controller domain according to there version
	versionObjMap := make(map[string][]*sdk.APIObject, 0)
	for _, obj := range apiObjects {
		for _, v := range obj.Versions {
			if _, exists := versionObjMap[*v]; exists {
				versionObjMap[*v] = append(versionObjMap[*v], obj)
			} else {
				versionObjMap[*v] = []*sdk.APIObject{obj}
			}
		}
	}

	for _, objects := range versionObjMap {

		controllerConfig.ReconciledObjects = make([]string, 0)
		for _, obj := range objects {
			if obj.Reconcilable != nil && *obj.Reconcilable {
				controllerConfig.ReconciledObjects = append(controllerConfig.ReconciledObjects, *obj.Name)
			}
		}

		// Get module path if its an extension
		var modulePath string
		if extension {
			var modError error
			modulePath, modError = GetPathFromGoModule()
			if modError != nil {
				return fmt.Errorf("could not get go module path for extension: %w", modError)
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

		if err := controllerConfig.ReconcilerFunctions(); err != nil {
			return fmt.Errorf("failed to generate code for controller's reconciler functions: %w", err)
		}

		//// generate the controller's reconcile functions
		//if err := controllerConfig.NotificationHelper(); err != nil {
		//	return fmt.Errorf("failed to generate notification helper for controller's reconcilers: %w", err)
		//}
	}

	return nil
}
