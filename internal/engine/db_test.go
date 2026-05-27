package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestWALReplay(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := db.Set("k1", "v1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := db.Set("k2", "v2"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := db.Delete("k2"); err != nil {
		t.Fatalf("del: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	db2, err := Open(Options{DataDir: dir})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = db2.Close() }()

	if v, ok := db2.Get("k1"); !ok || v != "v1" {
		t.Fatalf("expected k1=v1, got %q ok=%v", v, ok)
	}
	if _, ok := db2.Get("k2"); ok {
		t.Fatalf("expected k2 deleted")
	}
}

func TestFlushCreatesSSTableAndResetsWAL(t *testing.T) {
	dir := t.TempDir()
	// Small threshold so we can force a flush.
	db, err := Open(Options{DataDir: dir, FlushThresholdBytes: 64})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	largeValue := strings.Repeat("x", 200)
	if err := db.Set("a", largeValue); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Verify at least one SSTable exists.
	sstDir := filepath.Join(dir, "sstables")
	entries, err := os.ReadDir(sstDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	found := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".sst" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected .sst file in %s", sstDir)
	}

	// WAL should be reset to small size after flush.
	walPath := filepath.Join(dir, "wal.bin")
	fi, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("stat wal: %v", err)
	}
	if fi.Size() != 0 {
		t.Fatalf("expected wal size 0 after flush, got %d", fi.Size())
	}

	// Value should still be retrievable (from SSTable).
	if v, ok := db.Get("a"); !ok || v != largeValue {
		t.Fatalf("expected a preserved after flush, got ok=%v len=%d", ok, len(v))
	}
}

func TestListKeysMergesMemtableAndSSTables(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{DataDir: dir, FlushThresholdBytes: 64})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Force flush by writing a large value.
	if err := db.Set("alpha", strings.Repeat("a", 200)); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Now in a new memtable.
	if err := db.Set("beta", "b"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := db.Delete("alpha"); err != nil {
		t.Fatalf("del: %v", err)
	}

	keys, err := db.ListKeys(ListKeysOptions{})
	if err != nil {
		t.Fatalf("listkeys: %v", err)
	}
	if len(keys) != 1 || keys[0] != "beta" {
		t.Fatalf("expected [beta], got %v", keys)
	}

	prefixKeys, err := db.ListKeys(ListKeysOptions{Prefix: "b"})
	if err != nil {
		t.Fatalf("listkeys prefix: %v", err)
	}
	if len(prefixKeys) != 1 || prefixKeys[0] != "beta" {
		t.Fatalf("expected [beta], got %v", prefixKeys)
	}
}

func TestCompactionReducesSSTableCount(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{DataDir: dir, FlushThresholdBytes: 64})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Create multiple SSTables by forcing flushes.
	for i := 0; i < 3; i++ {
		if err := db.Set("k"+strconv.Itoa(i), strings.Repeat("x", 200)); err != nil {
			t.Fatalf("set: %v", err)
		}
	}

	st1, err := db.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if st1.SSTableCount < 2 {
		t.Fatalf("expected multiple sstables before compaction, got %d", st1.SSTableCount)
	}

	if err := db.Compact(); err != nil {
		t.Fatalf("compact: %v", err)
	}

	st2, err := db.Stats()
	if err != nil {
		t.Fatalf("stats2: %v", err)
	}
	if st2.SSTableCount != 1 {
		t.Fatalf("expected 1 sstable after compaction, got %d", st2.SSTableCount)
	}
}

func TestScanRangeBounds(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	for _, k := range []string{"a", "b", "c", "d", "e", "f"} {
		if err := db.Set(k, k); err != nil {
			t.Fatalf("set: %v", err)
		}
	}

	res, err := db.Scan(ScanOptions{Start: "b", End: "e", Limit: 100})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	got := strings.Join(res.Keys, ",")
	if got != "b,c,d" {
		t.Fatalf("expected b,c,d, got %s", got)
	}

	// End-only.
	res2, _ := db.Scan(ScanOptions{End: "c", Limit: 100})
	if strings.Join(res2.Keys, ",") != "a,b" {
		t.Fatalf("end-only: got %v", res2.Keys)
	}

	// Start-only.
	res3, _ := db.Scan(ScanOptions{Start: "e", Limit: 100})
	if strings.Join(res3.Keys, ",") != "e,f" {
		t.Fatalf("start-only: got %v", res3.Keys)
	}
}

func TestSnapshotCopiesWALAndSSTables(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{DataDir: dir, FlushThresholdBytes: 64})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Write enough to create at least one SSTable and leave some WAL.
	for i := 0; i < 3; i++ {
		if err := db.Set("k"+strconv.Itoa(i), strings.Repeat("v", 200)); err != nil {
			t.Fatalf("set: %v", err)
		}
	}
	if err := db.Set("tail", "small"); err != nil {
		t.Fatalf("set tail: %v", err)
	}

	path, err := db.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	if _, err := os.Stat(filepath.Join(path, "wal.bin")); err != nil {
		t.Fatalf("snapshot wal missing: %v", err)
	}
	ents, err := os.ReadDir(filepath.Join(path, "sstables"))
	if err != nil {
		t.Fatalf("snapshot sstables: %v", err)
	}
	if len(ents) == 0 {
		t.Fatalf("expected snapshot to contain sstables")
	}
}

func TestWALCorruptedTailIsIgnored(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Set("good", "value"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Append garbage to WAL, simulates a torn write at the tail.
	walPath := filepath.Join(dir, "wal.bin")
	f, err := os.OpenFile(walPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	// Write a header that looks valid (opSet, keyLen=3, valLen=3) with a wrong CRC.
	garbage := []byte{
		1,             // opSet
		3, 0, 0, 0,    // keyLen
		3, 0, 0, 0,    // valLen
		0xff, 0xff, 0xff, 0xff, // bad CRC
		'b', 'a', 'd',
		'x', 'y', 'z',
	}
	if _, err := f.Write(garbage); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	_ = f.Close()

	db2, err := Open(Options{DataDir: dir})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = db2.Close() }()

	if v, ok := db2.Get("good"); !ok || v != "value" {
		t.Fatalf("expected good=value after replay, got %q ok=%v", v, ok)
	}
	if _, ok := db2.Get("bad"); ok {
		t.Fatalf("corrupted tail record should have been ignored")
	}
}

type denyGate struct{ reason string }

func (d denyGate) Allow() (bool, string) { return false, d.reason }

func TestCompactionGateBlocks(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{DataDir: dir, FlushThresholdBytes: 64})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Force at least two SSTables so compaction actually has work to do.
	for i := 0; i < 3; i++ {
		if err := db.Set("k"+strconv.Itoa(i), strings.Repeat("x", 200)); err != nil {
			t.Fatalf("set: %v", err)
		}
	}

	db.SetCompactionGate(denyGate{reason: "battery low"})
	err = db.Compact()
	if err == nil {
		t.Fatal("expected throttle error, got nil")
	}
	var throttled *ErrCompactionThrottled
	if !errors.As(err, &throttled) {
		t.Fatalf("expected *ErrCompactionThrottled, got %T: %v", err, err)
	}
	if throttled.Reason != "battery low" {
		t.Fatalf("reason mismatch: %q", throttled.Reason)
	}
}

// Run with `go test -race ./...` once cgo is available to catch any
// concurrency bugs. Without -race this still exercises every lock path
// and surfaces deadlocks or panics under load.
func TestConcurrentReadWriteDelete(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{DataDir: dir, FlushThresholdBytes: 4 * 1024})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	const writers = 8
	const readers = 8
	const opsPerGoroutine = 500

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for w := 0; w < writers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				k := fmt.Sprintf("w%d:k%d", id, i%50)
				if i%5 == 4 {
					_ = db.Delete(k)
					continue
				}
				if err := db.Set(k, strings.Repeat("v", 32)); err != nil {
					t.Errorf("set: %v", err)
					return
				}
			}
		}(w)
	}

	for r := 0; r < readers; r++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				k := fmt.Sprintf("w%d:k%d", id%writers, i%50)
				_, _ = db.Get(k)
				if i%50 == 0 {
					_, _ = db.ListKeys(ListKeysOptions{Limit: 20})
				}
			}
		}(r)
	}

	wg.Wait()

	if _, err := db.Stats(); err != nil {
		t.Fatalf("stats after concurrent ops: %v", err)
	}
}
