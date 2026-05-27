package bridge

import (
	"fmt"

	"solderdb/internal/collections"
	"solderdb/internal/engine"
)

// CollectionsService is exposed to Wails so the frontend can manage typed
// record collections layered on top of the KV engine.
type CollectionsService struct {
	svc *collections.Service
}

func NewCollectionsService(db *engine.DB) *CollectionsService {
	return &CollectionsService{svc: collections.New(db)}
}

// --- DTOs ---
//
// Wails generates TypeScript classes from these. Keep them flat & JSON-friendly.

type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Unique   bool   `json:"unique"`
}

type CollectionMeta struct {
	Name    string  `json:"name"`
	Fields  []Field `json:"fields"`
	Created string  `json:"created"`
	Updated string  `json:"updated"`
}

type Document struct {
	ID      string                 `json:"id"`
	Created string                 `json:"created"`
	Updated string                 `json:"updated"`
	Data    map[string]interface{} `json:"data"`
}

type ListRecordsResult struct {
	Records   []Document `json:"records"`
	NextAfter string     `json:"nextAfter"`
}

// --- Conversions ---

func toEngineMeta(m CollectionMeta) collections.CollectionMeta {
	fs := make([]collections.Field, len(m.Fields))
	for i, f := range m.Fields {
		fs[i] = collections.Field{
			Name:     f.Name,
			Type:     collections.FieldType(f.Type),
			Required: f.Required,
			Unique:   f.Unique,
		}
	}
	return collections.CollectionMeta{Name: m.Name, Fields: fs}
}

func fromEngineMeta(m collections.CollectionMeta) CollectionMeta {
	fs := make([]Field, len(m.Fields))
	for i, f := range m.Fields {
		fs[i] = Field{
			Name:     f.Name,
			Type:     string(f.Type),
			Required: f.Required,
			Unique:   f.Unique,
		}
	}
	return CollectionMeta{Name: m.Name, Fields: fs, Created: m.Created, Updated: m.Updated}
}

func fromEngineRecord(r collections.Record) Document {
	return Document{ID: r.ID, Created: r.Created, Updated: r.Updated, Data: r.Data}
}

// --- API ---

func (c *CollectionsService) CreateCollection(meta CollectionMeta) (CollectionMeta, error) {
	if c.svc == nil {
		return CollectionMeta{}, fmt.Errorf("collections not initialized")
	}
	m, err := c.svc.CreateCollection(toEngineMeta(meta))
	if err != nil {
		return CollectionMeta{}, err
	}
	return fromEngineMeta(m), nil
}

func (c *CollectionsService) UpdateCollection(name string, fields []Field) (CollectionMeta, error) {
	tmp := CollectionMeta{Name: name, Fields: fields}
	em := toEngineMeta(tmp)
	m, err := c.svc.UpdateCollection(name, em.Fields)
	if err != nil {
		return CollectionMeta{}, err
	}
	return fromEngineMeta(m), nil
}

func (c *CollectionsService) GetCollection(name string) (CollectionMeta, error) {
	m, err := c.svc.GetCollection(name)
	if err != nil {
		return CollectionMeta{}, err
	}
	return fromEngineMeta(m), nil
}

func (c *CollectionsService) ListCollections() ([]CollectionMeta, error) {
	ms, err := c.svc.ListCollections()
	if err != nil {
		return nil, err
	}
	out := make([]CollectionMeta, len(ms))
	for i, m := range ms {
		out[i] = fromEngineMeta(m)
	}
	return out, nil
}

func (c *CollectionsService) DeleteCollection(name string) error {
	return c.svc.DeleteCollection(name)
}

func (c *CollectionsService) InsertRecord(collection string, data map[string]interface{}) (Document, error) {
	r, err := c.svc.Insert(collection, data)
	if err != nil {
		return Document{}, err
	}
	return fromEngineRecord(r), nil
}

func (c *CollectionsService) GetRecord(collection, id string) (Document, error) {
	r, err := c.svc.GetRecord(collection, id)
	if err != nil {
		return Document{}, err
	}
	return fromEngineRecord(r), nil
}

func (c *CollectionsService) UpdateRecord(collection, id string, patch map[string]interface{}) (Document, error) {
	r, err := c.svc.UpdateRecord(collection, id, patch)
	if err != nil {
		return Document{}, err
	}
	return fromEngineRecord(r), nil
}

func (c *CollectionsService) DeleteRecord(collection, id string) error {
	return c.svc.DeleteRecord(collection, id)
}

func (c *CollectionsService) ListRecords(collection, after string, limit int) (ListRecordsResult, error) {
	res, err := c.svc.ListRecords(collection, after, limit)
	if err != nil {
		return ListRecordsResult{}, err
	}
	out := make([]Document, len(res.Records))
	for i, r := range res.Records {
		out[i] = fromEngineRecord(r)
	}
	return ListRecordsResult{Records: out, NextAfter: res.NextAfter}, nil
}

// DB exposes the underlying engine for re-binding via main.
func (c *CollectionsService) bind() {}
