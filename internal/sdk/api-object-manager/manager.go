package apiobjectmanager

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/threeport/threeport/internal/sdk"
)

// The APIObjectManager is a struct that provides all operations to manage api objects
// via the SDK
type APIObjectManager struct {
	// List of objects being managed
	APIObjects map[string][]*sdk.APIObject
}

func CreateManager(config *sdk.SDKConfig) (*APIObjectManager, error) {
	manager := &APIObjectManager{
		APIObjects: config.APIObjects,
	}

	return manager, nil
}

func (manager *APIObjectManager) CreateAPIObject(apiObjectConfig sdk.APIObjectConfig) error {
	// check to see if controller domain already exists
	for cd := range apiObjectConfig.APIObjects {
		if _, exists := manager.APIObjects[cd]; exists {
			return fmt.Errorf("adding to an existing controller domain is not supported. please update the api file manually")
		}
	}

	// API object is part of a new controller domain, create necessary scaffolding
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get working directory: %w", err)
	}

	// For each of the provided api objects in a new controller domain, create the necessary scaffolding
	for controllerDomain, apiObjects := range apiObjectConfig.APIObjects {
		apiFilePath := filepath.Join(wd, "pkg", "api", "v0", fmt.Sprintf("%s.go", controllerDomain))

		// Create api file for controller domain in pkg/api/v0
		if err := CreateNewAPIFile(controllerDomain, apiObjects, apiFilePath); err != nil {
			return fmt.Errorf("could not create api file: %w", err)
		}

		// Ensure the appropiate dirs are create so that subsequent code gen works as expected
		if err := CreateControllerDirs(controllerDomain, wd); err != nil {
			return fmt.Errorf("could not create dirs for controller domain: %w", err)
		}

		// Ensure the docker file exists for controller builds
		if err := CreateControllerDockerfile(controllerDomain); err != nil {
			return fmt.Errorf("could not create dockerfile for controller domain: %w", err)
		}

	}

	return nil
}
