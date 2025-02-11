package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dave/jennifer/jen"
)

// WriteCodeToFile writes code generated using jennifer/jen library to a file.
// It creates the necessary directories for the file if needed.  It returns
// true if the file was written and any errors.
func WriteCodeToFile(
	jenFile *jen.File,
	filePath string,
	overwrite bool,
) (bool, error) {
	// parse directory from filename
	dir, _ := filepath.Split(filePath)

	// ensure directories exist
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return false, fmt.Errorf("failed to ensure directories %s exist: %w", dir, err)
		}
	}

	// check for file presence if not overwriting
	if !overwrite {
		fileExists := true
		if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
			fileExists = false
		}
		if fileExists {
			return false, nil
		}
	}

	// write file
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return false, fmt.Errorf("failed to open file at filepath %s to write generated code: %w", filePath, err)
	}
	defer file.Close()
	if err := jenFile.Render(file); err != nil {
		return false, fmt.Errorf("failed to render generated source code at filepath %s: %w", filePath, err)
	}

	return true, nil
}
