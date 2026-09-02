package openapi

// BuildDefaultComponents returns the complete set of reusable OpenAPI 3.1.0 schemas and security schemes.
func BuildDefaultComponents() Components {
	return Components{
		SecuritySchemes: map[string]SecurityScheme{
			"bearerAuth": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "JWT",
				Description:  "Authenticate requests using JWT Access Tokens or API Keys (np_live_...)",
			},
		},
		Schemas: map[string]*Schema{
			"ErrorResponse": {
				Type: "object",
				Properties: map[string]*Schema{
					"error": {Type: "string", Example: "Resource not found or unauthorized access"},
				},
				Required: []string{"error"},
			},
			"User": {
				Type: "object",
				Properties: map[string]*Schema{
					"id":         {Type: "string", Format: "uuid", Example: "66e168c9-73f9-4d8f-af28-e70b76b18e4a"},
					"username":   {Type: "string", Example: "sagar"},
					"email":      {Type: "string", Format: "email", Example: "sagar@example.com"},
					"role":       {Type: "string", Example: "user"},
					"created_at": {Type: "string", Format: "date-time"},
					"updated_at": {Type: "string", Format: "date-time"},
				},
				Required: []string{"id", "username", "email", "role"},
			},
			"SignUpRequest": {
				Type: "object",
				Properties: map[string]*Schema{
					"username": {Type: "string", Example: "sagar"},
					"email":    {Type: "string", Format: "email", Example: "sagar@example.com"},
					"password": {Type: "string", Format: "password", Example: "SecurePassword123!"},
				},
				Required: []string{"username", "email", "password"},
			},
			"LoginRequest": {
				Type: "object",
				Properties: map[string]*Schema{
					"login":    {Type: "string", Example: "sagar"},
					"password": {Type: "string", Format: "password", Example: "SecurePassword123!"},
				},
				Required: []string{"login", "password"},
			},
			"RefreshRequest": {
				Type: "object",
				Properties: map[string]*Schema{
					"refresh_token": {Type: "string", Example: "eyJhbGciOiJIUzI1..."},
				},
				Required: []string{"refresh_token"},
			},
			"AuthResponse": {
				Type: "object",
				Properties: map[string]*Schema{
					"access_token":  {Type: "string"},
					"refresh_token": {Type: "string"},
					"expires_at":    {Type: "string", Format: "date-time"},
					"user":          {Ref: "#/components/schemas/User"},
				},
			},
			"APIKeyResponse": {
				Type: "object",
				Properties: map[string]*Schema{
					"api_key":    {Type: "string", Example: "np_live_4a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d"},
					"name":       {Type: "string", Example: "Production Integration Key"},
					"created_at": {Type: "string", Format: "date-time"},
				},
			},
			"Bucket": {
				Type: "object",
				Properties: map[string]*Schema{
					"id":         {Type: "string", Format: "uuid"},
					"name":       {Type: "string", Example: "documents"},
					"public":     {Type: "boolean", Example: false},
					"created_by": {Type: "string", Format: "uuid"},
					"created_at": {Type: "string", Format: "date-time"},
				},
			},
			"Object": {
				Type: "object",
				Properties: map[string]*Schema{
					"id":          {Type: "string", Format: "uuid"},
					"bucket_id":   {Type: "string", Format: "uuid"},
					"name":        {Type: "string", Example: "reports/q3.pdf"},
					"size":        {Type: "integer", Format: "int64", Example: 1048576},
					"mime_type":   {Type: "string", Example: "application/pdf"},
					"uploaded_by": {Type: "string", Format: "uuid"},
					"created_at":  {Type: "string", Format: "date-time"},
					"updated_at":  {Type: "string", Format: "date-time"},
				},
			},
			"Policy": {
				Type: "object",
				Properties: map[string]*Schema{
					"id":         {Type: "string", Format: "uuid"},
					"table_name": {Type: "string", Example: "documents"},
					"action":     {Type: "string", Example: "SELECT"},
					"role":       {Type: "string", Example: "user"},
					"expression": {Type: "string", Example: "user.id == row.owner_id"},
					"created_at": {Type: "string", Format: "date-time"},
					"updated_at": {Type: "string", Format: "date-time"},
				},
			},
			"FunctionMeta": {
				Type: "object",
				Properties: map[string]*Schema{
					"name":          {Type: "string", Example: "hello"},
					"runtime":       {Type: "string", Example: "js"},
					"file_path":     {Type: "string", Example: "nodephone-data/functions/hello.js"},
					"timeout":       {Type: "string", Example: "5s"},
					"cron_schedule": {Type: "string", Example: "*/5 * * * *"},
					"created_at":    {Type: "string", Format: "date-time"},
				},
			},
			"EventMessage": {
				Type: "object",
				Properties: map[string]*Schema{
					"type":        {Type: "string", Example: "publish"},
					"room":        {Type: "string", Example: "global"},
					"payload":     {Type: "object"},
					"sender_id":   {Type: "string", Format: "uuid"},
					"sender_name": {Type: "string", Example: "sagar"},
					"timestamp":   {Type: "string", Format: "date-time"},
				},
			},
			"HealthResponse": {
				Type: "object",
				Properties: map[string]*Schema{
					"status":    {Type: "string", Example: "ok"},
					"timestamp": {Type: "string", Format: "date-time"},
				},
			},
			"VersionResponse": {
				Type: "object",
				Properties: map[string]*Schema{
					"version":    {Type: "string", Example: "v0.1.0-dev"},
					"go_version": {Type: "string", Example: "go1.25.0"},
					"os":         {Type: "string", Example: "windows"},
					"arch":       {Type: "string", Example: "amd64"},
				},
			},
			"ReadyResponse": {
				Type: "object",
				Properties: map[string]*Schema{
					"status": {Type: "string", Example: "ready"},
					"checks": {Type: "object"},
				},
			},
		},
	}
}
