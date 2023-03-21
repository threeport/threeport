package models

import "path/filepath"

func apiRoutesPath() string {
	return filepath.Join("..", "..", "..", "internal", "api", "routes")
}

func apiHandlersPath() string {
	return filepath.Join("..", "..", "..", "internal", "api", "handlers")
}

func apiVersionsPath() string {
	return filepath.Join("..", "..", "..", "internal", "api", "versions")
}

func apiInternalPath() string {
	return filepath.Join("..", "..", "..", "internal", "api")
}
