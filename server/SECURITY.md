# Security Policy - NodePhone Server

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.0.x   | :white_check_mark: |
| < 1.0.0 | :x:                |

## Reporting a Vulnerability

The NodePhone Security Team takes security issues seriously. If you discover a vulnerability in NodePhone Server, please do **NOT** open a public GitHub issue.

### How to Report
Please send an email detailing your discovery to `security@nodephone.dev` with:
- Description of the vulnerability and impact.
- Steps to reproduce or proof-of-concept payload.
- Recommended mitigation if available.

We will acknowledge receipt within **24 hours** and provide periodic updates until a fix is released.

## Security Practices in NodePhone Server
- **Password Hashing**: Argon2id with cryptographically secure random salts.
- **Tokens**: Short-lived JWT access tokens (15 mins) & refresh tokens (7 days).
- **Storage Safety**: Boundary verification against path traversal attempts.
- **Access Control**: Row-Level Security (RLS) policies with default-deny evaluation.
- **Disaster Recovery**: AES-256-GCM encrypted backup archives with SHA-256 integrity checksums.
