<div align="center">

<img src="./build/appicon.png" alt="SolderDB" width="140" />

# SolderDB

![Version](https://img.shields.io/badge/Version-1.0.0-e07a25?style=flat-square)
![Engine](https://img.shields.io/badge/Engine-LSM%20from%20scratch-5b6b8a?style=flat-square)
![Language](https://img.shields.io/badge/Go-1.22%2B-00ADD8?style=flat-square&logo=go&logoColor=white)
![Frontend](https://img.shields.io/badge/React-TS%20strict-61DAFB?style=flat-square&logo=react&logoColor=black)
![Wails](https://img.shields.io/badge/Wails-v2-DF0000?style=flat-square)
![License](https://img.shields.io/badge/License-MIT-16a34a?style=flat-square)
![Status](https://img.shields.io/badge/Status-Stable-blueviolet?style=flat-square)

### **A local-first single-binary database.**

LSM engine · REST · Auth · Collections · Realtime · Files · CLI · JS + Go SDKs — built from scratch in Go.

<sub>Precision · Connection · Data</sub>

[Quickstart](#quickstart) · [Features](#features) · [SDKs](#sdks) · [REST API](#rest-api) · [Architecture](#architecture) · [Docs](./docs/index.md) · [Roadmap](./docs/roadmap.md)

</div>

---

## Quickstart

**1. Download `SolderDB.exe`** from [Releases](https://github.com/N9601/SolderDB/releases) and run it. A native window opens. Register the first account — that user becomes the admin.

**2. Hit it from your code:**

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

Or hit it with `curl`, the [Go SDK](#go), the [`solderdb` CLI](#cli), or the in-app **API Explorer**. The whole product is one local HTTP server on `127.0.0.1:8787` plus a desktop admin UI.

---

## Why

PocketBase and Supabase pin you to SQLite/Postgres. SolderDB gives you the same DX — *typed collections, auth, realtime, files, admin UI* — on a storage engine **you own end-to-end**. From-scratch Log-Structured Merge tree, zero third-party DB libs. Every byte that touches disk is code you can audit in this repo.

Single binary. Your machine. Nothing leaves the box.

---

## Features

<table>
<tr>
<td width="50%" valign="top">

### 🔧 Engine
- **LSM tree** — Memtable + WAL + SSTables, hand-built
- **CRC32C** per WAL record; torn-tail aware
- **Bloom filter** per SSTable (~1% FPR)
- **Hardware-aware compaction** — pauses on low battery / hot CPU
- **Crash recovery** via WAL replay
- **Snapshots** — consistent disk-level copies

### 📚 Collections
- Typed records (`text` · `number` · `bool` · `json` · `date`)
- Schema editor with required + unique fields
- **Per-op access rules** — `public` / `authed` / `admin` on each of `list / view / create / update / delete`
- Time-sortable ULID-style IDs
- Range scans with `Start` / `End` bounds

### 🔐 Auth
- bcrypt + HMAC-SHA256 sessions
- First user → admin
- Role-based middleware (admin / authed / public)
- Password change endpoint

</td>
<td width="50%" valign="top">

### 🌐 REST & Realtime
- Embedded `net/http` server on `127.0.0.1:8787`
- **Server-Sent Events** for realtime subscriptions
- CORS-enabled for local web dev
- Activity log streamed live (admin)

### 📦 Files
- Multipart upload, SHA-256 hashed
- 100 MB per file
- Authenticated `?token=` URLs for `<img>` tags

### 🛠 Developer Surface
- **JS / TS SDK** (`npm install solderdb`)
- **Go SDK** (stdlib-only)
- **CLI** (`solderdb` binary)
- Wails-generated TS bindings

### 🎨 Admin GUI
- Dark sidebar + light workspace (with **dark mode**)
- **Cmd-K** command palette
- **Lifecycle visualizer** — live LSM animation
- **API Explorer** — Postman-style
- Activity logs, Profile, Snapshots, Hardware monitor
- Boot splash, animated counters, toast notifications

</td>
</tr>
</table>

---

## SDKs

### JavaScript / TypeScript

```bash
npm install solderdb
```

```ts
import { SolderDB } from "solderdb";

const db = new SolderDB("http://localhost:8787");

// First-ever user becomes admin
await db.auth.register("admin@example.com", "supersecret");

// Define a collection (admin only)
await db.admin.createCollection({
  name: "notes",
  fields: [
    { name: "title", type: "text", required: true },
    { name: "pinned", type: "bool" }
  ]
});

// CRUD
type Note = { title: string; pinned?: boolean };
const notes = db.collection<Note>("notes");

const doc = await notes.create({ title: "hello", pinned: true });
const list = await notes.list({ limit: 50 });
await notes.update(doc.id, { pinned: false });
await notes.delete(doc.id);

// Realtime
const stop = notes.subscribe(evt => {
  console.log(evt.kind, evt.id);
});
// stop()

// Files
const meta = await db.files.upload(myFile);
const src  = db.files.url(meta.id);  // includes ?token= for <img src>
```

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

### CLI

```bash
go install github.com/N9601/SolderDB/cmd/solderdb-cli@latest

solderdb register you@example.com supersecret
solderdb coll create notes title:text:required pinned:bool
solderdb rec  add notes title="hello" pinned=true
solderdb rec  ls  notes
solderdb logs 20
```

---

## REST API

Base URL: `http://127.0.0.1:8787`. Tokens in `Authorization: Bearer <token>` (or `?token=` for SSE / `<img>`).

```
GET    /api/health                             public
POST   /api/auth/register                      public · first user = admin
POST   /api/auth/login                         public
GET    /api/auth/me                            authed
POST   /api/auth/password                      authed

GET    /api/collections                        authed
POST   /api/collections                        admin
GET    /api/collections/:name                  authed
PATCH  /api/collections/:name                  admin
DELETE /api/collections/:name                  admin

GET    /api/collections/:name/records          per-collection rule
POST   /api/collections/:name/records          per-collection rule
GET    /api/collections/:name/records/:id      per-collection rule
PATCH  /api/collections/:name/records/:id      per-collection rule
DELETE /api/collections/:name/records/:id      per-collection rule

GET    /api/files                              authed
POST   /api/files     (multipart or raw)       authed
GET    /api/files/:id                          authed (?token= OK)
DELETE /api/files/:id                          admin

GET    /api/realtime?topic=...                 authed (?token= OK)
GET    /api/logs                               admin
GET    /api/stats                              admin
```

Full reference: [`docs/rest-api.md`](./docs/rest-api.md). Or open the **API Explorer** inside the app — every endpoint, interactive.

---

## Architecture

```
                    ┌──────────────────────────────────────┐
                    │  Wails desktop window (React + TS)   │
                    │  · Cmd-K · Lifecycle · API Explorer  │
                    └────────────┬─────────────────────────┘
                                 │ Wails-generated TS bindings
                    ┌────────────▼─────────────────────────┐
                    │  bridge package (Go)                 │
                    └────────────┬─────────────────────────┘
                                 │
                ┌────────────────▼────────────────┐    ┌─────────────────────┐
                │  REST API (127.0.0.1:8787)      │◄───┤  JS / Go SDK / CLI  │
                │  net/http + auth middleware     │    └─────────────────────┘
                └────────────────┬────────────────┘
                                 │
              ┌──────────────────┼──────────────────┐
              │                  │                  │
     ┌────────▼──────┐ ┌─────────▼──────┐ ┌────────▼────────┐
     │  collections  │ │     files      │ │      auth       │
     │   + rules     │ │   blob store   │ │  bcrypt + HMAC  │
     └────────┬──────┘ └────────┬───────┘ └────────┬────────┘
              └─────────────────┼──────────────────┘
                                │
                  ┌─────────────▼──────────────┐
                  │  engine (Go, stdlib only)  │
                  │  Memtable → WAL → SSTables │
                  │  Bloom · Compactor · Gate  │
                  └────────────────────────────┘
```

Storage path: `%APPDATA%\SolderDB\` (Windows) · `~/.config/SolderDB/` (Linux) · `~/Library/Application Support/SolderDB/` (macOS).

---

## Develop

```bash
git clone https://github.com/N9601/SolderDB.git
cd SolderDB
wails dev
```

Tests:

```bash
go test ./...
cd frontend && npx tsc --noEmit && npm run build
```

Build a release binary:

```bash
wails build -clean -platform windows/amd64
# output: build/bin/SolderDB.exe
```

Regenerate the app icon (after editing `tools/genicon/main.go`):

```bash
go run ./tools/genicon
```

Full release process: [`RELEASE.md`](./RELEASE.md).

---

## Project layout

```
SolderDB/
├── main.go                   Wails entry, wires everything up
├── internal/
│   ├── engine/               LSM core — Memtable, WAL, SSTable, compaction
│   ├── collections/          Typed-record layer
│   ├── auth/                 bcrypt + HMAC sessions
│   ├── files/                Blob storage
│   ├── realtime/             In-process pub/sub
│   ├── hardware/             Per-OS battery + thermal monitors
│   ├── logs/                 Ring buffer of recent requests
│   ├── api/                  REST server + middleware
│   └── bridge/               Wails service adapters
├── cmd/solderdb-cli/         Terminal client
├── sdk/
│   ├── solderdb-js/          npm package
│   └── solderdb-go/          Go module
├── frontend/src/             React + TS admin GUI
├── tools/genicon/            Procedural icon renderer
└── docs/                     Embedded markdown documentation
```

---

## License

MIT. See [LICENSE](./LICENSE).

---

<div align="center">

**v1.0.0** — first stable release.
Engine · Collections · Auth · REST · Realtime · Files · SDKs · CLI · Admin GUI.

[Changelog](./CHANGELOG.md) · [Roadmap](./docs/roadmap.md) · [Issues](https://github.com/N9601/SolderDB/issues)

</div>
