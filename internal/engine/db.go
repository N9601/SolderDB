package engine

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

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

	f, err := os.OpenFile(walPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open wal: %w", err)
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
	return firstErr
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

// WAL record format (binary, little-endian):
// [1 byte opcode][4 bytes keyLen][4 bytes valLen][key bytes][value bytes]
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

	if _, err := db.walW.Write(header[:]); err != nil {
		return fmt.Errorf("wal write header: %w", err)
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
	// Sorted keys and offsets into the data section.
	keys    []string
	offsets []uint64
}

type sstRecord struct {
	key     string
	value   string
	deleted bool
}

const (
	sstMagic      = "SDBSST01"
	sstFooterMagic = "SDBEND01"
	sstVersion    = uint32(1)
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
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat sstable: %w", err)
	}
	if fi.Size() < int64(len(sstMagic)+4+4+8+len(sstFooterMagic)) {
		return nil, fmt.Errorf("sstable too small: %s", path)
	}

	// Read header: magic(8) + version(u32) + count(u32)
	header := make([]byte, 8+4+4)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, fmt.Errorf("read sstable header: %w", err)
	}
	if string(header[:8]) != sstMagic {
		return nil, fmt.Errorf("sstable bad magic: %s", path)
	}
	if binary.LittleEndian.Uint32(header[8:12]) != sstVersion {
		return nil, fmt.Errorf("sstable unsupported version: %s", path)
	}
	count := binary.LittleEndian.Uint32(header[12:16])

	// Read footer: [indexOffset u64][footerMagic 8]
	if _, err := f.Seek(fi.Size()-int64(8+8), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek footer: %w", err)
	}
	var footer [16]byte
	if _, err := io.ReadFull(f, footer[:]); err != nil {
		return nil, fmt.Errorf("read footer: %w", err)
	}
	indexOffset := binary.LittleEndian.Uint64(footer[0:8])
	if string(footer[8:16]) != sstFooterMagic {
		return nil, fmt.Errorf("sstable bad footer magic: %s", path)
	}

	// Read index
	if _, err := f.Seek(int64(indexOffset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek index: %w", err)
	}
	r := bufio.NewReaderSize(f, 64*1024)

	keys := make([]string, 0, count)
	offsets := make([]uint64, 0, count)
	for i := uint32(0); i < count; i++ {
		var klBuf [4]byte
		if _, err := io.ReadFull(r, klBuf[:]); err != nil {
			return nil, fmt.Errorf("read index keyLen: %w", err)
		}
		kl := binary.LittleEndian.Uint32(klBuf[:])
		if kl == 0 || kl > 16*1024*1024 {
			return nil, fmt.Errorf("sstable invalid index keyLen: %s", path)
		}
		kb := make([]byte, kl)
		if _, err := io.ReadFull(r, kb); err != nil {
			return nil, fmt.Errorf("read index key: %w", err)
		}
		var offBuf [8]byte
		if _, err := io.ReadFull(r, offBuf[:]); err != nil {
			return nil, fmt.Errorf("read index offset: %w", err)
		}
		keys = append(keys, string(kb))
		offsets = append(offsets, binary.LittleEndian.Uint64(offBuf[:]))
	}

	return &sstable{path: path, keys: keys, offsets: offsets}, nil
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
	i := sort.SearchStrings(t.keys, key)
	if i >= len(t.keys) || t.keys[i] != key {
		return "", false, false, nil
	}
	offset := t.offsets[i]

	f, err := os.Open(t.path)
	if err != nil {
		return "", false, false, err
	}
	defer f.Close()

	if _, err := f.Seek(int64(offset), io.SeekStart); err != nil {
		return "", false, false, err
	}

	r := bufio.NewReaderSize(f, 64*1024)
	tomb, err := r.ReadByte()
	if err != nil {
		return "", false, false, err
	}
	var lens [8]byte
	if _, err := io.ReadFull(r, lens[:]); err != nil {
		return "", false, false, err
	}
	keyLen := binary.LittleEndian.Uint32(lens[0:4])
	valLen := binary.LittleEndian.Uint32(lens[4:8])
	if keyLen == 0 || keyLen > 16*1024*1024 || valLen > 64*1024*1024 {
		return "", false, false, errors.New("sstable record size invalid")
	}
	kb := make([]byte, keyLen)
	if _, err := io.ReadFull(r, kb); err != nil {
		return "", false, false, err
	}
	if string(kb) != key {
		return "", false, false, errors.New("sstable index mismatch")
	}
	vb := make([]byte, valLen)
	if valLen > 0 {
		if _, err := io.ReadFull(r, vb); err != nil {
			return "", false, false, err
		}
	}
	if tomb == 1 {
		return "", true, true, nil
	}
	return string(vb), true, false, nil
}

func (t *sstable) listKeys(prefix string) (keys []string, tombstones []string, err error) {
	// For now, read by random access per key to learn tombstones.
	// This is OK for small browsing; later we can cache tombstone bitsets or add a key-only index.
	start := 0
	if prefix != "" {
		start = sort.Search(len(t.keys), func(i int) bool { return t.keys[i] >= prefix })
	}
	for i := start; i < len(t.keys); i++ {
		k := t.keys[i]
		if prefix != "" && (len(k) < len(prefix) || k[:len(prefix)] != prefix) {
			break
		}
		_, ok, deleted, err := t.get(k)
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

	// Data section offsets are from file start.
	offset := uint64(len(sstMagic) + 4 + 4)
	for _, r := range records {
		index = append(index, idx{key: r.key, offset: offset})

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

	// Footer: indexOffset + footer magic
	var off [8]byte
	binary.LittleEndian.PutUint64(off[:], indexOffset)
	if _, err := w.Write(off[:]); err != nil {
		return fmt.Errorf("sstable footer offset: %w", err)
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
