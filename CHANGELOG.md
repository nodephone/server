# Changelog - NodePhone Server Kernel

All notable changes to the NodePhone Server repository will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.0.0] - 2026-09-02

### Production Release (v1.0.0)

#### Features Included
- **Configuration Engine (`internal/config`)**: Auto-creates `nodephone-data/` layout, manages `config.json`, and handles secret persistence.
- **SQLite Database Engine (`internal/database`)**: Embedded SQLite database connection manager with WAL mode, foreign keys, and versioned schema auto-migrations.
- **Authentication Engine (`internal/auth`)**: Argon2id password hashing, JWT access/refresh token pairs, session management, and API key generation.
- **Storage Engine (`internal/storage`)**: Disk-backed bucket storage, multipart file streaming, path safety sanitization, and HMAC signed access URLs.
- **Realtime Engine (`internal/realtime`)**: WebSocket hub supporting rooms, broadcasts, heartbeat keepalives, and online user presence tracking.
- **Functions Engine (`internal/functions`)**: Auto-discovers JS functions, executes runtime logic via Goja JS interpreter, and isolates execution panics.
- **Permissions Engine (`internal/permissions`)**: Row-Level Security (RLS) policy evaluator supporting default-deny rules and Goja expression matching.
- **OpenAPI & SDK Generator (`internal/openapi`)**: Automatic OpenAPI 3.1.0 spec compiler and interactive RapiDoc documentation UI (`/docs`).
- **Deployment Engine (`internal/deploy`)**: Secure reverse HTTPS tunnels (`https://*.nodephone.dev`), automatic TLS certificate management, and custom domain binding.
- **Backup & Restore Engine (`internal/backup`)**: Atomic, AES-256-GCM encrypted `.npb` Zip archives with SHA-256 integrity verification, automated scheduler pruning (keeping last 30 backups), and full server restoration.
- **Testing Framework (`internal/testing`, `tests/`)**: Automated test suites across all packages and GitHub Actions CI workflow setup.
