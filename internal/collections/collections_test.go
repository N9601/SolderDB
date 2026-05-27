package collections

import (
	"testing"

	"solderdb/internal/engine"
)

func openTestDB(t *testing.T) (*engine.DB, *Service) {
	t.Helper()
	db, err := engine.Open(engine.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, New(db)
}

func TestCreateAndListCollections(t *testing.T) {
	_, svc := openTestDB(t)

	meta := CollectionMeta{
		Name: "users",
		Fields: []Field{
			{Name: "email", Type: FieldText, Required: true},
			{Name: "age", Type: FieldNumber},
		},
	}
	if _, err := svc.CreateCollection(meta); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.CreateCollection(meta); err == nil {
		t.Fatalf("expected duplicate-create error")
	}

	list, err := svc.ListCollections()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "users" {
		t.Fatalf("unexpected list: %+v", list)
	}
}

func TestInsertValidationAndRetrieval(t *testing.T) {
	_, svc := openTestDB(t)
	_, err := svc.CreateCollection(CollectionMeta{
		Name: "notes",
		Fields: []Field{
			{Name: "title", Type: FieldText, Required: true},
			{Name: "pinned", Type: FieldBool},
			{Name: "score", Type: FieldNumber},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Missing required field.
	if _, err := svc.Insert("notes", map[string]interface{}{"pinned": true}); err == nil {
		t.Fatalf("expected required-field error")
	}
	// Unknown field.
	if _, err := svc.Insert("notes", map[string]interface{}{"title": "x", "ghost": 1}); err == nil {
		t.Fatalf("expected unknown-field error")
	}
	// Wrong type.
	if _, err := svc.Insert("notes", map[string]interface{}{"title": "x", "pinned": "no"}); err == nil {
		t.Fatalf("expected type error")
	}

	rec, err := svc.Insert("notes", map[string]interface{}{
		"title":  "first",
		"pinned": true,
		"score":  float64(10),
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if rec.ID == "" || rec.Created == "" {
		t.Fatalf("missing id/created: %+v", rec)
	}

	got, err := svc.GetRecord("notes", rec.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Data["title"].(string) != "first" {
		t.Fatalf("title mismatch: %+v", got.Data)
	}
}

func TestUpdateMergesAndRevalidates(t *testing.T) {
	_, svc := openTestDB(t)
	_, _ = svc.CreateCollection(CollectionMeta{
		Name: "items",
		Fields: []Field{
			{Name: "name", Type: FieldText, Required: true},
			{Name: "price", Type: FieldNumber},
		},
	})
	rec, err := svc.Insert("items", map[string]interface{}{"name": "widget", "price": float64(5)})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	updated, err := svc.UpdateRecord("items", rec.ID, map[string]interface{}{"price": float64(7)})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Data["price"].(float64) != 7 {
		t.Fatalf("price not updated: %+v", updated.Data)
	}
	if updated.Data["name"].(string) != "widget" {
		t.Fatalf("name lost during merge: %+v", updated.Data)
	}
	if updated.Updated == rec.Updated {
		t.Fatalf("Updated timestamp not bumped")
	}
}

func TestListRecordsSortedByID(t *testing.T) {
	_, svc := openTestDB(t)
	_, _ = svc.CreateCollection(CollectionMeta{
		Name: "events",
		Fields: []Field{
			{Name: "kind", Type: FieldText, Required: true},
		},
	})
	for i := 0; i < 5; i++ {
		if _, err := svc.Insert("events", map[string]interface{}{"kind": "ping"}); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	res, err := svc.ListRecords("events", "", 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.Records) != 5 {
		t.Fatalf("expected 5 records, got %d", len(res.Records))
	}
	// IDs should be sortable; engine returns them in sort order.
	for i := 1; i < len(res.Records); i++ {
		if res.Records[i-1].ID >= res.Records[i].ID {
			t.Fatalf("records not in sorted ID order: %s >= %s", res.Records[i-1].ID, res.Records[i].ID)
		}
	}
}

func TestDeleteCollectionRemovesRecords(t *testing.T) {
	_, svc := openTestDB(t)
	_, _ = svc.CreateCollection(CollectionMeta{
		Name: "tmp",
		Fields: []Field{
			{Name: "x", Type: FieldText, Required: true},
		},
	})
	if _, err := svc.Insert("tmp", map[string]interface{}{"x": "a"}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := svc.DeleteCollection("tmp"); err != nil {
		t.Fatalf("delete coll: %v", err)
	}
	if _, err := svc.GetCollection("tmp"); err == nil {
		t.Fatalf("expected not-found after delete")
	}
	list, _ := svc.ListRecords("tmp", "", 100)
	if len(list.Records) != 0 {
		t.Fatalf("expected zero records after delete, got %d", len(list.Records))
	}
}
