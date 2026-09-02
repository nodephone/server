# Contributing to NodePhone Server

Thank you for your interest in contributing to the **NodePhone Server Kernel** (`nodephone/server`)!

## Development Workflow

1. **Fork and Clone**
   ```bash
   git clone https://github.com/nodephone/server.git
   cd server
   ```

2. **Branching Model**
   - Create a feature branch: `git checkout -b feature/my-new-feature`
   - Create a fix branch: `git checkout -b fix/issue-description`

3. **Running Code & Tests**
   - Run server: `go run ./cmd/nodephone`
   - Run test suite: `go test -v -cover ./...`

4. **Code Quality & Commit Standards**
   - Ensure all code is formatted using `gofmt`.
   - Maintain unit and integration tests for new functionality.
   - Write clear, imperative commit messages (`feat: ...`, `fix: ...`, `docs: ...`).

5. **Pull Requests**
   - Submit PRs against the `main` branch.
   - All automated GitHub Actions CI tests must pass cleanly before merging.
