package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"solderdb/internal/auth"
	"solderdb/internal/collections"
	"solderdb/internal/engine"
	"solderdb/internal/files"
	"solderdb/internal/realtime"
)

func startTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := engine.Open(engine.Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	colls := collections.New(db)
	hub := realtime.NewHub()
	colls.SetNotifier(realtime.CollectionsNotifier{Hub: hub})
	authSvc, err := auth.New(db, colls, dir)
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	fileSvc, err := files.New(colls, dir)
	if err != nil {
		t.Fatalf("files: %v", err)
	}
	srv := New(db, colls, authSvc, hub, fileSvc, Config{Addr: "127.0.0.1:0"})
	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	return srv, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
		_ = db.Close()
	}
}

func TestFilesUploadDownloadDelete(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()

	// Raw upload via X-Filename header.
	body := strings.Repeat("solder", 1000)
	req, _ := http.NewRequest(http.MethodPost, base+"/api/files", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-Filename", "blob.txt")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	out, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("upload code=%d body=%s", resp.StatusCode, out)
	}
	var meta struct {
		ID, Name, MimeType, SHA256 string
		Size                       int64
	}
	if err := json.Unmarshal(out, &meta); err != nil || meta.ID == "" || meta.Size != int64(len(body)) {
		t.Fatalf("bad meta: %s err=%v", out, err)
	}

	// Download and verify.
	code, raw := doRaw(t, http.MethodGet, base+"/api/files/"+meta.ID)
	if code != 200 || string(raw) != body {
		t.Fatalf("download code=%d len=%d", code, len(raw))
	}

	// List.
	code, listBody := doJSON(t, http.MethodGet, base+"/api/files", nil)
	if code != 200 || !strings.Contains(string(listBody), meta.ID) {
		t.Fatalf("list code=%d body=%s", code, listBody)
	}

	// Delete.
	code, _ = doJSON(t, http.MethodDelete, base+"/api/files/"+meta.ID, nil)
	if code != 200 {
		t.Fatalf("delete code=%d", code)
	}
	// Now 404.
	code, _ = doJSON(t, http.MethodGet, base+"/api/files/"+meta.ID, nil)
	if code != 404 {
		t.Fatalf("expected 404 after delete, got %d", code)
	}
}

func doRaw(t *testing.T, method, url string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func TestSSEReceivesCollectionEvents(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()

	// Set up the collection first.
	if code, body := doJSON(t, http.MethodPost, base+"/api/collections", map[string]any{
		"name":   "live",
		"fields": []map[string]any{{"name": "x", "type": "text", "required": true}},
	}); code != 201 {
		t.Fatalf("setup coll code=%d body=%s", code, body)
	}

	// Open SSE stream.
	req, _ := http.NewRequest(http.MethodGet, base+"/api/realtime?topic=coll:live", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sse: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("sse code=%d", resp.StatusCode)
	}

	// Insert a record concurrently.
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = doJSON(t, http.MethodPost, base+"/api/collections/live/records", map[string]any{"x": "hi"})
	}()

	// Read in a goroutine; wait up to 3s for the create event.
	gotCreate := make(chan bool, 1)
	go func() {
		buf := make([]byte, 4096)
		var carry string
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				carry += string(buf[:n])
				if strings.Contains(carry, "event: create") && strings.Contains(carry, `"collection":"live"`) {
					gotCreate <- true
					return
				}
			}
			if err != nil {
				gotCreate <- false
				return
			}
		}
	}()

	select {
	case ok := <-gotCreate:
		if !ok {
			t.Fatal("stream closed before create event")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive create event over SSE")
	}
}

func TestAuthFlow(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()

	// Register first user — should become admin.
	code, body := doJSON(t, http.MethodPost, base+"/api/auth/register", map[string]any{
		"email":    "alice@example.com",
		"password": "supersecret",
	})
	if code != 201 {
		t.Fatalf("register code=%d body=%s", code, body)
	}
	var sess struct {
		Token string `json:"token"`
		User  struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &sess); err != nil {
		t.Fatalf("decode session: %v body=%s", err, body)
	}
	if sess.Token == "" || sess.User.Role != "admin" {
		t.Fatalf("expected token + admin role, got %+v", sess)
	}

	// Duplicate registration -> 400.
	code, _ = doJSON(t, http.MethodPost, base+"/api/auth/register", map[string]any{
		"email": "alice@example.com", "password": "supersecret",
	})
	if code != 400 {
		t.Fatalf("expected 400 on duplicate, got %d", code)
	}

	// Wrong password -> 401.
	code, _ = doJSON(t, http.MethodPost, base+"/api/auth/login", map[string]any{
		"email": "alice@example.com", "password": "wrong",
	})
	if code != 401 {
		t.Fatalf("expected 401, got %d", code)
	}

	// Correct login.
	code, body = doJSON(t, http.MethodPost, base+"/api/auth/login", map[string]any{
		"email": "alice@example.com", "password": "supersecret",
	})
	if code != 200 {
		t.Fatalf("login code=%d body=%s", code, body)
	}
	if err := json.Unmarshal(body, &sess); err != nil {
		t.Fatalf("decode session: %v", err)
	}

	// /me with token.
	req, _ := http.NewRequest(http.MethodGet, base+"/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("me code=%d body=%s", resp.StatusCode, out)
	}

	// /me without token -> 401.
	code, _ = doJSON(t, http.MethodGet, base+"/api/auth/me", nil)
	if code != 401 {
		t.Fatalf("expected 401 without token, got %d", code)
	}

	// /me with garbage token -> 401.
	req2, _ := http.NewRequest(http.MethodGet, base+"/api/auth/me", nil)
	req2.Header.Set("Authorization", "Bearer not.a.token")
	resp2, _ := http.DefaultClient.Do(req2)
	if resp2.StatusCode != 401 {
		t.Fatalf("expected 401 with bad token, got %d", resp2.StatusCode)
	}
	_ = resp2.Body.Close()

	// Second registration should be a regular user, not admin.
	code, body = doJSON(t, http.MethodPost, base+"/api/auth/register", map[string]any{
		"email": "bob@example.com", "password": "anothersecret",
	})
	if code != 201 {
		t.Fatalf("register bob code=%d body=%s", code, body)
	}
	_ = json.Unmarshal(body, &sess)
	if sess.User.Role != "user" {
		t.Fatalf("expected bob to be 'user', got %q", sess.User.Role)
	}
}

func doJSON(t *testing.T, method, url string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func TestHealth(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	code, body := doJSON(t, http.MethodGet, "http://"+srv.Addr()+"/api/health", nil)
	if code != 200 {
		t.Fatalf("health code=%d body=%s", code, body)
	}
	if !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("unexpected health body: %s", body)
	}
}

func TestCollectionsCRUDOverHTTP(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()

	// Create a collection.
	createBody := map[string]any{
		"name": "tasks",
		"fields": []map[string]any{
			{"name": "title", "type": "text", "required": true},
			{"name": "done", "type": "bool"},
		},
	}
	code, body := doJSON(t, http.MethodPost, base+"/api/collections", createBody)
	if code != 201 {
		t.Fatalf("create coll code=%d body=%s", code, body)
	}

	// List.
	code, body = doJSON(t, http.MethodGet, base+"/api/collections", nil)
	if code != 200 || !strings.Contains(string(body), `"tasks"`) {
		t.Fatalf("list coll code=%d body=%s", code, body)
	}

	// Insert a record.
	rec := map[string]any{"title": "ship it", "done": false}
	code, body = doJSON(t, http.MethodPost, base+"/api/collections/tasks/records", rec)
	if code != 201 {
		t.Fatalf("insert code=%d body=%s", code, body)
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.ID == "" {
		t.Fatalf("bad insert body: %s err=%v", body, err)
	}

	// Get the record.
	code, body = doJSON(t, http.MethodGet, base+"/api/collections/tasks/records/"+parsed.ID, nil)
	if code != 200 || !strings.Contains(string(body), `"ship it"`) {
		t.Fatalf("get record code=%d body=%s", code, body)
	}

	// Patch it.
	patch := map[string]any{"done": true}
	code, body = doJSON(t, http.MethodPatch, base+"/api/collections/tasks/records/"+parsed.ID, patch)
	if code != 200 || !strings.Contains(string(body), `"done":true`) {
		t.Fatalf("patch code=%d body=%s", code, body)
	}

	// List records.
	code, body = doJSON(t, http.MethodGet, base+"/api/collections/tasks/records", nil)
	if code != 200 || !strings.Contains(string(body), parsed.ID) {
		t.Fatalf("list records code=%d body=%s", code, body)
	}

	// Delete it.
	code, _ = doJSON(t, http.MethodDelete, base+"/api/collections/tasks/records/"+parsed.ID, nil)
	if code != 200 {
		t.Fatalf("delete code=%d", code)
	}

	// Delete the collection.
	code, _ = doJSON(t, http.MethodDelete, base+"/api/collections/tasks", nil)
	if code != 200 {
		t.Fatalf("delete coll code=%d", code)
	}
}

func TestValidationErrors(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()

	// Create coll with required title.
	_, _ = doJSON(t, http.MethodPost, base+"/api/collections", map[string]any{
		"name":   "n",
		"fields": []map[string]any{{"name": "title", "type": "text", "required": true}},
	})

	// Missing required field -> 400.
	code, body := doJSON(t, http.MethodPost, base+"/api/collections/n/records", map[string]any{})
	if code != 400 {
		t.Fatalf("expected 400, got %d body=%s", code, body)
	}

	// Unknown field -> 400.
	code, body = doJSON(t, http.MethodPost, base+"/api/collections/n/records", map[string]any{"title": "x", "ghost": 1})
	if code != 400 {
		t.Fatalf("expected 400 for unknown field, got %d body=%s", code, body)
	}
}

func TestKVEndpoints(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()

	// PUT
	code, _ := doJSON(t, http.MethodPut, base+"/api/kv/foo", map[string]any{"value": "bar"})
	if code != 200 {
		t.Fatalf("put code=%d", code)
	}
	// GET
	code, body := doJSON(t, http.MethodGet, base+"/api/kv/foo", nil)
	if code != 200 || !strings.Contains(string(body), `"value":"bar"`) {
		t.Fatalf("get code=%d body=%s", code, body)
	}
	// DELETE
	code, _ = doJSON(t, http.MethodDelete, base+"/api/kv/foo", nil)
	if code != 200 {
		t.Fatalf("delete code=%d", code)
	}
	// GET after delete
	code, _ = doJSON(t, http.MethodGet, base+"/api/kv/foo", nil)
	if code != 404 {
		t.Fatalf("expected 404, got %d", code)
	}
}
