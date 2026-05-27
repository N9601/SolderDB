// Package collections implements a typed-record layer on top of the KV engine.
// Records are stored as JSON values under deterministic keys so the underlying
// engine remains a pure KV store — no engine changes are required.
//
// Key layout (all keys live in the same KV namespace as user data):
//
//	_coll:meta:<name>              JSON  CollectionMeta
//	_coll:rec:<name>:<id>          JSON  Record (id, created, updated, data)
//	_coll:idx:<name>:<field>:<v>:<id>   "" (unique-constraint marker, future)
//
// IDs are ULID-like — 26 chars of crockford-base32, time-prefixed so they
// sort by creation order. This means listing records via the engine's
// sorted Scan() yields chronological order by default.
package collections

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"solderdb/internal/engine"
)

type FieldType string

const (
	FieldText   FieldType = "text"
	FieldNumber FieldType = "number"
	FieldBool   FieldType = "bool"
	FieldJSON   FieldType = "json"
	FieldDate   FieldType = "date" // ISO-8601 string
)

type Field struct {
	Name     string    `json:"name"`
	Type     FieldType `json:"type"`
	Required bool      `json:"required,omitempty"`
	Unique   bool      `json:"unique,omitempty"`
}

type CollectionMeta struct {
	Name    string  `json:"name"`
	Fields  []Field `json:"fields"`
	Created string  `json:"created"`
	Updated string  `json:"updated"`
}

type Record struct {
	ID      string                 `json:"id"`
	Created string                 `json:"created"`
	Updated string                 `json:"updated"`
	Data    map[string]interface{} `json:"data"`
}

type ListResult struct {
	Records   []Record `json:"records"`
	NextAfter string   `json:"nextAfter"`
}

type Service struct {
	db *engine.DB
	mu sync.Mutex // serializes schema changes; record writes rely on engine locks
}

func New(db *engine.DB) *Service {
	return &Service{db: db}
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

const (
	metaPrefix = "_coll:meta:"
	recPrefix  = "_coll:rec:"
)

func metaKey(name string) string         { return metaPrefix + name }
func recKey(name, id string) string      { return recPrefix + name + ":" + id }
func recScanPrefix(name string) string   { return recPrefix + name + ":" }

// ---------------- Collections ----------------

func (s *Service) CreateCollection(meta CollectionMeta) (CollectionMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !nameRe.MatchString(meta.Name) {
		return CollectionMeta{}, fmt.Errorf("invalid collection name %q: must match %s", meta.Name, nameRe.String())
	}
	if err := validateFields(meta.Fields); err != nil {
		return CollectionMeta{}, err
	}
	if _, ok, err := s.getMeta(meta.Name); err != nil {
		return CollectionMeta{}, err
	} else if ok {
		return CollectionMeta{}, fmt.Errorf("collection %q already exists", meta.Name)
	}

	now := nowIso()
	meta.Created = now
	meta.Updated = now
	if err := s.putMeta(meta); err != nil {
		return CollectionMeta{}, err
	}
	return meta, nil
}

func (s *Service) UpdateCollection(name string, fields []Field) (CollectionMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateFields(fields); err != nil {
		return CollectionMeta{}, err
	}
	meta, ok, err := s.getMeta(name)
	if err != nil {
		return CollectionMeta{}, err
	}
	if !ok {
		return CollectionMeta{}, fmt.Errorf("collection %q not found", name)
	}
	meta.Fields = fields
	meta.Updated = nowIso()
	if err := s.putMeta(meta); err != nil {
		return CollectionMeta{}, err
	}
	return meta, nil
}

func (s *Service) GetCollection(name string) (CollectionMeta, error) {
	meta, ok, err := s.getMeta(name)
	if err != nil {
		return CollectionMeta{}, err
	}
	if !ok {
		return CollectionMeta{}, fmt.Errorf("collection %q not found", name)
	}
	return meta, nil
}

func (s *Service) ListCollections() ([]CollectionMeta, error) {
	keys, err := s.db.ListKeys(engine.ListKeysOptions{Prefix: metaPrefix, Limit: 0})
	if err != nil {
		return nil, err
	}
	out := make([]CollectionMeta, 0, len(keys))
	for _, k := range keys {
		val, ok := s.db.Get(k)
		if !ok {
			continue
		}
		var m CollectionMeta
		if err := json.Unmarshal([]byte(val), &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// DeleteCollection removes the metadata AND every record in the collection.
// This is a hard delete — relies on tombstones in the engine; run Compact() to reclaim space.
func (s *Service) DeleteCollection(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok, err := s.getMeta(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("collection %q not found", name)
	}

	// Delete all records first.
	keys, err := s.db.ListKeys(engine.ListKeysOptions{Prefix: recScanPrefix(name), Limit: 0})
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := s.db.Delete(k); err != nil {
			return err
		}
	}
	return s.db.Delete(metaKey(name))
}

// ---------------- Records ----------------

func (s *Service) Insert(collection string, data map[string]interface{}) (Record, error) {
	meta, ok, err := s.getMeta(collection)
	if err != nil {
		return Record{}, err
	}
	if !ok {
		return Record{}, fmt.Errorf("collection %q not found", collection)
	}
	if err := validateRecord(meta, data); err != nil {
		return Record{}, err
	}
	now := nowIso()
	id := newID()
	rec := Record{
		ID:      id,
		Created: now,
		Updated: now,
		Data:    coerceTypes(meta, data),
	}
	if err := s.putRecord(collection, rec); err != nil {
		return Record{}, err
	}
	return rec, nil
}

func (s *Service) GetRecord(collection, id string) (Record, error) {
	val, ok := s.db.Get(recKey(collection, id))
	if !ok {
		return Record{}, fmt.Errorf("record %q not found in %q", id, collection)
	}
	var rec Record
	if err := json.Unmarshal([]byte(val), &rec); err != nil {
		return Record{}, fmt.Errorf("decode record: %w", err)
	}
	return rec, nil
}

func (s *Service) UpdateRecord(collection, id string, patch map[string]interface{}) (Record, error) {
	meta, ok, err := s.getMeta(collection)
	if err != nil {
		return Record{}, err
	}
	if !ok {
		return Record{}, fmt.Errorf("collection %q not found", collection)
	}
	rec, err := s.GetRecord(collection, id)
	if err != nil {
		return Record{}, err
	}
	// Merge patch into current data.
	merged := make(map[string]interface{}, len(rec.Data)+len(patch))
	for k, v := range rec.Data {
		merged[k] = v
	}
	for k, v := range patch {
		merged[k] = v
	}
	if err := validateRecord(meta, merged); err != nil {
		return Record{}, err
	}
	rec.Data = coerceTypes(meta, merged)
	rec.Updated = nowIso()
	if err := s.putRecord(collection, rec); err != nil {
		return Record{}, err
	}
	return rec, nil
}

func (s *Service) DeleteRecord(collection, id string) error {
	if _, ok := s.db.Get(recKey(collection, id)); !ok {
		return fmt.Errorf("record %q not found in %q", id, collection)
	}
	return s.db.Delete(recKey(collection, id))
}

func (s *Service) ListRecords(collection string, after string, limit int) (ListResult, error) {
	if limit <= 0 {
		limit = 50
	}
	scanRes, err := s.db.Scan(engine.ScanOptions{
		Prefix: recScanPrefix(collection),
		After:  after,
		Limit:  limit,
	})
	if err != nil {
		return ListResult{}, err
	}
	out := make([]Record, 0, len(scanRes.Keys))
	for _, k := range scanRes.Keys {
		val, ok := s.db.Get(k)
		if !ok {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(val), &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return ListResult{Records: out, NextAfter: scanRes.NextAfter}, nil
}

// ---------------- internals ----------------

func (s *Service) getMeta(name string) (CollectionMeta, bool, error) {
	val, ok := s.db.Get(metaKey(name))
	if !ok {
		return CollectionMeta{}, false, nil
	}
	var m CollectionMeta
	if err := json.Unmarshal([]byte(val), &m); err != nil {
		return CollectionMeta{}, false, fmt.Errorf("decode collection meta: %w", err)
	}
	return m, true, nil
}

func (s *Service) putMeta(m CollectionMeta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode collection meta: %w", err)
	}
	return s.db.Set(metaKey(m.Name), string(b))
}

func (s *Service) putRecord(collection string, rec Record) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("encode record: %w", err)
	}
	return s.db.Set(recKey(collection, rec.ID), string(b))
}

func validateFields(fields []Field) error {
	if len(fields) == 0 {
		return errors.New("collection must have at least one field")
	}
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		if !nameRe.MatchString(f.Name) {
			return fmt.Errorf("invalid field name %q", f.Name)
		}
		if _, dup := seen[f.Name]; dup {
			return fmt.Errorf("duplicate field %q", f.Name)
		}
		seen[f.Name] = struct{}{}
		switch f.Type {
		case FieldText, FieldNumber, FieldBool, FieldJSON, FieldDate:
		default:
			return fmt.Errorf("unknown field type %q on %q", f.Type, f.Name)
		}
	}
	return nil
}

func validateRecord(meta CollectionMeta, data map[string]interface{}) error {
	allowed := make(map[string]Field, len(meta.Fields))
	for _, f := range meta.Fields {
		allowed[f.Name] = f
	}
	for name, val := range data {
		f, ok := allowed[name]
		if !ok {
			return fmt.Errorf("unknown field %q for collection %q", name, meta.Name)
		}
		if err := checkType(f, val); err != nil {
			return err
		}
	}
	for _, f := range meta.Fields {
		if !f.Required {
			continue
		}
		v, ok := data[f.Name]
		if !ok || isEmpty(v) {
			return fmt.Errorf("required field %q missing", f.Name)
		}
	}
	return nil
}

func checkType(f Field, val interface{}) error {
	if val == nil {
		return nil
	}
	switch f.Type {
	case FieldText, FieldDate:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("field %q must be string", f.Name)
		}
	case FieldNumber:
		switch val.(type) {
		case float64, int, int64, float32:
		default:
			return fmt.Errorf("field %q must be number", f.Name)
		}
	case FieldBool:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("field %q must be boolean", f.Name)
		}
	case FieldJSON:
		// Anything JSON-encodable is fine.
	}
	return nil
}

func coerceTypes(meta CollectionMeta, data map[string]interface{}) map[string]interface{} {
	// Currently a passthrough — present so future migrations (e.g. int -> float64)
	// have a single chokepoint.
	_ = meta
	return data
}

func isEmpty(v interface{}) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
		return true
	}
	return false
}

// ---------------- ID generation ----------------

// Crockford base32 alphabet (sortable).
const idAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newID returns a 26-char, time-prefixed, sortable ID similar to ULID.
// First 10 chars encode 48-bit milliseconds; last 16 are random.
func newID() string {
	ms := uint64(time.Now().UTC().UnixMilli())
	var buf [26]byte

	for i := 9; i >= 0; i-- {
		buf[i] = idAlphabet[ms&0x1F]
		ms >>= 5
	}
	var rnd [10]byte
	_, _ = rand.Read(rnd[:])
	for i := 0; i < 16; i++ {
		// pack pseudo-randomness across the 10 bytes; simple folding is fine here.
		buf[10+i] = idAlphabet[rnd[i%10]&0x1F]
	}
	return string(buf[:])
}

func nowIso() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
