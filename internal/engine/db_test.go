package engine

import (
	"os"
	"path/filepath"
	"strings"
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

