package util

import (
	"github.com/dave/jennifer/jen"
)

// SetImportAlias is a helper function for setting import aliases based on
// whether the project is an extension or threeport/threeport.
func SetImportAlias(
	importPath string,
	tpAlias string,
	extAlias string,
	extension bool,
) (string, string) {
	if extension {
		return importPath, extAlias
	} else {
		return importPath, tpAlias
	}
}

// QualifiedOrLocal produces a jen statement that is either an identifier or a
// qualified identifier based on whether the project is an extension
func QualifiedOrLocal(
	extension bool,
	packagePath string,
	identifier string,
) *jen.Statement {
	s := &jen.Statement{}
	if extension {
		s.Qual(
			packagePath,
			identifier,
		)
	} else {
		s.Id(identifier)
	}

	return s
}
