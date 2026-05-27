---
title: Roadmap
order: 9
---

# Roadmap

What's shipping next, in rough priority order. Nothing here is committed-to with a date.

## v1.1 — quality of life

- **SolderQL** — small SQL-flavored query language (`SELECT / INSERT / UPDATE / DELETE / SHOW / DESCRIBE`), compiled to collection operations. Hand-rolled lexer + parser, no third-party DB libs. Comes with a SQL editor view in the GUI.
- **Automatic compaction** — periodic background gate-checked compaction trigger; current behavior (manual button) stays as a fallback.
- **Snapshot restore UI** — currently you create snapshots from the UI but restore is manual file-copy. v1.1 adds a restore button.
- **API-exposed snapshots** — `POST /api/snapshot` and `GET /api/snapshots` so SDKs / CI can take backups.
- **Field uniqueness** — `unique: true` on a collection field actually enforces uniqueness via a secondary index.
- **Schema migrations** — rename / coerce-type operations that don't just drop the old data.

## v1.2 — sharper auth

- **Owner-based rules** — a fourth rule mode where the rule expression references the user (e.g. `record.userId == @user.id`). Lets you build real multi-tenant apps.
- **API keys** — long-lived non-bearer credentials, per-collection scopes, revocable via UI.
- **Audit log** — auth events (login, password change, role change) recorded in their own collection.

## v1.x — the networking flex

- **Peer-to-peer local sync** — two SolderDB instances on the same machine, same LAN, or over USB tethering automatically reconcile their WALs. Custom protocol over UDP discovery + framed TCP. No SaaS in the middle. *This is the big one.*
- **Replication primitives** — leader/follower mode for read scaling.

## v2 — opinionated extensions

- **TLS + bind to network** — first-class support for running SolderDB as a service on a private network with proper TLS.
- **Range indexes** — secondary indexes on number/date fields for fast range filtering inside collections.
- **Storage encryption at rest** — XChaCha20 over every SSTable + the WAL, key derived from a passphrase entered on launch.
- **Multiplayer / CRDT collections** — automatic conflict resolution for offline-first apps.

## What we won't do (probably)

- **Joins across collections.** Document-store semantics don't fit relational joins cleanly. SolderQL will reject them.
- **Embedded query languages we don't own.** No SQLite under the hood — defeats the "from scratch" point.
- **A hosted version.** SolderDB is local-first. We'd rather make local-first work brilliantly than chase Supabase.

## Open ideas — would love feedback

- **Android/ARM node** that the desktop can attach to over USB tethering — interesting deployment shape, modest engineering once we have P2P sync
- **Built-in scheduler** for periodic queries / cleanup jobs
- **Hooks** (write-side validation / computed fields) in JS or Starlark — adds significant scope; needs careful sandboxing

Pitches → [GitHub issues](https://github.com/N9601/SolderDB/issues).
