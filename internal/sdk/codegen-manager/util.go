package codegenmanager

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	. "github.com/dave/jennifer/jen"
	"github.com/threeport/threeport/internal/sdk"
)

// CreateNewAPIFile creates the source code scaffolding for a new API object.
func CreateNewAPIFile(controllerDomain string, apiObjects []*sdk.ApiObject, apiFilePath string) error {
	f := NewFile("v0")
	f.HeaderComment("generated by 'threeport-sdk create api-objects'")
	f.Line()

	// Create the necessary structs for each object in the domain api file
	for _, obj := range apiObjects {
		structFields := make([]Code, 0)
		structFields = append(structFields, Id("Common").Tag(map[string]string{"swaggerignore": "true", "mapstructure": ",squash"}))

		// Infer if object needs to be reconciled and add appropiate markers and fields
		if obj.Reconcilable != nil && *obj.Reconcilable {
			structFields = append(structFields, Id("Reconciliation").Tag(map[string]string{"mapstructure": ",squash"}))

			// Infer if the object is an instance or a definition and add appropiate fields for it
			if strings.HasSuffix(strings.ToLower(*obj.Name), "instance") {
				structFields = append(structFields, Id("Instance").Tag(map[string]string{"mapstructure": ",squash"}))
			}
			if strings.HasSuffix(strings.ToLower(*obj.Name), "definition") {
				structFields = append(structFields, Id("Definition").Tag(map[string]string{"mapstructure": ",squash"}))
			}
		}

		// Define the struct for the object
		f.Type().Id(*obj.Name).Struct(structFields...)
		f.Line()
	}

	// write code to file
	file, err := os.OpenFile(apiFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for %s api file: %w", controllerDomain, err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for %s api file: %w", controllerDomain, err)
	}
	fmt.Printf("code generation complete for %s api file\n", controllerDomain)

	return nil
}

// CreateControllerDirs creates the directory scaffolding for a controller that will reconcile state for API objects.
func CreateControllerDirs(controllerDomain string, rootDir string) error {
	// controller domain name is in snake case and the dir name has to be kebab case
	kebabDomain := strings.ReplaceAll(controllerDomain, "_", "-")
	controllerName := fmt.Sprintf("%s-controller", controllerDomain)

	// create dir for controller cmd
	if err := os.Mkdir(fmt.Sprintf("cmd/%s", controllerName), 0755); err != nil {
		return fmt.Errorf("could not create cmd dir for controller domain: %s, %w", kebabDomain, err)
	}

	// create dir for controller image
	if err := os.Mkdir(fmt.Sprintf("cmd/%s/image", controllerName), 0755); err != nil {
		return fmt.Errorf("could not create image dir for cmd controller domain: %s, %w", kebabDomain, err)
	}

	// create internal dir for reconcilers
	if err := os.Mkdir(fmt.Sprintf("internal/%s", kebabDomain), 0755); err != nil {
		return fmt.Errorf("could not create internal dir for controller domain: %s, %w", kebabDomain, err)
	}

	fmt.Printf("dir creation complete for controller %s\n", controllerName)
	return nil
}

var dockerfileTemplate string = `ARG ARCH=amd64
FROM golang:1.20 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -a -o $CONTROLLER_NAME cmd/$CONTROLLER_NAME/main_gen.go
FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/$CONTROLLER_NAME /
USER 65532:65532
ENTRYPOINT ["/$CONTROLLER_NAME"]
`

// CreateControllerDockerfile creates the Dockerfile boilerplate for container builds of a controller.
func CreateControllerDockerfile(controllerDomain string) error {
	// controller domain name is in snake case and the dir name has to be kebab case
	controllerName := fmt.Sprintf("%s-controller", controllerDomain)
	dockerFile := strings.Replace(dockerfileTemplate, "$CONTROLLER_NAME", controllerName, -1)

	dockerfilePath := filepath.Join("cmd", controllerName, "image", "Dockerfile")
	if err := ioutil.WriteFile(dockerfilePath, []byte(dockerFile), 0644); err != nil {
		return fmt.Errorf("could not write dockerfile contents: %w", err)
	}

	fmt.Printf("dockerfile creation complete for controller %s\n", controllerName)
	return nil
}
