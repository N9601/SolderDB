package collections

import (
	"sort"
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

func TestUpdateAfterFieldRemovedFromSchema(t *testing.T) {
	_, svc := openTestDB(t)
	if _, err := svc.CreateCollection(CollectionMeta{
		Name: "notes",
		Fields: []Field{
			{Name: "title", Type: FieldText, Required: true},
			{Name: "legacy", Type: FieldText},
		},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	rec, err := svc.Insert("notes", map[string]interface{}{"title": "a", "legacy": "old"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Drop "legacy" from the schema; the stored value stays on the record.
	if _, err := svc.UpdateCollection("notes", CollectionPatch{
		Fields: []Field{{Name: "title", Type: FieldText, Required: true}},
	}); err != nil {
		t.Fatalf("update schema: %v", err)
	}

	got, err := svc.UpdateRecord("notes", rec.ID, map[string]interface{}{"title": "b"})
	if err != nil {
		t.Fatalf("update after field removal: %v", err)
	}
	if got.Data["title"] != "b" || got.Data["legacy"] != "old" {
		t.Fatalf("unexpected data after update: %+v", got.Data)
	}

	// The patch itself is still validated against the current schema.
	if _, err := svc.UpdateRecord("notes", rec.ID, map[string]interface{}{"legacy": "new"}); err == nil {
		t.Fatalf("expected error when patching a removed field")
	}
	if _, err := svc.UpdateRecord("notes", rec.ID, map[string]interface{}{"title": ""}); err == nil {
		t.Fatalf("expected error when clearing a required field")
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

func TestListRecordsPaginatesByID(t *testing.T) {
	_, svc := openTestDB(t)
	if _, err := svc.CreateCollection(CollectionMeta{
		Name:   "events",
		Fields: []Field{{Name: "n", Type: FieldNumber}},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	var ids []string
	for i := 0; i < 5; i++ {
		rec, err := svc.Insert("events", map[string]interface{}{"n": float64(i)})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		ids = append(ids, rec.ID)
	}
	sort.Strings(ids)

	page1, err := svc.ListRecords("events", "", 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page1.Records) != 2 || page1.NextAfter != ids[1] {
		t.Fatalf("page 1: got %d records, nextAfter=%q; want 2 and %q", len(page1.Records), page1.NextAfter, ids[1])
	}

	// The cursor is a record ID, so a client can also resume after any ID it saw.
	page2, err := svc.ListRecords("events", page1.NextAfter, 10)
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if len(page2.Records) != 3 || page2.Records[0].ID != ids[2] || page2.NextAfter != "" {
		t.Fatalf("page 2: got %+v", page2)
	}

	// A full storage key (the old cursor shape) still works.
	legacy, err := svc.ListRecords("events", recKey("events", ids[3]), 10)
	if err != nil {
		t.Fatalf("list legacy cursor: %v", err)
	}
	if len(legacy.Records) != 1 || legacy.Records[0].ID != ids[4] {
		t.Fatalf("legacy cursor: got %+v", legacy)
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
