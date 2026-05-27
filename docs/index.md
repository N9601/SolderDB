---
title: SolderDB Documentation
order: 0
---

# SolderDB

Welcome to the SolderDB docs.

SolderDB is a **local-first single-binary database**. One executable on your machine gives you:

- An LSM-tree storage engine, built from scratch in Go
- A REST API and Server-Sent Events realtime
- Authentication and role-based access rules
- Typed record collections with schemas
- File / blob storage
- An admin GUI with command palette, lifecycle visualizer, and API explorer
- TypeScript and Go client SDKs, plus a CLI

Nothing is uploaded anywhere. Your data lives at `%APPDATA%\SolderDB` (Windows), `~/.config/SolderDB` (Linux), or `~/Library/Application Support/SolderDB` (macOS).

## Reading order

1. **[Getting started](./getting-started.md)**: install, first user, first collection
2. **[Collections & records](./collections.md)**: typed records, schemas, validation, rules
3. **[Authentication](./auth.md)**: users, sessions, tokens, password changes
4. **[Realtime](./realtime.md)**: Server-Sent Events, subscribe topics, client patterns
5. **[Files](./files.md)**: uploading blobs, downloading, authenticated `<img>` tags
6. **[REST API reference](./rest-api.md)**: every endpoint, every status code
7. **[Engine internals](./engine.md)**: Memtable, WAL, SSTables, bloom filters, compaction, hardware-aware compaction
8. **[Operating SolderDB](./operating.md)**: backups, snapshots, data directory, upgrades
9. **[Roadmap](./roadmap.md)**: what's planned for v1.x and beyond
