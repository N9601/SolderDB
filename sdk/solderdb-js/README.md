# solderdb

JavaScript / TypeScript SDK for [SolderDB](https://github.com/N9601/SolderDB), a local-first single-binary database.

Works in browsers and Node 18+. No runtime dependencies.

## Install

```bash
npm install solderdb
```

## Quick start

```ts
import { SolderDB } from "solderdb";

const db = new SolderDB("http://localhost:8787");

// First-ever user becomes admin
await db.auth.register("you@example.com", "supersecret");

// Define a collection (admin-only)
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
const stop = notes.subscribe((evt) => {
  console.log(evt.kind, evt.id, evt.data);
});
// later: stop();

// Files
const meta = await db.files.upload(new File([blob], "diagram.png"));
const imageURL = db.files.url(meta.id); // includes ?token=... for <img src>
```

## API

- `db.auth.register(email, password)` · `login` · `me` · `logout`
- `db.collection<T>(name).list({after, limit})` · `get(id)` · `create(data)` · `update(id, patch)` · `delete(id)` · `subscribe(handler)`
- `db.admin.listCollections()` · `createCollection(meta)` · `deleteCollection(name)` · `stats()`
- `db.files.list()` · `upload(file)` · `delete(id)` · `url(id)`

All methods return promises. Errors are thrown as `SolderDBError` with a `.status` field matching the HTTP code.
