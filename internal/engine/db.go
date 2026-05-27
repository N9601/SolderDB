package engine

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

var crcTable = crc32.MakeTable(crc32.Castagnoli)

// ---------------- Bloom filter ----------------
// Simple bit-array Bloom filter with double-hashing using FNV-1a 64.
// Sized for ~1% false-positive rate: m = ceil(10 * n), k = 7.

type bloomFilter struct {
	m    uint32 // number of bits (rounded up to byte boundary)
	k    uint32 // number of hash functions
	bits []byte
}

func newBloomForN(n int) *bloomFilter {
	if n < 1 {
		n = 1
	}
	bits := uint32(n * 10)
	if bits < 64 {
		bits = 64
	}
	if bits%8 != 0 {
		bits += 8 - (bits % 8)
	}
	return &bloomFilter{m: bits, k: 7, bits: make([]byte, bits/8)}
}

func bloomHashes(key string) (uint64, uint64) {
	const (
		fnvOffset uint64 = 14695981039346656037
		fnvPrime  uint64 = 1099511628211
	)
	h1 := fnvOffset
	for i := 0; i < len(key); i++ {
		h1 ^= uint64(key[i])
		h1 *= fnvPrime
	}
	// Second hash from a different seed (perturbed offset).
	h2 := fnvOffset ^ 0x9e3779b97f4a7c15
	for i := 0; i < len(key); i++ {
		h2 ^= uint64(key[i])
		h2 *= fnvPrime
	}
	return h1, h2
}

func (b *bloomFilter) add(key string) {
	if b == nil || b.m == 0 {
		return
	}
	h1, h2 := bloomHashes(key)
	for i := uint32(0); i < b.k; i++ {
		pos := uint32((h1 + uint64(i)*h2) % uint64(b.m))
		b.bits[pos/8] |= 1 << (pos % 8)
	}
}

func (b *bloomFilter) mayContain(key string) bool {
	if b == nil || b.m == 0 {
		return true // no filter = assume possible
	}
	h1, h2 := bloomHashes(key)
	for i := uint32(0); i < b.k; i++ {
		pos := uint32((h1 + uint64(i)*h2) % uint64(b.m))
		if b.bits[pos/8]&(1<<(pos%8)) == 0 {
			return false
		}
	}
	return true
}

type opCode byte

const (
	opSet opCode = 1
	opDel opCode = 2
)

type memEntry struct {
	value   string
	deleted bool
}

type DB struct {
	mu       sync.RWMutex
	memtable map[string]memEntry
	memBytes int64

	dataDir string
	walPath string
	walFile *os.File
	walW    *bufio.Writer

	sstDir   string
	sstables []*sstable

	flushThresholdBytes int64
}

type Stats struct {
	DataDir        string
	WALPath        string
	WALBytes       int64
	Keys           int
	Tombstones     int
	LiveKeys       int
	MemtableBytes  int64
	SSTableCount   int
}

type Options struct {
	DataDir string
	FlushThresholdBytes int64
}

type ListKeysOptions struct {
	Prefix string
	Limit  int
}

type ScanOptions struct {
	Prefix string
	After  string // exclusive cursor; return keys strictly greater than After
	Limit  int    // must be >0
}

type ScanResult struct {
	Keys       []string
	NextAfter  string // empty when no more results
}

// ListKeys returns a sorted list of live keys visible to reads (memtable first, then SSTables newest->oldest).
// Tombstoned keys are excluded. Limit<=0 means "no limit".
func (db *DB) ListKeys(opts ListKeysOptions) ([]string, error) {
	prefix := opts.Prefix
	limit := opts.Limit

	db.mu.RLock()
	memSnapshot := make(map[string]memEntry, len(db.memtable))
	for k, v := range db.memtable {
		memSnapshot[k] = v
	}
	tables := append([]*sstable(nil), db.sstables...)
	db.mu.RUnlock()

	visible := make(map[string]struct{}, len(memSnapshot))
	deleted := make(map[string]struct{})

	for k, v := range memSnapshot {
		if prefix != "" && (len(k) < len(prefix) || k[:len(prefix)] != prefix) {
			continue
		}
		if v.deleted {
			deleted[k] = struct{}{}
			continue
		}
		visible[k] = struct{}{}
	}

	// Newest->oldest so newer tables shadow older ones.
	for i := len(tables) - 1; i >= 0; i-- {
		keys, tombs, err := tables[i].listKeys(prefix)
		if err != nil {
			return nil, err
		}
		for _, k := range tombs {
			if _, alreadyVisible := visible[k]; alreadyVisible {
				continue
			}
			deleted[k] = struct{}{}
		}
		for _, k := range keys {
			if _, isDeleted := deleted[k]; isDeleted {
				continue
			}
			if _, alreadyVisible := visible[k]; alreadyVisible {
				continue
			}
			visible[k] = struct{}{}
		}
	}

	out := make([]string, 0, len(visible))
	for k := range visible {
		out = append(out, k)
	}
	sort.Strings(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Scan returns keys in sorted order using a cursor. This is intended for UI pagination.
func (db *DB) Scan(opts ScanOptions) (ScanResult, error) {
	if opts.Limit <= 0 {
		return ScanResult{}, errors.New("scan limit must be > 0")
	}
	all, err := db.ListKeys(ListKeysOptions{Prefix: opts.Prefix, Limit: 0})
	if err != nil {
		return ScanResult{}, err
	}

	start := 0
	if opts.After != "" {
		start = sort.SearchStrings(all, opts.After)
		for start < len(all) && all[start] <= opts.After {
			start++
		}
	}

	if start >= len(all) {
		return ScanResult{Keys: []string{}, NextAfter: ""}, nil
	}

	end := start + opts.Limit
	if end > len(all) {
		end = len(all)
	}
	keys := append([]string(nil), all[start:end]...)
	nextAfter := ""
	if end < len(all) {
		nextAfter = keys[len(keys)-1]
	}
	return ScanResult{Keys: keys, NextAfter: nextAfter}, nil
}

func Open(opts Options) (*DB, error) {
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = defaultDataDir()
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	walPath := filepath.Join(dataDir, "wal.bin")
	sstDir := filepath.Join(dataDir, "sstables")
	if err := os.MkdirAll(sstDir, 0o755); err != nil {
		return nil, fmt.Errorf("create sstable dir: %w", err)
	}

	// Note: do not use O_APPEND on Windows if you need to Truncate() during WAL reset.
	// We maintain the file offset ourselves under db.mu.
	f, err := os.OpenFile(walPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open wal: %w", err)
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("seek wal end: %w", err)
	}

	db := &DB{
		memtable: make(map[string]memEntry),
		dataDir:   dataDir,
		walPath:   walPath,
		walFile:   f,
		walW:      bufio.NewWriterSize(f, 64*1024),
		sstDir:    sstDir,

		flushThresholdBytes: 1 * 1024 * 1024, // 1MB
	}
	if opts.FlushThresholdBytes > 0 {
		db.flushThresholdBytes = opts.FlushThresholdBytes
	}

	if err := db.loadSSTables(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := db.replayWAL(); err != nil {
		_ = db.Close()
		return nil, err
	}
	db.recomputeMemBytesLocked()

	return db, nil
}

func defaultDataDir() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "SolderDB")
	}
	return filepath.Join(".", "data")
}

func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var firstErr error
	if db.walW != nil {
		if err := db.walW.Flush(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if db.walFile != nil {
		if err := db.walFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	db.walW = nil
	db.walFile = nil

	for _, t := range db.sstables {
		_ = t.close()
	}
	db.sstables = nil
	return firstErr
}

// DataDir returns the configured data directory.
func (db *DB) DataDir() string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.dataDir
}

func (db *DB) Stats() (Stats, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var s Stats
	s.DataDir = db.dataDir
	s.WALPath = db.walPath

	var live int
	var tomb int
	var approxBytes int64
	for k, v := range db.memtable {
		approxBytes += int64(len(k))
		if v.deleted {
			tomb++
			continue
		}
		live++
		approxBytes += int64(len(v.value))
	}
	s.Keys = len(db.memtable)
	s.Tombstones = tomb
	s.LiveKeys = live
	s.MemtableBytes = approxBytes
	s.SSTableCount = len(db.sstables)

	if db.walFile != nil {
		fi, err := db.walFile.Stat()
		if err != nil {
			return Stats{}, fmt.Errorf("stat wal: %w", err)
		}
		s.WALBytes = fi.Size()
	}

	return s, nil
}

func (db *DB) Get(key string) (string, bool) {
	db.mu.RLock()
	entry, ok := db.memtable[key]
	if !ok || entry.deleted {
		// Fall through to SSTables (need to release RLock first to avoid holding it during I/O).
	} else {
		val := entry.value
		db.mu.RUnlock()
		return val, true
	}
	db.mu.RUnlock()

	return db.getFromSSTables(key)
}

func (db *DB) Set(key, value string) error {
	if key == "" {
		return errors.New("key cannot be empty")
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	if err := db.appendWAL(opSet, key, value); err != nil {
		return err
	}
	prev, hadPrev := db.memtable[key]
	db.memtable[key] = memEntry{value: value, deleted: false}

	db.memBytes += int64(len(key)) + int64(len(value))
	if hadPrev {
		db.memBytes -= int64(len(key))
		if !prev.deleted {
			db.memBytes -= int64(len(prev.value))
		}
	}

	if db.memBytes >= db.flushThresholdBytes {
		if err := db.flushMemtableToSSTableLocked(); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) Delete(key string) error {
	if key == "" {
		return errors.New("key cannot be empty")
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	if err := db.appendWAL(opDel, key, ""); err != nil {
		return err
	}
	prev, hadPrev := db.memtable[key]
	db.memtable[key] = memEntry{deleted: true}

	db.memBytes += int64(len(key))
	if hadPrev {
		db.memBytes -= int64(len(key))
		if !prev.deleted {
			db.memBytes -= int64(len(prev.value))
		}
	}

	if db.memBytes >= db.flushThresholdBytes {
		if err := db.flushMemtableToSSTableLocked(); err != nil {
			return err
		}
	}
	return nil
}

// Snapshot copies the WAL and all SSTables to a new timestamped folder under
// <dataDir>/snapshots/. Returns the absolute destination path.
//
// The lock is held for the duration of the copy so the snapshot is internally
// consistent: no flush or compaction can run while files are being copied.
func (db *DB) Snapshot() (string, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.walW != nil {
		if err := db.walW.Flush(); err != nil {
			return "", fmt.Errorf("snapshot flush wal: %w", err)
		}
	}
	if db.walFile != nil {
		if err := db.walFile.Sync(); err != nil {
			return "", fmt.Errorf("snapshot sync wal: %w", err)
		}
	}

	ts := time.Now().UTC().Format("20060102-150405.000000000")
	dst := filepath.Join(db.dataDir, "snapshots", ts)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", fmt.Errorf("snapshot mkdir: %w", err)
	}

	if db.walPath != "" {
		if err := copyFile(db.walPath, filepath.Join(dst, "wal.bin")); err != nil {
			return "", fmt.Errorf("snapshot wal: %w", err)
		}
	}

	sstDst := filepath.Join(dst, "sstables")
	if err := os.MkdirAll(sstDst, 0o755); err != nil {
		return "", fmt.Errorf("snapshot sstables dir: %w", err)
	}
	for _, t := range db.sstables {
		name := filepath.Base(t.path)
		if err := copyFile(t.path, filepath.Join(sstDst, name)); err != nil {
			return "", fmt.Errorf("snapshot sst %s: %w", name, err)
		}
	}
	return dst, nil
}

type SnapshotInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	CreatedAt string `json:"createdAt"`
}

// ListSnapshots returns previously created snapshot directories, newest first.
// Returns an empty slice if no snapshots exist yet (no error).
func (db *DB) ListSnapshots() ([]SnapshotInfo, error) {
	dir := filepath.Join(db.dataDir, "snapshots")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SnapshotInfo{}, nil
		}
		return nil, fmt.Errorf("read snapshots dir: %w", err)
	}
	out := make([]SnapshotInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		full := filepath.Join(dir, e.Name())
		size := dirSize(full)
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, SnapshotInfo{
			Name:      e.Name(),
			Path:      full,
			Bytes:     size,
			CreatedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// Compact merges all current SSTables into a single new SSTable (newest-wins).
// This reduces the number of files and improves read performance.
func (db *DB) Compact() error {
	db.mu.RLock()
	tables := append([]*sstable(nil), db.sstables...)
	db.mu.RUnlock()

	if len(tables) <= 1 {
		return nil
	}

	merged := make(map[string]sstRecord, 1024)

	// Oldest -> newest so newer records overwrite.
	for i := 0; i < len(tables); i++ {
		recs, err := tables[i].readAll()
		if err != nil {
			return err
		}
		for _, r := range recs {
			merged[r.key] = r
		}
	}

	out := make([]sstRecord, 0, len(merged))
	for _, r := range merged {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })

	ts := time.Now().UTC().UnixNano()
	filename := "sst_compact_" + strconv.FormatInt(ts, 10) + ".sst"
	newPath := filepath.Join(db.sstDir, filename)
	if err := writeSSTable(newPath, out); err != nil {
		return err
	}

	newTable, err := openSSTable(newPath)
	if err != nil {
		return err
	}

	db.mu.Lock()
	oldTables := append([]*sstable(nil), db.sstables...)
	db.sstables = []*sstable{newTable}
	db.mu.Unlock()

	// Best-effort delete old SSTables. If deletion fails (e.g., antivirus lock),
	// the new table is still active and old files can be cleaned later.
	for _, t := range oldTables {
		if t.path == newPath {
			continue
		}
		_ = t.close()
		_ = os.Remove(t.path)
	}

	return nil
}

// WAL record format (binary, little-endian):
// [1 byte opcode][4 bytes keyLen][4 bytes valLen][4 bytes crc32c][key bytes][value bytes]
// CRC32C covers [opcode][keyLen][valLen][key][value].
func (db *DB) appendWAL(op opCode, key, value string) error {
	if db.walW == nil {
		return errors.New("wal not initialized")
	}

	keyB := []byte(key)
	valB := []byte(value)
	if op == opDel {
		valB = nil
	}

	var header [1 + 4 + 4]byte
	header[0] = byte(op)
	binary.LittleEndian.PutUint32(header[1:5], uint32(len(keyB)))
	binary.LittleEndian.PutUint32(header[5:9], uint32(len(valB)))

	h := crc32.New(crcTable)
	_, _ = h.Write(header[:])
	_, _ = h.Write(keyB)
	if len(valB) > 0 {
		_, _ = h.Write(valB)
	}
	var crcBuf [4]byte
	binary.LittleEndian.PutUint32(crcBuf[:], h.Sum32())

	if _, err := db.walW.Write(header[:]); err != nil {
		return fmt.Errorf("wal write header: %w", err)
	}
	if _, err := db.walW.Write(crcBuf[:]); err != nil {
		return fmt.Errorf("wal write crc: %w", err)
	}
	if _, err := db.walW.Write(keyB); err != nil {
		return fmt.Errorf("wal write key: %w", err)
	}
	if len(valB) > 0 {
		if _, err := db.walW.Write(valB); err != nil {
			return fmt.Errorf("wal write value: %w", err)
		}
	}
	if err := db.walW.Flush(); err != nil {
		return fmt.Errorf("wal flush: %w", err)
	}
	if err := db.walFile.Sync(); err != nil {
		return fmt.Errorf("wal fsync: %w", err)
	}
	return nil
}

func (db *DB) replayWAL() error {
	// Read WAL from the beginning; apply operations to memtable.
	if _, err := db.walFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("wal seek: %w", err)
	}

	r := bufio.NewReaderSize(db.walFile, 64*1024)

	for {
		opByte, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("wal read opcode: %w", err)
		}

		var lens [8]byte
		if _, err := io.ReadFull(r, lens[:]); err != nil {
			// Treat partial tail as corruption and stop (common after crash).
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return fmt.Errorf("wal read lengths: %w", err)
		}

		keyLen := binary.LittleEndian.Uint32(lens[0:4])
		valLen := binary.LittleEndian.Uint32(lens[4:8])

		if keyLen == 0 {
			return errors.New("wal record with empty key")
		}
		if keyLen > 16*1024*1024 || valLen > 64*1024*1024 {
			return errors.New("wal record exceeds size limits")
		}

		var crcBuf [4]byte
		if _, err := io.ReadFull(r, crcBuf[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return fmt.Errorf("wal read crc: %w", err)
		}
		want := binary.LittleEndian.Uint32(crcBuf[:])

		key := make([]byte, keyLen)
		if _, err := io.ReadFull(r, key); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return fmt.Errorf("wal read key: %w", err)
		}

		value := make([]byte, valLen)
		if valLen > 0 {
			if _, err := io.ReadFull(r, value); err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					break
				}
				return fmt.Errorf("wal read value: %w", err)
			}
		}

		// Verify CRC. A mismatch on the tail record means torn write — stop here.
		h := crc32.New(crcTable)
		var hdr [1 + 4 + 4]byte
		hdr[0] = opByte
		binary.LittleEndian.PutUint32(hdr[1:5], keyLen)
		binary.LittleEndian.PutUint32(hdr[5:9], valLen)
		_, _ = h.Write(hdr[:])
		_, _ = h.Write(key)
		if valLen > 0 {
			_, _ = h.Write(value)
		}
		if h.Sum32() != want {
			// Torn or corrupt record — stop replay, leave earlier records applied.
			break
		}

		switch opCode(opByte) {
		case opSet:
			db.memtable[string(key)] = memEntry{value: string(value), deleted: false}
		case opDel:
			db.memtable[string(key)] = memEntry{deleted: true}
		default:
			return fmt.Errorf("wal unknown opcode: %d", opByte)
		}
	}

	// Return to append mode.
	if _, err := db.walFile.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("wal seek end: %w", err)
	}
	db.walW.Reset(db.walFile)
	return nil
}

func (db *DB) recomputeMemBytesLocked() {
	var b int64
	for k, v := range db.memtable {
		b += int64(len(k))
		if !v.deleted {
			b += int64(len(v.value))
		}
	}
	db.memBytes = b
}

// ---------------- SSTables ----------------

type sstable struct {
	path string
	f    *os.File
	// Sorted keys and offsets into the data section.
	keys    []string
	offsets []uint64
	bloom   *bloomFilter
}

func (t *sstable) close() error {
	if t.f == nil {
		return nil
	}
	err := t.f.Close()
	t.f = nil
	return err
}

type sstRecord struct {
	key     string
	value   string
	deleted bool
}

const (
	sstMagic       = "SDBSST02"
	sstFooterMagic = "SDBEND02"
	sstVersion     = uint32(2)
	sstFooterSize  = 8 + 8 + 8 // indexOffset + bloomOffset + footerMagic
)

func (db *DB) loadSSTables() error {
	entries, err := os.ReadDir(db.sstDir)
	if err != nil {
		return fmt.Errorf("read sstable dir: %w", err)
	}

	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".sst" {
			continue
		}
		paths = append(paths, filepath.Join(db.sstDir, e.Name()))
	}
	sort.Strings(paths)

	db.sstables = nil
	for _, p := range paths {
		t, err := openSSTable(p)
		if err != nil {
			return err
		}
		db.sstables = append(db.sstables, t)
	}
	return nil
}

func openSSTable(path string) (*sstable, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open sstable: %w", err)
	}

	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat sstable: %w", err)
	}
	if fi.Size() < int64(len(sstMagic)+4+4+sstFooterSize) {
		_ = f.Close()
		return nil, fmt.Errorf("sstable too small: %s", path)
	}

	// Read header: magic(8) + version(u32) + count(u32)
	header := make([]byte, 8+4+4)
	if _, err := io.ReadFull(f, header); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("read sstable header: %w", err)
	}
	if string(header[:8]) != sstMagic {
		_ = f.Close()
		return nil, fmt.Errorf("sstable bad magic: %s", path)
	}
	if binary.LittleEndian.Uint32(header[8:12]) != sstVersion {
		_ = f.Close()
		return nil, fmt.Errorf("sstable unsupported version: %s", path)
	}
	count := binary.LittleEndian.Uint32(header[12:16])

	// Read footer: [indexOffset u64][bloomOffset u64][footerMagic 8]
	if _, err := f.Seek(fi.Size()-int64(sstFooterSize), io.SeekStart); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("seek footer: %w", err)
	}
	var footer [sstFooterSize]byte
	if _, err := io.ReadFull(f, footer[:]); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("read footer: %w", err)
	}
	indexOffset := binary.LittleEndian.Uint64(footer[0:8])
	bloomOffset := binary.LittleEndian.Uint64(footer[8:16])
	if string(footer[16:24]) != sstFooterMagic {
		_ = f.Close()
		return nil, fmt.Errorf("sstable bad footer magic: %s", path)
	}

	// Read bloom: [m u32][k u32][bits...]
	var bloom *bloomFilter
	if bloomOffset > 0 {
		if _, err := f.Seek(int64(bloomOffset), io.SeekStart); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("seek bloom: %w", err)
		}
		var bhdr [8]byte
		if _, err := io.ReadFull(f, bhdr[:]); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("read bloom header: %w", err)
		}
		m := binary.LittleEndian.Uint32(bhdr[0:4])
		k := binary.LittleEndian.Uint32(bhdr[4:8])
		if m == 0 || m > 1<<30 || m%8 != 0 {
			_ = f.Close()
			return nil, fmt.Errorf("sstable invalid bloom params: %s", path)
		}
		bits := make([]byte, m/8)
		if _, err := io.ReadFull(f, bits); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("read bloom bits: %w", err)
		}
		bloom = &bloomFilter{m: m, k: k, bits: bits}
	}

	// Read index
	if _, err := f.Seek(int64(indexOffset), io.SeekStart); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("seek index: %w", err)
	}
	r := bufio.NewReaderSize(f, 64*1024)

	keys := make([]string, 0, count)
	offsets := make([]uint64, 0, count)
	for i := uint32(0); i < count; i++ {
		var klBuf [4]byte
		if _, err := io.ReadFull(r, klBuf[:]); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("read index keyLen: %w", err)
		}
		kl := binary.LittleEndian.Uint32(klBuf[:])
		if kl == 0 || kl > 16*1024*1024 {
			_ = f.Close()
			return nil, fmt.Errorf("sstable invalid index keyLen: %s", path)
		}
		kb := make([]byte, kl)
		if _, err := io.ReadFull(r, kb); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("read index key: %w", err)
		}
		var offBuf [8]byte
		if _, err := io.ReadFull(r, offBuf[:]); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("read index offset: %w", err)
		}
		keys = append(keys, string(kb))
		offsets = append(offsets, binary.LittleEndian.Uint64(offBuf[:]))
	}

	return &sstable{path: path, f: f, keys: keys, offsets: offsets, bloom: bloom}, nil
}

func (t *sstable) readAll() ([]sstRecord, error) {
	fi, err := t.f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat sstable: %w", err)
	}

	// Read footer to learn data section end (= bloomOffset if present, else indexOffset).
	if _, err := t.f.Seek(fi.Size()-int64(sstFooterSize), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek footer: %w", err)
	}
	var footer [sstFooterSize]byte
	if _, err := io.ReadFull(t.f, footer[:]); err != nil {
		return nil, fmt.Errorf("read footer: %w", err)
	}
	indexOffset := binary.LittleEndian.Uint64(footer[0:8])
	bloomOffset := binary.LittleEndian.Uint64(footer[8:16])
	if string(footer[16:24]) != sstFooterMagic {
		return nil, fmt.Errorf("sstable bad footer magic: %s", t.path)
	}
	dataEnd := indexOffset
	if bloomOffset > 0 && bloomOffset < dataEnd {
		dataEnd = bloomOffset
	}

	// Seek to start of data section (after header).
	dataStart := int64(len(sstMagic) + 4 + 4)
	if _, err := t.f.Seek(dataStart, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek data: %w", err)
	}

	r := bufio.NewReaderSize(t.f, 64*1024)
	var out []sstRecord
	bytesRead := uint64(dataStart)

	for bytesRead < dataEnd {
		tomb, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("read record tomb: %w", err)
		}
		bytesRead += 1

		var lens [8]byte
		if _, err := io.ReadFull(r, lens[:]); err != nil {
			return nil, fmt.Errorf("read record lens: %w", err)
		}
		bytesRead += 8

		keyLen := binary.LittleEndian.Uint32(lens[0:4])
		valLen := binary.LittleEndian.Uint32(lens[4:8])
		if keyLen == 0 || keyLen > 16*1024*1024 || valLen > 64*1024*1024 {
			return nil, fmt.Errorf("sstable invalid record sizes: %s", t.path)
		}

		kb := make([]byte, keyLen)
		if _, err := io.ReadFull(r, kb); err != nil {
			return nil, fmt.Errorf("read record key: %w", err)
		}
		bytesRead += uint64(keyLen)

		vb := make([]byte, valLen)
		if valLen > 0 {
			if _, err := io.ReadFull(r, vb); err != nil {
				return nil, fmt.Errorf("read record value: %w", err)
			}
			bytesRead += uint64(valLen)
		}

		out = append(out, sstRecord{
			key:     string(kb),
			value:   string(vb),
			deleted: tomb == 1,
		})
	}

	return out, nil
}

func (db *DB) getFromSSTables(key string) (string, bool) {
	db.mu.RLock()
	tables := append([]*sstable(nil), db.sstables...)
	db.mu.RUnlock()

	// Newest tables last (sorted by name); search from newest to oldest.
	for i := len(tables) - 1; i >= 0; i-- {
		val, ok, deleted, err := tables[i].get(key)
		if err != nil {
			continue
		}
		if ok {
			if deleted {
				return "", false
			}
			return val, true
		}
	}
	return "", false
}

func (t *sstable) get(key string) (value string, ok bool, deleted bool, err error) {
	if !t.bloom.mayContain(key) {
		return "", false, false, nil
	}
	i := sort.SearchStrings(t.keys, key)
	if i >= len(t.keys) || t.keys[i] != key {
		return "", false, false, nil
	}
	offset := t.offsets[i]
	return t.readAt(offset, key)
}

func (t *sstable) listKeys(prefix string) (keys []string, tombstones []string, err error) {
	// For now, read by random access per key to learn tombstones.
	// This is OK for browsing; we avoid reading full values by checking tombstone bytes at offsets.
	start := 0
	if prefix != "" {
		start = sort.Search(len(t.keys), func(i int) bool { return t.keys[i] >= prefix })
	}
	for i := start; i < len(t.keys); i++ {
		k := t.keys[i]
		if prefix != "" && (len(k) < len(prefix) || k[:len(prefix)] != prefix) {
			break
		}
		deleted, ok, err := t.isTombstoneAt(t.offsets[i])
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		if deleted {
			tombstones = append(tombstones, k)
			continue
		}
		keys = append(keys, k)
	}
	return keys, tombstones, nil
}

func (t *sstable) isTombstoneAt(offset uint64) (deleted bool, ok bool, err error) {
	var b [1]byte
	n, err := t.f.ReadAt(b[:], int64(offset))
	if err != nil {
		if errors.Is(err, io.EOF) {
			return false, false, nil
		}
		return false, false, err
	}
	if n != 1 {
		return false, false, nil
	}
	return b[0] == 1, true, nil
}

func (t *sstable) readAt(offset uint64, key string) (value string, ok bool, deleted bool, err error) {
	// Record header is 1(tomb)+4(keyLen)+4(valLen).
	var hdr [9]byte
	if _, err := t.f.ReadAt(hdr[:], int64(offset)); err != nil {
		if errors.Is(err, io.EOF) {
			return "", false, false, nil
		}
		return "", false, false, err
	}
	tomb := hdr[0]
	keyLen := binary.LittleEndian.Uint32(hdr[1:5])
	valLen := binary.LittleEndian.Uint32(hdr[5:9])
	if keyLen == 0 || keyLen > 16*1024*1024 || valLen > 64*1024*1024 {
		return "", false, false, errors.New("sstable record size invalid")
	}

	keyOff := int64(offset) + 9
	kb := make([]byte, keyLen)
	if _, err := t.f.ReadAt(kb, keyOff); err != nil {
		return "", false, false, err
	}
	if string(kb) != key {
		return "", false, false, errors.New("sstable index mismatch")
	}

	if tomb == 1 {
		return "", true, true, nil
	}

	vb := make([]byte, valLen)
	if valLen > 0 {
		valOff := keyOff + int64(keyLen)
		if _, err := t.f.ReadAt(vb, valOff); err != nil {
			return "", false, false, err
		}
	}
	return string(vb), true, false, nil
}

// flushMemtableToSSTableLocked flushes the current memtable into a new immutable SSTable.
// Caller must hold db.mu.Lock().
func (db *DB) flushMemtableToSSTableLocked() error {
	if len(db.memtable) == 0 {
		db.memBytes = 0
		return nil
	}

	records := make([]sstRecord, 0, len(db.memtable))
	for k, v := range db.memtable {
		records = append(records, sstRecord{key: k, value: v.value, deleted: v.deleted})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].key < records[j].key })

	ts := time.Now().UTC().UnixNano()
	filename := "sst_" + strconv.FormatInt(ts, 10) + ".sst"
	path := filepath.Join(db.sstDir, filename)

	if err := writeSSTable(path, records); err != nil {
		return err
	}

	t, err := openSSTable(path)
	if err != nil {
		return err
	}
	db.sstables = append(db.sstables, t)

	// Reset memtable and WAL after successful flush.
	db.memtable = make(map[string]memEntry)
	db.memBytes = 0

	if err := db.resetWALLocked(); err != nil {
		return err
	}
	return nil
}

func writeSSTable(path string, records []sstRecord) error {
	// Create exclusively to avoid clobbering.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create sstable: %w", err)
	}
	defer func() { _ = f.Close() }()

	w := bufio.NewWriterSize(f, 64*1024)

	// Header
	if _, err := w.Write([]byte(sstMagic)); err != nil {
		return fmt.Errorf("sstable header magic: %w", err)
	}
	var hdr [8]byte
	binary.LittleEndian.PutUint32(hdr[0:4], sstVersion)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(records)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("sstable header: %w", err)
	}

	type idx struct {
		key    string
		offset uint64
	}
	index := make([]idx, 0, len(records))

	// Build bloom filter from live (non-tombstoned) keys + all keys; including
	// tombstoned ones is fine — mayContain just permits seeks for them too.
	bloom := newBloomForN(len(records))

	// Data section offsets are from file start.
	offset := uint64(len(sstMagic) + 4 + 4)
	for _, r := range records {
		index = append(index, idx{key: r.key, offset: offset})
		bloom.add(r.key)

		tomb := byte(0)
		valB := []byte(r.value)
		if r.deleted {
			tomb = 1
			valB = nil
		}
		keyB := []byte(r.key)

		var recHdr [1 + 4 + 4]byte
		recHdr[0] = tomb
		binary.LittleEndian.PutUint32(recHdr[1:5], uint32(len(keyB)))
		binary.LittleEndian.PutUint32(recHdr[5:9], uint32(len(valB)))

		if _, err := w.Write(recHdr[:]); err != nil {
			return fmt.Errorf("sstable record header: %w", err)
		}
		if _, err := w.Write(keyB); err != nil {
			return fmt.Errorf("sstable record key: %w", err)
		}
		if len(valB) > 0 {
			if _, err := w.Write(valB); err != nil {
				return fmt.Errorf("sstable record value: %w", err)
			}
		}

		offset += uint64(len(recHdr)) + uint64(len(keyB)) + uint64(len(valB))
	}

	// Bloom section: [m u32][k u32][bits...]
	bloomOffset := offset
	var bHdr [8]byte
	binary.LittleEndian.PutUint32(bHdr[0:4], bloom.m)
	binary.LittleEndian.PutUint32(bHdr[4:8], bloom.k)
	if _, err := w.Write(bHdr[:]); err != nil {
		return fmt.Errorf("sstable bloom header: %w", err)
	}
	if _, err := w.Write(bloom.bits); err != nil {
		return fmt.Errorf("sstable bloom bits: %w", err)
	}
	offset += uint64(len(bHdr)) + uint64(len(bloom.bits))

	indexOffset := offset
	for _, it := range index {
		kb := []byte(it.key)
		var kl [4]byte
		binary.LittleEndian.PutUint32(kl[:], uint32(len(kb)))
		if _, err := w.Write(kl[:]); err != nil {
			return fmt.Errorf("sstable index keyLen: %w", err)
		}
		if _, err := w.Write(kb); err != nil {
			return fmt.Errorf("sstable index key: %w", err)
		}
		var off [8]byte
		binary.LittleEndian.PutUint64(off[:], it.offset)
		if _, err := w.Write(off[:]); err != nil {
			return fmt.Errorf("sstable index offset: %w", err)
		}
		offset += uint64(len(kl)) + uint64(len(kb)) + uint64(len(off))
	}

	// Footer: [indexOffset u64][bloomOffset u64][footerMagic 8]
	var off [8]byte
	binary.LittleEndian.PutUint64(off[:], indexOffset)
	if _, err := w.Write(off[:]); err != nil {
		return fmt.Errorf("sstable footer index offset: %w", err)
	}
	binary.LittleEndian.PutUint64(off[:], bloomOffset)
	if _, err := w.Write(off[:]); err != nil {
		return fmt.Errorf("sstable footer bloom offset: %w", err)
	}
	if _, err := w.Write([]byte(sstFooterMagic)); err != nil {
		return fmt.Errorf("sstable footer magic: %w", err)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("sstable flush: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sstable fsync: %w", err)
	}
	return nil
}

func (db *DB) resetWALLocked() error {
	if db.walW == nil || db.walFile == nil {
		return errors.New("wal not initialized")
	}
	if err := db.walW.Flush(); err != nil {
		return fmt.Errorf("wal flush before reset: %w", err)
	}
	if err := db.walFile.Truncate(0); err != nil {
		return fmt.Errorf("wal truncate: %w", err)
	}
	if _, err := db.walFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("wal seek start: %w", err)
	}
	db.walW.Reset(db.walFile)
	if err := db.walW.Flush(); err != nil {
		return fmt.Errorf("wal flush after reset: %w", err)
	}
	if err := db.walFile.Sync(); err != nil {
		return fmt.Errorf("wal fsync after reset: %w", err)
	}
	if _, err := db.walFile.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("wal seek end after reset: %w", err)
	}
	db.walW.Reset(db.walFile)
	return nil
}
