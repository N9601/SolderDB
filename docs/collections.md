---
title: Collections & records
order: 2
---

# Collections & records

Collections are the typed-record layer that sits on top of the raw KV engine. Each collection has a name, a schema (list of fields), and access rules. Records are JSON documents validated against the schema.

## Storage

A collection's metadata lives at the KV key `_coll:meta:<name>`. Its records live at `_coll:rec:<name>:<id>`. IDs are 26-character time-sortable strings (ULID-like) so `Scan` returns records in chronological order by default.

## Field types

| Type     | JSON shape                          | Notes |
|----------|-------------------------------------|-------|
| `text`   | `string`                            | UTF-8, unlimited length (bound by record size) |
| `number` | `number`                            | Stored as `float64`; integers up to 2^53 are exact |
| `bool`   | `boolean`                           |  |
| `json`   | any JSON value (object/array/etc.)  | Stored verbatim |
| `date`   | ISO-8601 `string`                   | E.g. `"2026-05-27T12:34:56Z"` |

## Validation

Validation runs on every `Insert` and `Update`:

- **Required fields** must be present and non-empty (empty string counts as empty for `text` fields)
- **Type checks** match the declared field type
- **Unknown fields** (not in the schema) are rejected with a 400

The engine doesn't enforce uniqueness yet — `unique: true` on a field is accepted but not yet implemented. Roadmapped for v1.x via secondary indexes.

## Access rules

Each collection carries five rules:

```ts
{ listRule, viewRule, createRule, updateRule, deleteRule }
```

Each is one of:

- `public` — anyone, no auth required
- `authed` — any signed-in user
- `admin` — only `role: "admin"` users

Defaults are `authed` if you don't set them.

Internal collections (names starting with `_`, like `_users` and `_files`) are **always admin-only over the API** regardless of stored rules, so a misconfigured rule can't expose passwords.

## API

```http
GET    /api/collections                              List schemas
POST   /api/collections                              Create
GET    /api/collections/:name                        One schema
PATCH  /api/collections/:name                        Update schema (fields + rules)
DELETE /api/collections/:name                        Delete (cascades to records)

GET    /api/collections/:name/records?limit=&after=  List records
POST   /api/collections/:name/records                Insert
GET    /api/collections/:name/records/:id            Read
PATCH  /api/collections/:name/records/:id            Update (merge-patch)
DELETE /api/collections/:name/records/:id            Delete
```

Listing returns:

```json
{
  "records": [
    { "id": "01H...", "created": "...", "updated": "...", "data": { ... } }
  ],
  "nextAfter": "01H..."
}
```

Pass `nextAfter` back as `?after=` to paginate.

## Editing a schema

The schema editor in the admin GUI lets you add fields, remove fields, change types, change required-ness, and edit rules.

**Removing a field doesn't delete existing data** — the bytes stay on disk in the affected records but become invisible to validation. Run snapshots before destructive schema changes.

**Renaming a field is a remove + add** — there's no rename operation in v1.

## SDKs

```ts
// JS / TS
type Note = { title: string; pinned?: boolean };
const notes = db.collection<Note>("notes");

const all = await notes.list({ limit: 50 });
const one = await notes.get("01H...");
const created = await notes.create({ title: "hi" });
const updated = await notes.update(created.id, { pinned: true });
await notes.delete(created.id);
```

```go
// Go
notes := c.Collection("notes")
list, _ := notes.List(ctx, solderdb.ListOptions{Limit: 50})
doc, _ := notes.Create(ctx, map[string]any{"title": "hi"})
```
