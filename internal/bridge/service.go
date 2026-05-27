package bridge

import (
	"fmt"

	"solderdb/internal/engine"
)

type DBService struct {
	db *engine.DB
}

type Stats struct {
	DataDir       string `json:"dataDir"`
	WALPath       string `json:"walPath"`
	WALBytes      int64  `json:"walBytes"`
	Keys          int    `json:"keys"`
	LiveKeys      int    `json:"liveKeys"`
	Tombstones    int    `json:"tombstones"`
	MemtableBytes int64  `json:"memtableBytes"`
	SSTableCount  int    `json:"ssTableCount"`
}

type ListKeysOptions struct {
	Prefix string `json:"prefix"`
	Limit  int    `json:"limit"`
}

func NewDBService(dataDir string) (*DBService, error) {
	db, err := engine.Open(engine.Options{DataDir: dataDir})
	if err != nil {
		return nil, err
	}
	return &DBService{db: db}, nil
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
		DataDir:       st.DataDir,
		WALPath:       st.WALPath,
		WALBytes:      st.WALBytes,
		Keys:          st.Keys,
		LiveKeys:      st.LiveKeys,
		Tombstones:    st.Tombstones,
		MemtableBytes: st.MemtableBytes,
		SSTableCount:  st.SSTableCount,
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
