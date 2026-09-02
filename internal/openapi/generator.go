package openapi

import (
	"fmt"
	"strings"
)

// BuildOpenAPISpec generates the complete OpenAPI 3.1.0 specification object from registered route metadata.
func BuildOpenAPISpec(host string, port int, routes []RouteMeta) *OpenAPI {
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	serverURL := fmt.Sprintf("http://%s:%d", host, port)

	spec := &OpenAPI{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:       "NodePhone API",
			Description: "NodePhone Open Source Server Engine — Production REST, Realtime, Storage, & Functions API",
			Version:     "v1.0.0",
			License: &License{
				Name: "MIT",
				URL:  "https://opensource.org/licenses/MIT",
			},
		},
		Servers: []Server{
			{
				URL:         serverURL,
				Description: "NodePhone Server Active Instance",
			},
		},
		Tags: []Tag{
			{Name: "System", Description: "Health probes, version, and server readiness status"},
			{Name: "Auth", Description: "User registration, Argon2id authentication, JWT sessions, and API keys"},
			{Name: "Storage", Description: "Disk file storage, multipart streaming, and HMAC signed access URLs"},
			{Name: "Realtime", Description: "WebSocket room channels, broadcast events, and online user presence"},
			{Name: "Functions", Description: "Serverless JavaScript function execution engine"},
			{Name: "Permissions", Description: "Row-level security policy administration"},
		},
		Components: BuildDefaultComponents(),
		Security: []map[string][]string{
			{"bearerAuth": {}},
		},
		Paths: make(map[string]PathItem),
	}

	// Standard error responses map
	stdResponses := map[string]Response{
		"400": {
			Description: "Bad Request - Invalid parameters or payload format",
			Content:     map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}},
		},
		"401": {
			Description: "Unauthorized - Missing or invalid authentication token",
			Content:     map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}},
		},
		"403": {
			Description: "Forbidden - Operation denied by Row-Level Security policy",
			Content:     map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}},
		},
		"404": {
			Description: "Not Found - Requested endpoint or resource does not exist",
			Content:     map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}},
		},
		"500": {
			Description: "Internal Server Error - Server execution failure",
			Content:     map[string]MediaType{"application/json": {Schema: &Schema{Ref: "#/components/schemas/ErrorResponse"}}},
		},
	}

	for _, r := range routes {
		pathKey := r.Path
		pathItem := spec.Paths[pathKey]

		op := &Operation{
			Summary:     r.Summary,
			Description: r.Description,
			Tags:        r.Tags,
			OperationID: buildOperationID(r.Method, r.Path),
			Responses:   make(map[string]Response),
		}

		// Security requirement
		if r.AuthRequired {
			op.Security = []map[string][]string{
				{"bearerAuth": {}},
			}
		} else {
			op.Security = []map[string][]string{}
		}

		// Attach standard responses
		for code, resp := range stdResponses {
			op.Responses[code] = resp
		}

		// Attach success response
		if r.ResponseSchema != nil {
			if refStr, ok := r.ResponseSchema.(string); ok {
				op.Responses["200"] = Response{
					Description: "Success",
					Content:     map[string]MediaType{"application/json": {Schema: &Schema{Ref: refStr}}},
				}
			}
		} else {
			op.Responses["200"] = Response{Description: "Successful operation"}
		}

		// Attach request body schema if present
		if r.RequestSchema != nil {
			if refStr, ok := r.RequestSchema.(string); ok {
				op.RequestBody = &RequestBody{
					Required:    true,
					Description: "Request body payload",
					Content:     map[string]MediaType{"application/json": {Schema: &Schema{Ref: refStr}}},
				}
			}
		}

		// Map HTTP method to PathItem
		method := strings.ToUpper(r.Method)
		switch method {
		case "GET":
			pathItem.Get = op
		case "POST":
			pathItem.Post = op
		case "PUT":
			pathItem.Put = op
		case "DELETE":
			pathItem.Delete = op
		case "PATCH":
			pathItem.Patch = op
		}

		spec.Paths[pathKey] = pathItem
	}

	return spec
}

func buildOperationID(method, pathStr string) string {
	cleanPath := strings.ReplaceAll(pathStr, "/", "_")
	cleanPath = strings.ReplaceAll(cleanPath, "{", "")
	cleanPath = strings.ReplaceAll(cleanPath, "}", "")
	return strings.ToLower(method) + cleanPath
}
