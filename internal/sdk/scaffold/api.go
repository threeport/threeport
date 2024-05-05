package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/dave/jennifer/jen"

	"github.com/threeport/threeport/internal/sdk"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// CreateAPIObject creates the boilerplate and scaffolding for a new API object.
func CreateAPIObjects(sdkConfig *sdk.SdkConfig, extension bool) error {
	// for each of the provided api objects in a new controller domain, create the necessary scaffolding
	for _, objectGroup := range sdkConfig.ApiObjectGroups {
		for _, object := range objectGroup.Objects {
			for _, version := range object.Versions {
				apiFilePath := filepath.Join("pkg", "api", *version, fmt.Sprintf("%s.go", *objectGroup.Name))

				// create directories if needed
				if err := createApiDirs(*version); err != nil {
					return fmt.Errorf("failed to create directories for APIs: %w", err)
				}

				// create api file for controller domain in pkg/api/v0
				if err := createNewApiFile(*version, *objectGroup.Name, objectGroup.Objects, apiFilePath, extension); err != nil {
					return fmt.Errorf("could not create API file: %w", err)
				}
			}
		}
	}

	return nil
}

// createApiDirs creates the directories for the API object source files.
func createApiDirs(version string) error {
	path := filepath.Join("pkg", "api", version)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create API object directory with path %s: %w", path, err)
		}
	}

	return nil
}

// createNewApiFile creates the source code scaffolding for a new API object.
func createNewApiFile(
	version string,
	controllerDomain string,
	apiObjects []*sdk.ApiObject,
	apiFilePath string,
	extension bool,
) error {
	f := NewFile(version)
	f.HeaderComment("originally generated by 'threeport-sdk create api-objects' for API object scaffolding but will not be re-generated - intended for modification")
	if extension {
		f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "tpapi_v0")
	}
	f.Line()

	// create the necessary structs for each object in the domain api file
	for _, obj := range apiObjects {
		structFields := make([]Code, 0)
		if extension {
			structFields = append(
				structFields,
				Qual(
					"github.com/threeport/threeport/pkg/api/v0",
					"Common",
				).Tag(map[string]string{"swaggerignore": "true", "mapstructure": ",squash"}),
			)
		} else {
			structFields = append(
				structFields,
				Id("Common").Tag(map[string]string{"swaggerignore": "true", "mapstructure": ",squash"}),
			)
		}

		// infer if object needs to be reconciled and add appropiate markers and fields
		if obj.Reconcilable != nil && *obj.Reconcilable {
			if extension {
				structFields = append(
					structFields,
					Qual(
						"github.com/threeport/threeport/pkg/api/v0",
						"Reconciliation",
					).Tag(map[string]string{"mapstructure": ",squash"}),
				)
			} else {
				structFields = append(
					structFields,
					Id("Reconciliation").Tag(map[string]string{"mapstructure": ",squash"}),
				)
			}

			// infer if the object is an instance or a definition and add appropiate fields for it
			if strings.HasSuffix(strings.ToLower(*obj.Name), "instance") {
				if extension {
					structFields = append(
						structFields,
						Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"Instance",
						).Tag(map[string]string{"mapstructure": ",squash"}),
					)
				} else {
					structFields = append(
						structFields,
						Id("Instance").Tag(map[string]string{"mapstructure": ",squash"}),
					)
				}
			}
			if strings.HasSuffix(strings.ToLower(*obj.Name), "definition") {
				if extension {
					structFields = append(
						structFields,
						Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"Definition",
						).Tag(map[string]string{"mapstructure": ",squash"}),
					)
				} else {
					structFields = append(
						structFields,
						Id("Definition").Tag(map[string]string{"mapstructure": ",squash"}),
					)
				}
			}
		}

		// define the struct for the object
		f.Type().Id(*obj.Name).Struct(structFields...)
		f.Line()
	}

	// write code to file if it doesn't already exist
	if _, err := os.Stat(apiFilePath); errors.Is(err, os.ErrNotExist) {
		file, err := os.OpenFile(apiFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open file to write generated code for %s API file: %w", controllerDomain, err)
		}
		defer file.Close()
		if err := f.Render(file); err != nil {
			return fmt.Errorf("failed to render generated source code for %s API file: %w", controllerDomain, err)
		}
		cli.Info(fmt.Sprintf("code generation complete for %s API file", controllerDomain))
	} else {
		cli.Info(fmt.Sprintf("API file %s already exists - not regenerated", controllerDomain))
	}

	return nil
}
