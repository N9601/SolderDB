# Changelog

All notable changes are tracked here. SolderDB follows [Semantic Versioning](https://semver.org/).

## [1.0.0] — 2026-05-27

First stable release. Everything below is included.

### Engine
- Log-Structured Merge tree built from scratch in Go, no third-party DB libs
- Memtable (`map[string]memEntry` + `sync.RWMutex`), append-only WAL with CRC32C per record + fsync on each write, sorted immutable SSTables with one bloom filter per file
- Crash recovery via WAL replay; torn-tail records are detected and ignored
- 1 MB flush threshold; manual `Compact()` merges all SSTables into one
- Range scans with explicit `Start` (inclusive) / `End` (exclusive) bounds
- Snapshot API copies the WAL + every SSTable to a timestamped folder under `<dataDir>/snapshots/`
- Hardware-aware compaction — pluggable `CompactionGate`; engine consults per-OS battery/thermal monitor before starting heavy disk work

### Collections
- Typed-record layer on top of the KV (`text` / `number` / `bool` / `json` / `date`)
- Required-field validation; time-sortable ULID-style IDs; merge-patch updates
- Per-operation access rules per collection: `public` / `authed` / `admin` on `list / view / create / update / delete`
- Internal `_users` and `_files` collections always admin-only over the API
- Schema editor in the GUI (add / remove / retype fields, edit rules)
- Realtime events published on `coll:<name>:<id>` topics

### Auth
- `_users` collection with bcrypt password hashes (cost 12) and roles (`admin` / `user`)
- First registered user is granted admin
- HMAC-SHA256 signed session tokens with 7-day lifetime; secret persisted to `<dataDir>/.secret`
- Password change endpoint
- Bearer-token middleware with route policies (`public` / `authed` / `admin`)
- Query-param `?token=` fallback on SSE and file GET routes for browser EventSource and `<img>` use

### REST API
- Embedded HTTP server on `127.0.0.1:8787`, stdlib-only (`net/http`)
- Collections CRUD, records CRUD, files CRUD, KV CRUD, stats, logs, auth, realtime, health
- CORS enabled for local web development (`Access-Control-Allow-Origin: *`)
- Consistent JSON error shape `{ "error": "message" }`

### Realtime
- In-process pub/sub hub with topic-prefix matching (`coll:notes` receives `coll:notes:abc` events)
- Server-Sent Events endpoint `/api/realtime?topic=...`
- Buffered subscriber channels — slow consumers drop events rather than block writers

### Files
- Blob storage under `<dataDir>/files/`, content-addressed by random ID
- Metadata stored in the internal `_files` collection — rides on engine snapshots
- SHA-256 hash and MIME type recorded on upload
- Multipart and raw uploads, streaming downloads, max 100 MB per file

### SDKs
- **JS / TS SDK** — `npm install solderdb`. Browser + Node 18+. Auth, typed collections, file upload, realtime via `EventSource`.
- **Go SDK** — stdlib-only client mirroring the JS one. SSE-based subscribe.
- **CLI** (`solderdb`) — login/register/whoami/stats/logs, collection + record + file CRUD; persists token under `~/.solderdb/token`

### Admin GUI
- Wails v2 desktop app (React + TypeScript + Tailwind)
- Two-tone design: dark gunmetal sidebar, light canvas workspace, copper accents
- Full **dark mode** with persisted preference + OS-respecting "system" option
- Boot splash animation
- **Cmd-K command palette** with fuzzy search across nav + actions
- Toast notifications, animated counters, page transitions, skeleton loaders
- Collapsible sidebar with `G+digit` keyboard shortcuts
- Profile page (account, change password, theme, current session token)
- Lifecycle visualizer — animated LSM tree showing Memtable fill, WAL growth, SSTable blocks
- Hardware-aware Compaction card with editable thresholds
- API Explorer — Postman-style interactive request builder
- Activity logs view — live tail of every API request via SSE
- Custom app icon (procedurally rendered via `tools/genicon`)

### Tooling
- `tools/genicon/` — stdlib-only Go renderer producing `build/appicon.png` + multi-size `build/windows/icon.ico`
- Tests across engine, collections, api, auth, files, realtime, logs, hardware packages
- Concurrent read/write stress test in the engine

### Known limitations
- No JOINs, GROUP BY, or aggregates beyond what collection listing provides
- No SQL (intentional — SolderDB is NoSQL; see roadmap for SolderQL)
- No P2P or multi-node replication (see roadmap)
- Linux/macOS supported by the engine but installer is Windows-only in v1
