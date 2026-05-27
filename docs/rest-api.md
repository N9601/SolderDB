---
title: REST API reference
order: 6
---

# REST API reference

Base URL: `http://127.0.0.1:8787` (default).

All responses are JSON. Errors use a consistent shape:

```json
{ "error": "human-readable message" }
```

Authentication via `Authorization: Bearer <token>` header, or `?token=<token>` query param on `/api/realtime` and `GET /api/files/:id`.

| Method | Path | Policy | Body / params | Returns |
|--------|------|--------|---------------|---------|
| GET | `/api/health` | public |  | `{ "ok": true, "service": "solderdb" }` |
| POST | `/api/auth/register` | public | `{ email, password }` | `Session` |
| POST | `/api/auth/login` | public | `{ email, password }` | `Session` |
| GET | `/api/auth/me` | authed |  | `User` |
| POST | `/api/auth/password` | authed | `{ current, next }` | `User` |
| GET | `/api/collections` | authed |  | `CollectionMeta[]` |
| POST | `/api/collections` | admin | `CollectionMeta` | `CollectionMeta` |
| GET | `/api/collections/:name` | authed |  | `CollectionMeta` |
| PATCH | `/api/collections/:name` | admin | `{ fields?, listRule?, viewRule?, createRule?, updateRule?, deleteRule? }` | `CollectionMeta` |
| DELETE | `/api/collections/:name` | admin |  | `{ deleted }` |
| GET | `/api/collections/:name/records` | rule | `?limit=&after=` | `{ records, nextAfter }` |
| POST | `/api/collections/:name/records` | rule | record data | `Document` |
| GET | `/api/collections/:name/records/:id` | rule |  | `Document` |
| PATCH | `/api/collections/:name/records/:id` | rule | partial record data | `Document` |
| DELETE | `/api/collections/:name/records/:id` | rule |  | `{ deleted }` |
| GET | `/api/files` | authed | `?limit=&after=` | `{ files, nextAfter }` |
| POST | `/api/files` | authed | multipart `file` or raw body with `X-Filename` | `FileMeta` |
| GET | `/api/files/:id` | authed |  | raw bytes |
| DELETE | `/api/files/:id` | admin |  | `{ deleted }` |
| GET | `/api/kv/:key` | authed |  | `{ key, value }` |
| PUT | `/api/kv/:key` | admin | `{ value }` | `{ key }` |
| DELETE | `/api/kv/:key` | admin |  | `{ deleted }` |
| GET | `/api/realtime` | authed | `?topic=...` | SSE stream |
| GET | `/api/logs` | admin | `?limit=` | `LogEntry[]` |
| GET | `/api/stats` | admin |  | `Stats` |

"rule" in the Policy column means the rule is sourced from the target collection's per-action rule (default `authed`).

## Status codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 201 | Created (POST that creates a resource) |
| 400 | Bad request (body parse error, validation, unknown field, etc.) |
| 401 | Missing or invalid token |
| 403 | Token valid but role insufficient (admin required) |
| 404 | Resource not found |
| 405 | Method not allowed on that path |
| 500 | Internal error |

## Object shapes

### `User`

```json
{
  "id": "01H...",
  "email": "you@example.com",
  "role": "admin",
  "created": "2026-05-27T12:34:56Z",
  "updated": "2026-05-27T12:34:56Z"
}
```

### `Session`

```json
{
  "token": "eyJzdWI...XYZ.HMACSIG",
  "user": { /* User */ },
  "expires": "2026-06-03T12:34:56Z"
}
```

### `CollectionMeta`

```json
{
  "name": "notes",
  "fields": [
    { "name": "title", "type": "text", "required": true, "unique": false },
    { "name": "pinned", "type": "bool" }
  ],
  "listRule":   "public",
  "viewRule":   "authed",
  "createRule": "admin",
  "updateRule": "admin",
  "deleteRule": "admin",
  "created": "2026-05-27T12:34:56Z",
  "updated": "2026-05-27T12:34:56Z"
}
```

### `Document`

```json
{
  "id": "01H...",
  "created": "2026-05-27T12:34:56Z",
  "updated": "2026-05-27T12:34:56Z",
  "data": { "title": "hello", "pinned": true }
}
```

### `FileMeta`

```json
{
  "id": "01H...",
  "name": "diagram.png",
  "size": 17483,
  "mimeType": "image/png",
  "sha256": "8a3b...",
  "created": "2026-05-27T12:34:56Z"
}
```

### `Stats`

```json
{
  "dataDir": "C:\\Users\\you\\AppData\\Roaming\\SolderDB",
  "walPath": "C:\\...\\wal.bin",
  "walBytes": 1024,
  "keys": 42,
  "liveKeys": 40,
  "tombstones": 2,
  "memtableBytes": 8192,
  "ssTableCount": 3,
  "ssTableSizes": [12345, 23456, 8910],
  "flushThresholdBytes": 1048576
}
```

### `LogEntry`

```json
{
  "timestamp": "2026-05-27T12:34:56.789Z",
  "method": "POST",
  "path": "/api/collections/notes/records",
  "status": 201,
  "durationMs": 4,
  "user": "you@example.com",
  "remote": "127.0.0.1:54321"
}
```

## CORS

The server sets `Access-Control-Allow-Origin: *` and handles `OPTIONS` preflight on every route. Safe because it only binds to `127.0.0.1`, so external networks can't reach it regardless.
