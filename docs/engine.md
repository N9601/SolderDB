---
title: Engine internals
order: 7
---

# Engine internals

SolderDB's storage engine is a hand-written Log-Structured Merge tree. **No third-party database libraries.** Every byte that touches disk is code in `internal/engine/`.

## The three layers

### Memtable

An in-memory `map[string]memEntry` protected by a `sync.RWMutex`. Every write lands here first.

```go
type memEntry struct {
    value   string
    deleted bool
}
```

Deletes don't remove the key — they store a tombstone (`deleted: true`). Reads check the memtable first; if a key is present with `deleted: false`, return the value; if it's tombstoned, return "not found"; if absent, fall through to disk.

### Write-Ahead Log (WAL)

`<dataDir>/wal.bin` — append-only binary file. Every write is recorded here **before** the memtable is updated, then the file is `fsync`'d. This is what gives us crash recovery.

Record format (little-endian):

```
[1 byte opcode][4 byte keyLen][4 byte valLen][4 byte crc32c][key bytes][value bytes]
```

CRC32C (Castagnoli) covers `opcode + keyLen + valLen + key + value`. On startup the engine replays the WAL into the memtable. If the last record has a bad CRC — the classic "torn write" after a power loss — replay stops cleanly at that point. Earlier records are kept; the corrupt tail is ignored.

### SSTables

When the memtable's approximate byte size reaches the flush threshold (1 MB by default), the entire memtable is serialized to a new immutable file under `<dataDir>/sstables/`.

```
SSTable v2 format

[8 byte magic "SDBSST02"]
[4 byte version = 2]
[4 byte record count]

[ records section ]
  per record:
    [1 byte tombstone flag]
    [4 byte keyLen][4 byte valLen]
    [key bytes][value bytes]

[ bloom filter section ]
  [4 byte m (bit count)]
  [4 byte k (hash count)]
  [m/8 bytes of filter]

[ index section ]
  per key (sorted):
    [4 byte keyLen][key bytes]
    [8 byte byte offset of record in data section]

[ footer ]
  [8 byte indexOffset]
  [8 byte bloomOffset]
  [8 byte magic "SDBEND02"]
```

Records are sorted by key. The bloom filter lets `Get()` skip the index lookup entirely for keys that aren't in this SSTable — false-positive rate is tuned to ~1% with `m ≈ 10n` and `k = 7`. The hash function is double-hashing FNV-1a 64.

## Read path

1. Check memtable. Tombstone → "not found". Hit → return.
2. Walk SSTables newest-to-oldest. For each: check bloom (skip if `mayContain` is false). Binary-search the in-memory index. Read the record at the offset. Tombstone → "not found". Hit → return.
3. Miss in every layer → "not found".

Newest-first ordering means later writes shadow earlier writes — same key in two SSTables resolves to the newer one.

## Compaction

Multiple SSTables hurt reads (more files to check). **Compaction** merges them: oldest-to-newest into a single in-memory map (so newer values overwrite older ones), then serialized as one new SSTable. Old SSTables are then deleted.

Compaction is **not automatic** in v1 — you trigger it via the `Compact SSTables` button in the GUI or `POST /api/collections/.../compact` is admin-only. (Auto-compaction is on the v1.x roadmap.)

## Hardware-aware compaction

Before each compaction, the engine consults a pluggable `CompactionGate`:

```go
type CompactionGate interface {
    Allow() (ok bool, reason string)
}
```

The default gate (`alwaysAllow`) lets everything through. In the wired-up app, the gate is backed by `internal/hardware/`, which:

- On **Windows**, calls `GetSystemPowerStatus` from `kernel32.dll` via `syscall` to read battery status
- On **Linux**, reads `/sys/class/power_supply/BAT*` and `/sys/class/thermal/thermal_zone*/temp`
- On **macOS**, parses `pmset -g batt` output

If you're on battery below the threshold, or the CPU is hotter than your max temp, `Allow()` returns `false` and `Compact()` returns `*ErrCompactionThrottled` with a human-readable reason. The UI surfaces this as a banner: *"Compaction paused — battery 23%"*.

You can adjust thresholds from the **Dashboard → Hardware-Aware Compaction** card.

## Crash safety guarantees

After a clean shutdown via `Close()`:
- WAL is flushed and fsync'd
- All SSTables are already on disk (they're fsync'd as part of `writeSSTable`)
- The data directory is internally consistent

After a hard crash (power loss, OOM kill):
- WAL is replayed on next open. Bad CRC at the tail is treated as a torn write and replay stops there.
- SSTables are immutable, so they can't be corrupted by an in-progress write. Worst case is a `.tmp` file from a failed flush; the engine ignores anything not ending in `.sst`.

## Range scans

`Scan(prefix, after, start, end, limit)` returns sorted keys constrained by:

- `prefix`: keys starting with this string
- `start`: inclusive lower bound (e.g. `"user:001"`)
- `end`: exclusive upper bound (e.g. `"user:999"`)
- `after`: exclusive cursor for pagination — return keys strictly > this value
- `limit`: max number of keys to return

`Scan` returns up to `limit` keys plus a `nextAfter` cursor. Pass `nextAfter` back as `after` for the next page.

## Concurrency model

- The memtable is protected by a single `sync.RWMutex`. Reads use RLock, writes use Lock.
- SSTables are immutable — no locks needed for reads. Loading the SSTable list takes the engine lock for the slice swap.
- Compaction takes a snapshot of the SSTable list under RLock, does its work without holding any locks, then takes the write lock briefly to swap in the result and delete the old files.
- The WAL append takes the engine lock for the duration of `write → flush → fsync` to maintain a single linear order on disk.

Run `go test -race ./internal/engine` to verify (requires `CGO_ENABLED=1` and a C compiler).
