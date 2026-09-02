# NodePhone Ecosystem Monorepo

Welcome to the **NodePhone Platform** monorepo repository. This folder contains all core modules, SDKs, applications, and documentation repositories for NodePhone.

## Project Structure & Repositories

| Local Directory | Component | Primary Tech Stack | Purpose |
| :--- | :--- | :--- | :--- |
| [`server/`](./server) | **NodePhone Server Kernel** | Go, SQLite, WebSockets, Goja | Core backend kernel engine (Auth, DB, Storage, Realtime, Functions, Permissions, OpenAPI, Deploy, Backup) |
| [`cli/`](./cli) | **NodePhone CLI** | Go / Cobra | Command line tool (`nodephone` / `agy`) for local dev, deployment, and management |
| [`studio/`](./studio) | **NodePhone Studio UI** | HTML / JS / React / Vite | Web admin dashboard for managing databases, storage, functions, policies, and realtime logs |
| [`website/`](./website) | **NodePhone Website** | HTML / CSS / JS | Marketing portal, landing page, and developer ecosystem website |
| [`android/`](./android) | **NodePhone Android App** | Kotlin / Android SDK | Mobile native application for NodePhone remote management and realtime notifications |
| [`sdk-js/`](./sdk-js) | **JavaScript / TS SDK** | TypeScript / JavaScript | Official Client SDK for Web, Node.js, React, and React Native |
| [`sdk-flutter/`](./sdk-flutter) | **Flutter SDK** | Dart / Flutter | Official Client SDK for iOS, Android, Desktop, and Web Flutter apps |
| [`sdk-python/`](./sdk-python) | **Python SDK** | Python 3 | Official Client SDK for Python scripts, AI pipelines, and backend servers |
| [`docs/`](./docs) | **Documentation Portal** | Markdown / VitePress | Developer guides, API reference, PRD specifications, and architecture docs |

---

## Working on Specific Modules in Antigravity

Each top-level directory is structured as a self-contained module. You can open any individual subfolder directly in **Antigravity** (e.g. `File -> Open Folder -> server/` or `File -> Open Folder -> studio/`) to focus on a specific project area.
