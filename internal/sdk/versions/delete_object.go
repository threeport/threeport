package versions

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
)

// DeleteSwitch generates code for deleting an object
// given a string representation of its type and an ID.
func (gvc *GlobalVersionConfig) DeleteObjects() error {
	for _, apiVersion := range gvc.Versions {
		f := NewFile(apiVersion.VersionName)
		f.HeaderComment("generated by 'threeport-sdk gen' for API object deletion boilerplate - do not edit")

		objects := &Statement{}
		for _, name := range apiVersion.RouteNames {
			objects.Case(Lit(fmt.Sprintf("%s.%s", apiVersion.VersionName, name))).Block(
				If(Id("_").Op(",").Id("err")).Op(":=").Id(
					fmt.Sprintf("Delete%s", name),
				).Call(List(Id("apiClient"), Id("apiAddr"), Id("id"))).Op(";").Err().Op("!=").Nil().Block(
					Return().Qual(
						"fmt", "Errorf",
					).Call(Lit(fmt.Sprintf("failed to delete %s: %%w", name)).Op(",").Id("err")),
				),
			)
			objects.Line()
		}

		f.Comment("DeleteObjectByTypeAndID deletes an instance given a string representation of its type and ID.")
		f.Func().Id("DeleteObjectByTypeAndID").Params(
			Id("apiClient").Op("*").Qual("net/http", "Client"),
			Id("apiAddr").Id("string"),
			Id("objectType").Id("string"),
			Id("id").Id("uint"),
		).Id("error").Block(
			Line(),
			Switch(Id("objectType")).Block(
				objects,
			),
			Line(),
			Return().Nil(),
		)
		f.Line()

		// write code to file
		routesFilepath := filepath.Join("pkg", "client", apiVersion.VersionName, "delete_object_gen.go")
		file, err := os.OpenFile(routesFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open file to write generated code for delete object function: %w", err)
		}
		defer file.Close()
		if err := f.Render(file); err != nil {
			return fmt.Errorf("failed to render generated source code for delete object function: %w", err)
		}
		fmt.Println("code generation complete for delete object function")

	}

	return nil
}
