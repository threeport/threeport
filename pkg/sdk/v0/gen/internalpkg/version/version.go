package version

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/dave/jennifer/jen"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// getVersionFilename returns the filename of the text file that contains the current
// version for each released component of the project.
func getVersionFilename() string {
	return "version.txt"
}

// CreateVersionPackage creates the version package that contains the referenced
// version of each component.
func GenVersionPackage() error {
	f := NewFile("version")
	f.HeaderComment(sdk.HeaderCommentGenNoEdit)
	f.Anon("embed")

	f.Comment("Version is the version in version.txt.")
	f.Comment(fmt.Sprintf("//go:embed %s", getVersionFilename()))
	f.Var().Id("Version").String()

	// Commit is a separate variable because the linker's -X flag cannot set a
	// go:embed variable.
	f.Comment("Commit is the 7-character short commit SHA of a Head build.  It is set at")
	f.Comment("build time with the following linker flag and is empty for RC and OSR builds:")
	f.Comment("")
	f.Comment("\t-ldflags \"-X <module path>/internal/version.Commit=<short sha>\"")
	f.Var().Id("Commit").String()

	f.Comment("GetVersion returns the version.  A Head build returns <version>.<commit>.")
	f.Func().Id("GetVersion").Params().String().Block(
		Id("v").Op(":=").Qual("strings", "TrimSuffix").Call(Id("Version"), Lit("\n")),
		If(Id("Commit").Op("!=").Lit("")).Block(
			Return(Id("v").Op("+").Lit(".").Op("+").Id("Commit")),
		),
		Return(Id("v")),
	)

	versionPath := filepath.Join("internal", "version")
	if _, err := os.Stat(versionPath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(versionPath, 0755); err != nil {
			return fmt.Errorf("could not create directores for internal version package: %s, %w", versionPath, err)
		}
	}

	// write code to file
	genFilepath := filepath.Join(versionPath, "version_gen.go")
	_, err := util.WriteCodeToFile(f, genFilepath, true)
	if err != nil {
		return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
	}
	cli.Info(fmt.Sprintf("source code for internal version package written to %s", genFilepath))

	// create version text file if it doesn't already exist
	verstionTextFilePath := filepath.Join(versionPath, getVersionFilename())
	if _, err := os.Stat(verstionTextFilePath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(verstionTextFilePath, []byte("v0.0.1-dev"), 0644); err != nil {
			return fmt.Errorf("failed to write version text file for version package: %w", err)
		}
		cli.Info(fmt.Sprintf("project version written to text file %s", verstionTextFilePath))
	} else {
		cli.Info(fmt.Sprintf("project version text file already exists at %s - not regenerated", verstionTextFilePath))
	}

	return nil
}
