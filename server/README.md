# NodePhone Server Kernel (`nodephone/server`)

> **Version:** `v0.1.0-dev`  
> **Repository:** `nodephone/server`  
> **Language:** Go 1.22+  
> **Database:** SQLite3 (WAL Mode, Foreign Keys ON)  

**NodePhone Server** is the core backend engine powering the NodePhone backend-as-a-service platform. It provides an embedded SQLite database engine, Argon2id authentication with JWT session management, disk-backed streaming storage, realtime WebSocket hubs, Goja JavaScript serverless function execution, fine-grained row-level permissions, automatic OpenAPI 3.1 spec generation, secure global HTTPS tunneling, and encrypted disaster recovery backups.

---

## 🚀 Quick Start

### Prerequisites
- [Go 1.22+](https://go.dev/dl/)

### Running the Server locally

```bash
# Clone or navigate to the server repository
cd server

# Run the NodePhone Server kernel
go run ./cmd/nodephone
```

Upon boot, NodePhone initializes the `nodephone-data/` directory structure, generates a cryptographically secure JWT secret key in `nodephone-data/secrets/jwt.key`, executes SQLite migrations, starts the realtime WebSocket hub, opens a secure HTTPS tunnel, and starts listening on `http://localhost:8080` (or configured port in `nodephone-data/config.json`).

---

## 🛠 Subsystem Architecture

| Subsystem | Package Path | Purpose & Features |
| :--- | :--- | :--- |
| **Configuration Engine** | [`internal/config`](./internal/config) | Auto-creates `nodephone-data/` directory structure, manages `config.json`, and loads JWT secrets. |
| **Database Engine** | [`internal/database`](./internal/database) | Embedded SQLite connection manager with WAL mode, foreign keys, and versioned migrations. |
| **Authentication Engine** | [`internal/auth`](./internal/auth) | Argon2id password hashing, JWT access/refresh token pairs, session tracking, and API keys. |
| **Storage Engine** | [`internal/storage`](./internal/storage) | Disk-backed bucket storage, multipart file streaming, path safety sanitization, and signed URLs. |
| **Realtime Engine** | [`internal/realtime`](./internal/realtime) | WebSocket hub supporting room creation, message broadcasts, heartbeat keepalives, and online presence tracking. |
| **Functions Engine** | [`internal/functions`](./internal/functions) | Auto-discovers JS functions in `nodephone-data/functions`, executes code via Goja JS engine, and isolates panics. |
| **Permissions Engine** | [`internal/permissions`](./internal/permissions) | Row-Level Security (RLS) policy evaluator supporting default-deny access control for database rows. |
| **OpenAPI Engine** | [`internal/openapi`](./internal/openapi) | Compiles OpenAPI 3.1.0 JSON specifications and serves embedded interactive RapiDoc documentation. |
| **Deployment Engine** | [`internal/deploy`](./internal/deploy) | Manages secure reverse HTTPS tunnels (`https://*.nodephone.dev`), TLS handling, and custom domains. |
| **Backup Engine** | [`internal/backup`](./internal/backup) | Creates AES-256-GCM encrypted `.npb` Zip backups containing database, storage, functions, and config with SHA-256 integrity verification. |

---

## 📡 API Endpoints Summary

### System & Metadata
- `GET /` - Server status & runtime metadata
- `GET /health` - Liveness health probe
- `GET /version` - System release version & Go runtime info
- `GET /ready` - Readiness probe (Config, DB, Storage checks)

### Authentication (`/api/auth`)
- `POST /api/auth/signup` - Register a new user account
- `POST /api/auth/login` - Authenticate & obtain JWT access + refresh tokens
- `POST /api/auth/logout` - Revoke active user session
- `POST /api/auth/refresh` - Refresh access token pair
- `GET /api/auth/me` - Fetch authenticated user profile
- `POST /api/auth/keys` - Issue a new API key

### Storage (`/api/storage`)
- `POST /api/storage/buckets` - Create storage bucket
- `GET /api/storage/buckets` - List buckets
- `DELETE /api/storage/buckets/{name}` - Delete bucket
- `POST /api/storage/buckets/{b}/objects` - Multipart upload object
- `GET /api/storage/buckets/{b}/objects/{n}` - Stream download object
- `DELETE /api/storage/buckets/{b}/objects/{n}` - Delete object
- `POST /api/storage/buckets/{b}/objects/{n}/sign` - Issue signed access URL

### Realtime Engine (`/realtime`)
- `WS /realtime?token=<jwt>` - Realtime WebSocket connection hub
- `GET /api/realtime/presence` - Query connected online users

### Serverless Functions (`/api/functions`)
- `GET /api/functions` - List discovered JS functions
- `ALL /api/functions/{name}` - Execute serverless function endpoint

### Row-Level Permissions (`/api/permissions`)
- `POST /api/permissions/policies` - Create RLS policy rule (Admin)
- `GET /api/permissions/policies` - List policy rules (Admin)
- `DELETE /api/permissions/policies/{id}` - Delete policy rule (Admin)

### Interactive Documentation (`/docs`)
- `GET /docs` - Interactive RapiDoc API UI
- `GET /docs/openapi.json` - OpenAPI 3.1.0 specification document
- `GET /docs/routes` - Registered route metadata listing

### Deployment Engine (`/deploy`)
- `GET /deploy/status` - Live deployment status & public HTTPS URL
- `GET /deploy/health` - Remote tunnel health check
- `GET/POST /deploy/domain` - Custom domain binding & SSL status

### Backup & Disaster Recovery (`/api/backup`)
- `POST /api/backup/create` - Create atomic `.npb` backup archive (Admin)
- `POST /api/backup/restore` - Restore full server state from `.npb` snapshot (Admin)
- `GET /api/backup/list` - List available backup snapshots (Admin)
- `DELETE /api/backup/{id}` - Delete specified backup archive (Admin)
- `GET /api/backup/status` - Backup system health & retention metrics (Admin)

---

## 🧪 Running Automated Tests

Run the complete automated test suite (Unit Tests, API Tests, Realtime Tests, Functions Tests, and E2E Integration Tests):

```bash
go test -v -cover ./...
```

---

## 📁 Directory Structure

```text
server/
├── cmd/
│   └── nodephone/
│       └── main.go          # Server entry point
├── internal/
│   ├── api/                 # HTTP Router, middleware, & server handlers
│   ├── auth/                # Argon2id & JWT authentication engine
│   ├── backup/              # Backup snapshot, encryption, & restore engine
│   ├── config/              # Configuration & nodephone-data layout manager
│   ├── database/            # SQLite database connection & auto-migrations
│   ├── deploy/              # Reverse HTTPS tunnel & custom domain engine
│   ├── functions/           # Goja JS serverless function executor
│   ├── kernel/              # Core NodePhone kernel bootloader
│   ├── openapi/             # OpenAPI 3.1.0 specification generator
│   ├── permissions/         # Row-Level Security policy evaluator
│   ├── realtime/            # WebSocket hub & presence engine
│   ├── storage/             # Disk storage, streaming, & signed URLs
│   └── testing/             # Test environment helpers & utilities
├── tests/                   # Automated package test suites
│   ├── auth/
│   ├── database/
│   ├── functions/
│   ├── integration/
│   ├── realtime/
│   └── storage/
├── .github/
│   └── workflows/
│       └── ci.yml           # GitHub Actions CI workflow
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```