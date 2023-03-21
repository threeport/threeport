package models

import "path/filepath"

func apiRoutesPath() string {
	return filepath.Join("..", "..", "..", "internal", "routes")
}

func apiHandlersPath() string {
	return filepath.Join("..", "..", "..", "internal", "handlers")
}

func apiVersionsPath() string {
	return filepath.Join("..", "..", "..", "internal", "versions")
}

func apiInternalPath() string {
	return filepath.Join("..", "..", "..", "internal", "api")
}
