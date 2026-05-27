---
title: Operating SolderDB
order: 8
---

# Operating SolderDB

Practical guide to running the database in production-ish use.

## Backup

The data directory is the database. Back it up by copying it.

```bash
# Stop SolderDB first, OR take a Snapshot from the UI / API to get a consistent copy
cp -r "$APPDATA/SolderDB" "$BACKUP_DIR/SolderDB-$(date +%F)"
```

The **Snapshot** feature is the recommended path — it holds the engine lock while it copies WAL + every SSTable to `<dataDir>/snapshots/<timestamp>/`, giving you a guaranteed-consistent copy *without* stopping the running database.

Trigger one from:

- UI: **Snapshots → Create snapshot**
- API: `POST /api/snapshot` (uses the `Snapshot()` method via the bridge — currently exposed only to the desktop app, not the REST API in v1; coming)
- CLI: `solderdb snapshot` (also pending; v1.x)

Restore by stopping the app and replacing the data directory.

## Compaction

Multiple SSTables accumulate after many flushes. Compact them periodically:

- **Manually:** click **Compact** in the topbar, or hit `POST /api/collections/.../compact`
- **Hardware-aware:** the engine refuses to compact when you're under battery / temp thresholds — set those in **Dashboard → Hardware-Aware Compaction**

There's no automatic compaction schedule in v1 — you decide when.

## Monitoring

- **Dashboard** stats card: live keys, tombstones, WAL bytes, SSTable count
- **Lifecycle** view: animated picture of the LSM state
- **Logs** view: every request, live-tailed
- **GET `/api/stats`** for programmatic access (admin token required)

If you want to alert on something (memtable hot, too many SSTables), poll `/api/stats` from a small script.

## Upgrading

v1.x patches are drop-in: replace the binary, run it. WAL + SSTable formats are versioned (`SDBSST02`, etc.) so older files are accepted as long as the version is supported.

Breaking format changes will bump the major version and ship with a migration tool.

## Resetting

To wipe everything and start fresh:

1. Close the app
2. Delete the data directory
3. Launch again — you'll be back at the register-first-user screen

The `.secret` file is regenerated automatically; existing tokens become invalid.

## Security checklist

- [ ] Don't commit your data directory to git
- [ ] Treat `<dataDir>/.secret` like a credential — its disclosure lets attackers forge tokens
- [ ] If you expose the API outside `127.0.0.1`, put it behind TLS — v1 doesn't ship with built-in TLS
- [ ] Rotate the secret after a leak: stop the app, delete `.secret`, restart. Every existing token becomes invalid; users re-login.
- [ ] Use a per-machine admin password manager — there's no password-reset flow in v1

## Performance notes

- Writes are bounded by your disk's `fsync` latency. SSDs: ~0.1–0.5 ms. HDDs: a few ms. The WAL is the bottleneck.
- Reads from the memtable are sub-microsecond. Reads from SSTables are bound by random-read latency on the index file; bloom filters short-circuit ~99% of misses.
- A typical desktop run easily sustains tens of thousands of writes per second.
- Memtable flush blocks writes for the duration of the flush. With the default 1 MB threshold this is usually <100 ms.

## Limits

| Thing | v1 limit |
|-------|----------|
| Key length | 16 MB |
| Value length | 64 MB |
| File upload | 100 MB |
| Memtable flush threshold | 1 MB (configurable in code) |
| Concurrent SSE subscribers | unbounded (each is one TCP conn + 64-event buffer) |
| Session token lifetime | 7 days |

## Logs & diagnostics

- App stdout shows Wails-level logs
- API requests are recorded in the in-memory ring buffer (500 entries; visible in the Logs view)
- For deeper engine logs, look at `<dataDir>` directly:
  - `wal.bin` — current WAL
  - `sstables/*.sst` — flushed tables (filename includes nanosecond timestamp)
- The Go engine's `_ = log.Println` calls land on the stdout of `wails dev` or the Windows event log when packaged
