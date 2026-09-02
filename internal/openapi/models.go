package openapi

// OpenAPI represents the root OpenAPI 3.1.0 specification structure.
type OpenAPI struct {
	OpenAPI    string                 `json:"openapi"` // "3.1.0"
	Info       Info                   `json:"info"`
	Servers    []Server               `json:"servers"`
	Paths      map[string]PathItem    `json:"paths"`
	Components Components             `json:"components"`
	Security   []map[string][]string  `json:"security,omitempty"`
	Tags       []Tag                  `json:"tags,omitempty"`
}

// Info provides metadata about the API.
type Info struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Version     string  `json:"version"`
	License     *License `json:"license,omitempty"`
}

// License provides license information for the API.
type License struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// Server represents a target server host.
type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// Tag provides metadata for API categories.
type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PathItem describes the operations available on a single path.
type PathItem struct {
	Get    *Operation `json:"get,omitempty"`
	Post   *Operation `json:"post,omitempty"`
	Put    *Operation `json:"put,omitempty"`
	Delete *Operation `json:"delete,omitempty"`
	Patch  *Operation `json:"patch,omitempty"`
}

// Operation describes a single API operation on a path.
type Operation struct {
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	OperationID string                `json:"operationId,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]Response   `json:"responses"`
	Security    []map[string][]string `json:"security,omitempty"`
}

// Parameter describes a single operation parameter (path, query, header).
type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"` // query, path, header
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required"`
	Schema      *Schema `json:"schema,omitempty"`
}

// RequestBody describes the request body for an operation.
type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required"`
	Content     map[string]MediaType `json:"content"`
}

// Response describes a single response from an API operation.
type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// MediaType provides schema details for a specific media type (e.g. application/json).
type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

// Components holds reusable objects for the OpenAPI spec.
type Components struct {
	Schemas         map[string]*Schema        `json:"schemas"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes"`
}

// SecurityScheme defines a security mechanism that can be used by operations.
type SecurityScheme struct {
	Type         string `json:"type"` // "http"
	Scheme       string `json:"scheme"` // "bearer"
	BearerFormat string `json:"bearerFormat,omitempty"` // "JWT"
	Description  string `json:"description,omitempty"`
}

// Schema represents a JSON Schema object for OpenAPI 3.1.0.
type Schema struct {
	Type        string             `json:"type,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Description string             `json:"description,omitempty"`
	Format      string             `json:"format,omitempty"`
	Example     interface{}        `json:"example,omitempty"`
	Ref         string             `json:"$ref,omitempty"`
}
