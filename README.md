<div align="center">

# SolderDB

**A local-first single-binary database.** LSM engine + REST + auth + collections + realtime + files — built from scratch in Go, wrapped in a desktop admin GUI.

<sub>Precision · Connection · Data</sub>

[Quickstart](#quickstart) · [Features](#features) · [Why](#why-this-exists) · [SDKs](#sdks) · [REST API](#rest-api) · [Architecture](#architecture) · [Develop](#develop)

</div>

---

## Quickstart

**1. Download** the latest `SolderDB.exe` from [Releases](https://github.com/N9601/SolderDB/releases).

**2. Run it.** A native window opens. Create the first account — that user becomes the admin of the instance. Data is stored under `%APPDATA%\SolderDB` (Windows) or `~/.config/SolderDB` (Linux/macOS).

**3. Hit it from code:**

```bash
npm install solderdb
```

```ts
import { SolderDB } from "solderdb";

const db = new SolderDB("http://localhost:8787");
await db.auth.login("you@example.com", "supersecret");

type Note = { title: string; pinned?: boolean };
const notes = db.collection<Note>("notes");

await notes.create({ title: "hello" });
notes.subscribe(evt => console.log(evt.kind, evt.id));
```

Or `curl`, or the Go SDK, or the included `solderdb` CLI, or the in-app **API Explorer**. The whole product is one local HTTP server on `127.0.0.1:8787` plus a desktop admin UI that talks to it.

## Why this exists

PocketBase and Supabase are amazing, but they pin you to SQLite/Postgres. SolderDB is for people who want the same developer experience — *typed collections, auth, realtime, file storage, admin UI* — on a storage engine they own end-to-end.

The engine is a from-scratch Log-Structured Merge tree (Memtable + WAL + SSTables + compaction) with **zero third-party database dependencies**. Every byte that touches disk is code you can audit in this repo.

It runs as a single binary on your machine. No SaaS, no cluster, no cloud account. Nothing leaves your box.

## Features

### Core engine
- Hand-built LSM tree — in-memory `Memtable`, append-only `WAL` with CRC32C per record, sorted immutable `SSTables` with one bloom filter per file, manual + scheduled compaction
- Crash recovery via WAL replay; torn-tail records ignored, not corrupted
- **Hardware-aware compaction** — pauses heavy disk work when your laptop is on battery or thermal-throttled (Windows/Linux/macOS native readers)
- Live `Lifecycle` view in the GUI shows the LSM behaving in real time

### Data model
- **Collections** layer on top of the KV — typed records (`text` / `number` / `bool` / `json` / `date`), required-field validation, time-sortable ULID-style IDs, schema editor
- **Per-operation access rules** per collection — `public` / `authed` / `admin` on each of `list / view / create / update / delete`
- Internal `_*` collections (users, files) are admin-only over the API regardless of stored rules

### Networking / API
- Embedded REST API on `127.0.0.1:8787` — every UI capability is reachable from any client
- **Realtime** via Server-Sent Events — subscribe to collection-wide or per-record topics
- **Auth** — bcrypt hashes, HMAC-SHA256 signed session tokens, role-based middleware (admin / authed / public)
- **File storage** — multipart upload, SHA-256 hashed, served with content-type from disk
- **CORS-enabled** for local web development

### Developer surface
- **JavaScript / TypeScript SDK** (`npm install solderdb`) — auth, typed collection clients, file upload, realtime subscribe
- **Go SDK** — stdlib-only client mirroring the JS one
- **CLI** (`solderdb`) — terminal client for scripting (login, coll/rec/file CRUD, logs tail)
- Generated Wails bindings for in-app TS callers

### Admin GUI
- Sleek dark-sidebar / light-workspace layout (with dark mode)
- **Cmd-K command palette** — fuzzy nav + actions, keyboard-driven
- **Activity log** — live tail of every API request streamed via SSE
- **API Explorer** — Postman-style request builder for every endpoint
- **Lifecycle visualizer** — animated LSM tree, write-spark indicators, SSTable blocks
- **Hardware monitor** — battery, CPU temp, throttle status with editable thresholds
- Boot splash, page transitions, animated counters, toast notifications, profile view

## SDKs

### JavaScript / TypeScript

```ts
import { SolderDB } from "solderdb";

const db = new SolderDB("http://localhost:8787");

// Register as the first user → becomes admin
await db.auth.register("admin@example.com", "supersecret");

// Define a collection (admin only)
await db.admin.createCollection({
  name: "notes",
  fields: [
    { name: "title", type: "text", required: true },
    { name: "pinned", type: "bool" }
  ]
});

// Use it
type Note = { title: string; pinned?: boolean };
const notes = db.collection<Note>("notes");

const created = await notes.create({ title: "hello", pinned: true });
const all = await notes.list({ limit: 50 });
await notes.update(created.id, { pinned: false });
await notes.delete(created.id);

// Real-time
const stop = notes.subscribe(evt => console.log(evt.kind, evt.id));
// stop()

// Files
const meta = await db.files.upload(myFile);
const url = db.files.url(meta.id); // includes ?token=... for <img src>
```

Source: [`sdk/solderdb-js/`](./sdk/solderdb-js/).

### Go

```bash
go get github.com/N9601/SolderDB/sdk/solderdb-go
```

```go
import "github.com/N9601/SolderDB/sdk/solderdb-go"

c := solderdb.New("http://localhost:8787")
_, _ = c.Auth.Login(ctx, "you@example.com", "supersecret")

notes := c.Collection("notes")
doc, _ := notes.Create(ctx, map[string]any{"title": "hi"})

stop, _ := notes.Subscribe(ctx, func(evt solderdb.Event) {
    fmt.Println(evt.Kind, evt.ID)
})
defer stop()
```

Source: [`sdk/solderdb-go/`](./sdk/solderdb-go/).

### CLI

```bash
go install github.com/N9601/SolderDB/cmd/solderdb-cli@latest

solderdb register you@example.com supersecret
solderdb coll create notes title:text:required pinned:bool
solderdb rec add notes title="hello" pinned=true
solderdb rec ls notes
solderdb logs 20
```

Source: [`cmd/solderdb-cli/`](./cmd/solderdb-cli/).

## REST API

The complete surface lives on `127.0.0.1:8787`. See the in-app **API Explorer** for an interactive reference.

```
GET    /api/health                                public
POST   /api/auth/register                         public — first user becomes admin
POST   /api/auth/login                            public
GET    /api/auth/me                               authed
POST   /api/auth/password                         authed

GET    /api/collections                           authed
POST   /api/collections                           admin
GET    /api/collections/:name                     authed
PATCH  /api/collections/:name                     admin
DELETE /api/collections/:name                     admin

GET    /api/collections/:name/records             per-collection listRule
POST   /api/collections/:name/records             per-collection createRule
GET    /api/collections/:name/records/:id         per-collection viewRule
PATCH  /api/collections/:name/records/:id         per-collection updateRule
DELETE /api/collections/:name/records/:id         per-collection deleteRule

GET    /api/files                                 authed
POST   /api/files (multipart or X-Filename raw)   authed
GET    /api/files/:id                             authed (?token= allowed)
DELETE /api/files/:id                             admin

GET    /api/kv/:key                               authed
PUT    /api/kv/:key                               admin
DELETE /api/kv/:key                               admin

GET    /api/realtime?topic=…                      authed (?token= allowed)
GET    /api/logs                                  admin
GET    /api/stats                                 admin
```

Tokens are bearer-style in `Authorization: Bearer <token>`. For browser-initiated requests that can't set headers (`<img>`, `EventSource`, downloads) the server also accepts `?token=<token>` on the file + realtime endpoints.

## Architecture

```
                    ┌──────────────────────────────────────┐
                    │  Wails desktop window (React + TS)   │
                    │  · Cmd-K palette · Lifecycle view    │
                    │  · Dashboard · Logs · API Explorer   │
                    └────────────┬─────────────────────────┘
                                 │  Wails-generated TS bindings
                    ┌────────────▼─────────────────────────┐
                    │  bridge package (Go)                 │
                    │  DBService · CollectionsService ·    │
                    │  AuthService · HardwareService       │
                    └────────────┬─────────────────────────┘
                                 │
                ┌────────────────▼────────────────┐    ┌─────────────────────┐
                │  REST API (127.0.0.1:8787)      │◄───┤  JS / Go SDK / CLI  │
                │  net/http only, auth middleware │    └─────────────────────┘
                └────────────────┬────────────────┘
                                 │
              ┌──────────────────┼──────────────────┐
              │                  │                  │
     ┌────────▼────────┐ ┌───────▼─────────┐ ┌──────▼─────────┐
     │  collections    │ │  files          │ │  auth          │
     │  + rules + idx  │ │  blob storage   │ │  bcrypt + HMAC │
     └────────┬────────┘ └────────┬────────┘ └──────┬─────────┘
              └───────────────────┼────────────────┘
                                  │
                  ┌───────────────▼────────────────┐
                  │  engine (Go, stdlib only)      │
                  │  ┌──────────┐                  │
                  │  │ Memtable │   in-memory map  │
                  │  └────┬─────┘                  │
                  │       │                        │
                  │  ┌────▼──────┐  CRC32C/record  │
                  │  │ WAL       │  fsync per put  │
                  │  └────┬──────┘                 │
                  │       │ flush @ 1 MB           │
                  │  ┌────▼──────┐  bloom filter   │
                  │  │ SSTables  │  per file       │
                  │  └────┬──────┘                 │
                  │       │ gate: battery / temp   │
                  │  ┌────▼──────┐                 │
                  │  │ Compactor │  manual + auto  │
                  │  └───────────┘                 │
                  └────────────────────────────────┘
```

Storage path: `$XDG_CONFIG_HOME/SolderDB/` on Linux, `~/Library/Application Support/SolderDB/` on macOS, `%APPDATA%\SolderDB\` on Windows.

## Develop

Prereqs: Go 1.22+, Node 18+, [Wails CLI v2](https://wails.io/docs/gettingstarted/installation).

```bash
git clone https://github.com/N9601/SolderDB.git
cd SolderDB
wails dev
```

Run the test suite:

```bash
go test ./...
cd frontend && npx tsc --noEmit && npm run build
```

Build a release binary + Windows installer:

```bash
wails build -clean -nsis
```

Regenerate the app icon (after editing `tools/genicon/main.go`):

```bash
go run ./tools/genicon
```

Project layout:

```
SolderDB/
├── main.go                        Wails entry, wires everything up
├── internal/
│   ├── engine/                    LSM core — Memtable, WAL, SSTable, compaction
│   ├── collections/               Typed-record layer over KV
│   ├── auth/                      bcrypt + HMAC sessions
│   ├── files/                     Blob storage on disk
│   ├── realtime/                  In-process pub/sub hub
│   ├── hardware/                  Per-OS battery + thermal monitors
│   ├── logs/                      Ring buffer of recent requests
│   ├── api/                       REST server + auth middleware
│   └── bridge/                    Wails service adapters (DTOs)
├── cmd/solderdb-cli/              Terminal client
├── sdk/
│   ├── solderdb-js/               npm package
│   └── solderdb-go/               Go module
├── frontend/src/
│   ├── App.tsx                    AppShell, routing, sidebar
│   ├── views/                     Dashboard · Lifecycle · Collections · Files · ...
│   ├── components/                Logo · BootSplash · CommandPalette · Toast · CountUp
│   └── lib/                       apiFetch · theme
├── tools/genicon/                 Procedural icon renderer
└── docs/                          Long-form documentation
```

## License

MIT. See [LICENSE](./LICENSE).

## Status

**v1.0.0** — first stable release. Engine, collections, auth, REST, realtime, files, SDKs, CLI, admin GUI. See [CHANGELOG.md](./CHANGELOG.md) for detail.
