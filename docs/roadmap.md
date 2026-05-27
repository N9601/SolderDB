---
title: Roadmap
order: 9
---

# Roadmap

What's shipping next, in rough priority order. Nothing here is committed to a date.

## v1.1: quality of life

- **SolderQL.** A small SQL-flavored query language (`SELECT / INSERT / UPDATE / DELETE / SHOW / DESCRIBE`), compiled to collection operations. Hand-rolled lexer and parser, no third-party DB libraries. Comes with a SQL editor view in the GUI.
- **Automatic compaction.** Periodic background gate-checked compaction trigger. Current behavior (manual button) stays as a fallback.
- **Snapshot restore UI.** Currently you create snapshots from the UI but restore is manual file-copy. v1.1 adds a restore button.
- **API-exposed snapshots.** `POST /api/snapshot` and `GET /api/snapshots` so SDKs and CI can take backups.
- **Field uniqueness.** `unique: true` on a collection field actually enforces uniqueness via a secondary index.
- **Schema migrations.** Rename and coerce-type operations that don't just drop the old data.

## v1.2: sharper auth

- **Owner-based rules.** A fourth rule mode where the rule expression references the user (e.g. `record.userId == @user.id`). Lets you build real multi-tenant apps.
- **API keys.** Long-lived non-bearer credentials, per-collection scopes, revocable via UI.
- **Audit log.** Auth events (login, password change, role change) recorded in their own collection.

## v1.x: the networking flex

- **Peer-to-peer local sync.** Two SolderDB instances on the same machine, same LAN, or over USB tethering automatically reconcile their WALs. Custom protocol over UDP discovery and framed TCP. No SaaS in the middle. *This is the big one.*
- **Replication primitives.** Leader/follower mode for read scaling.

## v2: opinionated extensions

- **TLS and bind to network.** First-class support for running SolderDB as a service on a private network with proper TLS.
- **Range indexes.** Secondary indexes on number and date fields for fast range filtering inside collections.
- **Storage encryption at rest.** XChaCha20 over every SSTable and the WAL, key derived from a passphrase entered on launch.
- **Multiplayer / CRDT collections.** Automatic conflict resolution for offline-first apps.

## What we won't do (probably)

- **Joins across collections.** Document-store semantics don't fit relational joins cleanly. SolderQL will reject them.
- **Embedded query languages we don't own.** No SQLite under the hood. That defeats the "from scratch" point.
- **A hosted version.** SolderDB is local-first. We'd rather make local-first work brilliantly than chase Supabase.

## Open ideas, would love feedback

- **Android/ARM node** that the desktop can attach to over USB tethering. Interesting deployment shape, modest engineering once we have P2P sync.
- **Built-in scheduler** for periodic queries and cleanup jobs.
- **Hooks** (write-side validation, computed fields) in JS or Starlark. Adds significant scope, needs careful sandboxing.

Pitches go to [GitHub issues](https://github.com/N9601/SolderDB/issues).
