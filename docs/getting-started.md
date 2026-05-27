---
title: Getting Started
order: 1
---

# Getting started

## Install

### Pre-built binary (Windows)

Download the latest `SolderDB.exe` from the [Releases](https://github.com/N9601/SolderDB/releases) page. Run it. That's all.

### Build from source

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
git clone https://github.com/N9601/SolderDB.git
cd SolderDB
wails build
```

The output binary lives in `build/bin/`.

## First run

When you launch SolderDB for the first time, an empty database is created and you're prompted to register the first user. **That user is granted the `admin` role.** Subsequent registrations become regular `user`s.

You can change roles by editing the `_users` collection directly via the admin GUI (Collections can't show it, but the engine knows) or by writing a small CLI snippet. For v1 we don't expose user role editing in the UI.

## Your first collection

In the GUI:

1. Click **Collections** in the sidebar
2. Click the **+** in the collections panel
3. Name it `notes`, add fields:
   - `title` (text, required)
   - `pinned` (bool)
4. Save

Switch to the new collection, click **+ New record**, fill in a title, hit **Insert**. You should see the row appear and the `LIVE` chip pulse copper for a moment.

## Connecting from code

The local server runs at `http://127.0.0.1:8787` by default. To talk to it from a JavaScript / TypeScript app:

```bash
npm install solderdb
```

```ts
import { SolderDB } from "solderdb";

const db = new SolderDB("http://localhost:8787");
await db.auth.login("you@example.com", "supersecret");

type Note = { title: string; pinned?: boolean };
const notes = db.collection<Note>("notes");

const list = await notes.list({ limit: 50 });
console.log(list.records);
```

Or from Go:

```go
import "github.com/N9601/SolderDB/sdk/solderdb-go"

c := solderdb.New("http://localhost:8787")
c.Auth.Login(ctx, "you@example.com", "supersecret")

notes := c.Collection("notes")
doc, _ := notes.Create(ctx, map[string]any{"title": "hello"})
```

Or with `curl`:

```bash
TOKEN=$(curl -s -X POST http://localhost:8787/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"supersecret"}' | jq -r .token)

curl -X POST http://localhost:8787/api/collections/notes/records \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"from curl"}'
```

## Data location

SolderDB stores everything under a single directory:

| OS      | Path                                       |
|---------|--------------------------------------------|
| Windows | `%APPDATA%\SolderDB\`                      |
| Linux   | `~/.config/SolderDB/`                      |
| macOS   | `~/Library/Application Support/SolderDB/`  |

Contents:

```
SolderDB/
├── wal.bin          Write-ahead log (binary, append-only)
├── sstables/        Immutable sorted-string tables
├── files/           Uploaded file blobs
├── snapshots/       Created via the Snapshot button
└── .secret          HMAC signing key (32 random bytes, do not share)
```

Back up the whole directory to back up the database. Restore by replacing it.

## Next steps

- [Collections & records](./collections.md) for the full schema, validation, and rule story
- [REST API reference](./rest-api.md) for every endpoint
- [Engine internals](./engine.md) if you want to understand how SolderDB stores your data
