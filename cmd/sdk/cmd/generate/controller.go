/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package gen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/controller"
)

func ControllerGen(controllerDomain string, apiVersion string) error {
	modelFilepathForController := filepath.Join("pkg", "api", apiVersion, fmt.Sprintf("%s.go", controllerDomain))
	// inspect source code
	fset := token.NewFileSet()
	pf, err := parser.ParseFile(fset, modelFilepathForController, nil, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return fmt.Errorf("failed to parse source code file: %w", err)
	}
	// ////////////////////////////////////////////////////////////////////////
	// // print the syntax tree for dev purposes
	//
	//	if err = ast.Print(fset, pf); err != nil {
	//		return err
	//	}
	//
	// ////////////////////////////////////////////////////////////////////////
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

	for _, node := range pf.Decls {
		switch node.(type) {
		case *ast.GenDecl:
			var objectName string
			genDecl := node.(*ast.GenDecl)
			for _, spec := range genDecl.Specs {
				switch spec.(type) {
				case *ast.TypeSpec:
					typeSpec := spec.(*ast.TypeSpec)
					objectName = typeSpec.Name.Name
				}
			}
			if genDecl.Doc != nil {
				for _, comment := range genDecl.Doc.List {
					if strings.Contains(comment.Text, sdk.ReconclierMarkerText) {
						controllerConfig.ReconciledObjects = append(controllerConfig.ReconciledObjects, objectName)
					}
				}
			}
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

	//// generate the controller's reconcile functions
	//if err := controllerConfig.NotificationHelper(); err != nil {
	//	return fmt.Errorf("failed to generate notification helper for controller's reconcilers: %w", err)
	//}

	return nil
}
