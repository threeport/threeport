package docs

import (
	_ "embed"
)

// SwaggerJson is a constant variable containing the swagger.json file info
//
//go:embed swagger.json
var SwaggerJson string
