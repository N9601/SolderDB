package bridge

import (
	"fmt"

	"solderdb/internal/engine"
)

type DBService struct {
	db      *engine.DB
	apiAddr string
}

// SetAPIAddr is called by main once the local REST API has bound a port,
// so the UI can display the address to users without hardcoding it.
func (s *DBService) SetAPIAddr(addr string) {
	s.apiAddr = addr
}

func (s *DBService) GetAPIAddr() string {
	return s.apiAddr
}

type Stats struct {
	DataDir             string  `json:"dataDir"`
	WALPath             string  `json:"walPath"`
	WALBytes            int64   `json:"walBytes"`
	Keys                int     `json:"keys"`
	LiveKeys            int     `json:"liveKeys"`
	Tombstones          int     `json:"tombstones"`
	MemtableBytes       int64   `json:"memtableBytes"`
	SSTableCount        int     `json:"ssTableCount"`
	SSTableSizes        []int64 `json:"ssTableSizes"`
	FlushThresholdBytes int64   `json:"flushThresholdBytes"`
}

type ListKeysOptions struct {
	Prefix string `json:"prefix"`
	Limit  int    `json:"limit"`
}

type ScanOptions struct {
	Prefix string `json:"prefix"`
	After  string `json:"after"`
	Start  string `json:"start"`
	End    string `json:"end"`
	Limit  int    `json:"limit"`
}

type ScanResult struct {
	Keys      []string `json:"keys"`
	NextAfter string   `json:"nextAfter"`
}

func NewDBService(dataDir string) (*DBService, error) {
	db, err := engine.Open(engine.Options{DataDir: dataDir})
	if err != nil {
		return nil, err
	}
	return &DBService{db: db}, nil
}

// Engine returns the underlying *engine.DB. Used by main to construct sibling
// services (collections, etc.) that share the same data store.
func (s *DBService) Engine() *engine.DB {
	return s.db
}

func (s *DBService) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *DBService) Get(key string) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("db not initialized")
	}
	val, ok := s.db.Get(key)
	if !ok {
		return "", nil
	}
	return val, nil
}

func (s *DBService) Set(key string, value string) error {
	if s.db == nil {
		return fmt.Errorf("db not initialized")
	}
	return s.db.Set(key, value)
}

func (s *DBService) Delete(key string) error {
	if s.db == nil {
		return fmt.Errorf("db not initialized")
	}
	return s.db.Delete(key)
}

func (s *DBService) GetStats() (Stats, error) {
	if s.db == nil {
		return Stats{}, fmt.Errorf("db not initialized")
	}
	st, err := s.db.Stats()
	if err != nil {
		return Stats{}, err
	}
	return Stats{
		DataDir:             st.DataDir,
		WALPath:             st.WALPath,
		WALBytes:            st.WALBytes,
		Keys:                st.Keys,
		LiveKeys:            st.LiveKeys,
		Tombstones:          st.Tombstones,
		MemtableBytes:       st.MemtableBytes,
		SSTableCount:        st.SSTableCount,
		SSTableSizes:        st.SSTableSizes,
		FlushThresholdBytes: st.FlushThresholdBytes,
	}, nil
}

func (s *DBService) ListKeys(opts ListKeysOptions) ([]string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	return s.db.ListKeys(engine.ListKeysOptions{
		Prefix: opts.Prefix,
		Limit:  opts.Limit,
	})
}

func (s *DBService) Compact() error {
	if s.db == nil {
		return fmt.Errorf("db not initialized")
	}
	return s.db.Compact()
}

func (s *DBService) Snapshot() (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("db not initialized")
	}
	return s.db.Snapshot()
}

type SnapshotInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	CreatedAt string `json:"createdAt"`
}

func (s *DBService) ListSnapshots() ([]SnapshotInfo, error) {
	if s.db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	list, err := s.db.ListSnapshots()
	if err != nil {
		return nil, err
	}
	out := make([]SnapshotInfo, len(list))
	for i, x := range list {
		out[i] = SnapshotInfo{Name: x.Name, Path: x.Path, Bytes: x.Bytes, CreatedAt: x.CreatedAt}
	}
	return out, nil
}

func (s *DBService) Scan(opts ScanOptions) (ScanResult, error) {
	if s.db == nil {
		return ScanResult{}, fmt.Errorf("db not initialized")
	}
	res, err := s.db.Scan(engine.ScanOptions{
		Prefix: opts.Prefix,
		After:  opts.After,
		Start:  opts.Start,
		End:    opts.End,
		Limit:  opts.Limit,
	})
	if err != nil {
		return ScanResult{}, err
	}
	return ScanResult{Keys: res.Keys, NextAfter: res.NextAfter}, nil
}
