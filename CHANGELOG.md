# Changelog

All notable changes are tracked here. SolderDB follows [Semantic Versioning](https://semver.org/).

## [1.0.0] - 2026-05-27

First stable release. Everything below is included.

### Engine
- Log-Structured Merge tree built from scratch in Go, no third-party DB libraries
- Memtable (`map[string]memEntry` + `sync.RWMutex`), append-only WAL with CRC32C per record and fsync on each write, sorted immutable SSTables with one bloom filter per file
- Crash recovery via WAL replay. Torn-tail records are detected and ignored
- 1 MB flush threshold. Manual `Compact()` merges all SSTables into one
- Range scans with explicit `Start` (inclusive) and `End` (exclusive) bounds
- Snapshot API copies the WAL and every SSTable to a timestamped folder under `<dataDir>/snapshots/`
- Hardware-aware compaction with a pluggable `CompactionGate`. The engine consults a per-OS battery and thermal monitor before starting heavy disk work

### Collections
- Typed-record layer on top of the KV (`text`, `number`, `bool`, `json`, `date`)
- Required-field validation. Time-sortable ULID-style IDs. Merge-patch updates
- Per-operation access rules per collection: `public`, `authed`, or `admin` on each of `list`, `view`, `create`, `update`, `delete`
- Internal `_users` and `_files` collections are always admin-only over the API
- Schema editor in the GUI (add, remove, retype fields, edit rules)
- Realtime events published on `coll:<name>:<id>` topics

### Auth
- `_users` collection with bcrypt password hashes (cost 12) and roles (`admin` or `user`)
- First registered user is granted admin
- HMAC-SHA256 signed session tokens with 7-day lifetime. Secret persisted to `<dataDir>/.secret`
- Password change endpoint
- Bearer-token middleware with route policies (`public`, `authed`, `admin`)
- Query-param `?token=` fallback on SSE and file GET routes for browser EventSource and `<img>` use

### REST API
- Embedded HTTP server on `127.0.0.1:8787`, stdlib-only (`net/http`)
- Collections CRUD, records CRUD, files CRUD, KV CRUD, stats, logs, auth, realtime, health
- CORS enabled for local web development (`Access-Control-Allow-Origin: *`)
- Consistent JSON error shape `{ "error": "message" }`

### Realtime
- In-process pub/sub hub with topic-prefix matching (`coll:notes` receives `coll:notes:abc` events)
- Server-Sent Events endpoint `/api/realtime?topic=...`
- Buffered subscriber channels. Slow consumers drop events rather than block writers

### Files
- Blob storage under `<dataDir>/files/`, content-addressed by random ID
- Metadata stored in the internal `_files` collection so files ride on engine snapshots
- SHA-256 hash and MIME type recorded on upload
- Multipart and raw uploads, streaming downloads, max 100 MB per file

### SDKs
- **JS / TS SDK**. `npm install solderdb`. Browser and Node 18+. Auth, typed collections, file upload, realtime via `EventSource`.
- **Go SDK**. Stdlib-only client mirroring the JS one. SSE-based subscribe.
- **CLI** (`solderdb`). login, register, whoami, stats, logs, plus collection / record / file CRUD. Persists token under `~/.solderdb/token`

### Admin GUI
- Wails v2 desktop app (React + TypeScript + Tailwind)
- Two-tone design: dark gunmetal sidebar, light canvas workspace, copper accents
- Full **dark mode** with persisted preference and OS-respecting "system" option
- Boot splash animation
- **Cmd+K command palette** with fuzzy search across nav and actions
- Toast notifications, animated counters, page transitions, skeleton loaders
- Collapsible sidebar with `G+digit` keyboard shortcuts
- Profile page (account, change password, theme, current session token)
- Lifecycle visualizer with animated LSM tree showing Memtable fill, WAL growth, SSTable blocks
- Hardware-aware Compaction card with editable thresholds
- API Explorer with Postman-style interactive request builder
- Activity logs view with live tail of every API request via SSE
- Custom app icon (procedurally rendered via `tools/genicon`)

### Tooling
- `tools/genicon/`. Stdlib-only Go renderer producing `build/appicon.png` and multi-size `build/windows/icon.ico`
- Tests across engine, collections, api, auth, files, realtime, logs, hardware packages
- Concurrent read/write stress test in the engine

### Known limitations
- No JOINs, GROUP BY, or aggregates beyond what collection listing provides
- No SQL (intentional. SolderDB is NoSQL. See roadmap for SolderQL)
- No P2P or multi-node replication (see roadmap)
- Linux and macOS supported by the engine, but the installer is Windows-only in v1
