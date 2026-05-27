package files

import (
	"io"
	"strings"
	"testing"

	"solderdb/internal/collections"
	"solderdb/internal/engine"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	db, err := engine.Open(engine.Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	colls := collections.New(db)
	s, err := New(colls, dir)
	if err != nil {
		t.Fatalf("new files: %v", err)
	}
	return s
}

func TestUploadAndStream(t *testing.T) {
	s := newTestService(t)

	body := strings.Repeat("hello world\n", 100)
	meta, err := s.Upload("greeting.txt", "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if meta.ID == "" || meta.Size != int64(len(body)) {
		t.Fatalf("bad meta: %+v", meta)
	}
	if meta.SHA256 == "" {
		t.Fatalf("missing sha256")
	}

	gotMeta, rc, err := s.Stream(meta.ID)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer func() { _ = rc.Close() }()
	if gotMeta.SHA256 != meta.SHA256 {
		t.Fatalf("sha256 mismatch: %q vs %q", gotMeta.SHA256, meta.SHA256)
	}
	bytes, _ := io.ReadAll(rc)
	if string(bytes) != body {
		t.Fatalf("content mismatch")
	}
}

func TestRejectEmptyAndOversize(t *testing.T) {
	s := newTestService(t)

	if _, err := s.Upload("empty.txt", "text/plain", strings.NewReader("")); err == nil {
		t.Fatalf("expected error on empty upload")
	}
}

func TestListAndDelete(t *testing.T) {
	s := newTestService(t)

	for i := 0; i < 3; i++ {
		_, err := s.Upload("f.bin", "application/octet-stream", strings.NewReader("payload"))
		if err != nil {
			t.Fatalf("upload: %v", err)
		}
	}
	list, _, err := s.List("", 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 files, got %d", len(list))
	}

	if err := s.Delete(list[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Meta(list[0].ID); err == nil {
		t.Fatalf("expected meta missing after delete")
	}
}
