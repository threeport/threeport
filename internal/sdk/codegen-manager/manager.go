package codegenmanager

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/threeport/threeport/internal/sdk"
)

// The CodegenManager is a struct that provides all operations to manage api objects
// via the SDK
type CodegenManager struct {
	// List of objects being managed
	ApiObjects map[string][]*sdk.ApiObject
}

// CreateManager returns a new APIObjectManager.
func CreateManager(config *sdk.ApiObjectConfig) (*CodegenManager, error) {

	objectMap := make(map[string][]*sdk.ApiObject)

	for _, og := range config.ApiObjectGroups {
		objectMap[*og.Name] = og.Objects

	}

	manager := &CodegenManager{
		ApiObjects: objectMap,
	}

	return manager, nil
}

// CreateAPIObject creates the boilerplate and scaffolding for a new API object.
func (manager *CodegenManager) CreateAPIObject(apiObjectConfig sdk.SdkConfig) error {
	// check to see if controller domain already exists
	for _, og := range apiObjectConfig.ApiObjectGroups {
		if _, exists := manager.ApiObjects[*og.Name]; exists {
			return fmt.Errorf("adding to an existing controller domain is not supported. please update the api file manually")
		}
	}

	// API object is part of a new controller domain, create necessary scaffolding
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get working directory: %w", err)
	}

	// For each of the provided api objects in a new controller domain, create the necessary scaffolding
	for _, og := range apiObjectConfig.ApiObjectGroups {
		apiFilePath := filepath.Join(wd, "pkg", "api", "v0", fmt.Sprintf("%s.go", *og.Name))

		// Create api file for controller domain in pkg/api/v0
		if err := CreateNewAPIFile(*og.Name, og.Objects, apiFilePath); err != nil {
			return fmt.Errorf("could not create api file: %w", err)
		}

		// Ensure the appropiate dirs are create so that subsequent code gen works as expected
		if err := CreateControllerDirs(*og.Name, wd); err != nil {
			return fmt.Errorf("could not create dirs for controller domain: %w", err)
		}

		// Ensure the docker file exists for controller builds
		if err := CreateControllerDockerfile(*og.Name); err != nil {
			return fmt.Errorf("could not create dockerfile for controller domain: %w", err)
		}

	}

	return nil
}
