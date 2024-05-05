package models

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"
	"github.com/threeport/threeport/internal/sdk"
)

// ConfigPkg generates the config package code.
func (cc *ControllerConfig) ConfigPkg() error {
	f := NewFile(cc.PackageName)

	for _, modelConf := range cc.ModelConfigs {
		configObjectName := fmt.Sprintf("%sConfig", modelConf.TypeName)
		objectHuman := strcase.ToDelimited(modelConf.TypeName, ' ')

		f.Comment(fmt.Sprintf(
			"%s contains the config for a %s.",
			configObjectName,
			objectHuman,
		))
		f.Type().Id(configObjectName).Struct(
			Id("Name").String().Tag(map[string]string{"json": "Name", "validate": "required"}),
		)
	}

	// create directories if they don't exist
	configPkgPath := filepath.Join("pkg", "config", cc.PackageName)
	if _, err := os.Stat(configPkgPath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(configPkgPath, 0755); err != nil {
			return fmt.Errorf("could not create directores for config package: %s, %w", configPkgPath, err)
		}
	}

	// write code to file
	genFilename := fmt.Sprintf("%s_gen.go", sdk.FilenameSansExt(cc.ModelFilename))
	genFilepath := filepath.Join(configPkgPath, genFilename)
	file, err := os.OpenFile(genFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for config package: %w", err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for config package: %w", err)
	}
	fmt.Printf("code generation complete for %s config package\n", cc.ControllerDomainLower)

	return nil
}
