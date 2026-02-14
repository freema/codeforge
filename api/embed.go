package api

import _ "embed"

// OpenAPISpec contains the raw OpenAPI YAML specification.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
