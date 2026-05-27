---
title: Files
order: 5
---

# Files

Blob storage on top of the engine. Files are written to disk under `<dataDir>/files/` and indexed in the internal `_files` collection.

## Upload (multipart)

```bash
curl -X POST http://localhost:8787/api/files \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@diagram.png"
```

Returns:

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

## Upload (raw body)

If you don't want to assemble a multipart payload, send the body raw with `X-Filename`:

```bash
curl -X POST http://localhost:8787/api/files \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: image/png" \
  -H "X-Filename: diagram.png" \
  --data-binary "@diagram.png"
```

## Constraints

- Max file size: **100 MB** (configurable in `internal/files/files.go`)
- The server hashes the body with SHA-256 as it streams it to disk. The hash is part of every file's metadata
- Uploads are atomic. Bytes land in `<dataDir>/files/<id>.tmp` first, then `rename()`'d to `<id>` only after successful close. A partial upload leaves no garbage.

## Download

```http
GET /api/files/:id
Authorization: Bearer <token>
```

Or for browser `<img>` and `<a href>`, append `?token=<token>` (no header):

```html
<img src="http://localhost:8787/api/files/01H...?token=eyJzdWI..." />
```

The JS SDK gives you this URL pre-built:

```ts
const url = db.files.url(meta.id);  // includes ?token=
```

Response headers include:

```
Content-Type:        <stored MIME>
Content-Length:      <byte size>
Content-Disposition: inline; filename="<original>"
X-File-SHA256:       <hex digest>
```

## List

```http
GET /api/files?limit=50&after=<id>
```

Returns the same shape as collection listing. Paginated, sorted by creation order.

## Delete

```http
DELETE /api/files/:id
Authorization: Bearer <admin-token>
```

Removes both the metadata record and the blob from disk.

## Linking files to records

Files are independent of collection records. To link them, store the file's `id` as a `text` field on a record:

```ts
type Photo = { caption: string; fileId: string };
const photos = db.collection<Photo>("photos");

const meta = await db.files.upload(myFile);
await photos.create({ caption: "iron 60w", fileId: meta.id });
```

When rendering, look up the file URL by ID:

```ts
const photo = await photos.get(id);
const src = db.files.url(photo.data.fileId);
```

There's no foreign-key enforcement in v1. If you delete a file the record's `fileId` becomes a dead reference. Validate manually or write a periodic cleanup.

## Realtime events for files

Files publish to `coll:_files` topics (because the metadata lives in a collection). Subscribe to react to uploads and deletes:

```ts
const es = new EventSource(`http://localhost:8787/api/realtime?topic=coll:_files&token=${token}`);
es.addEventListener("create", e => console.log("new file", JSON.parse(e.data)));
```
