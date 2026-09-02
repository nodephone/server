// Package openapi provides automatic route discovery, OpenAPI 3.1.0 specification generation,
// schema reflection, in-memory specification caching, and interactive API documentation for NodePhone.
package openapi

// RouteMeta holds metadata for a registered HTTP route used for docs and SDK generation.
type RouteMeta struct {
	Method        string      `json:"method"`
	Path          string      `json:"path"`
	Summary       string      `json:"summary"`
	Description   string      `json:"description"`
	Tags          []string    `json:"tags"`
	AuthRequired  bool        `json:"auth_required"`
	RequestSchema interface{} `json:"request_schema,omitempty"`
	ResponseSchema interface{} `json:"response_schema,omitempty"`
}

// SDKMeta holds metadata required for SDK code generators (TypeScript, Flutter, Python).
type SDKMeta struct {
	Language string      `json:"language"`
	Version  string      `json:"version"`
	Routes   []RouteMeta `json:"routes"`
}
