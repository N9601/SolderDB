package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"solderdb/internal/auth"
	"solderdb/internal/collections"
	"solderdb/internal/engine"
	"solderdb/internal/files"
	"solderdb/internal/logs"
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
	srv := New(db, colls, authSvc, hub, fileSvc, logs.New(100), Config{Addr: "127.0.0.1:0"})
	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	return srv, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
		_ = db.Close()
		testToken = ""
	}
}

func TestFilesUploadDownloadDelete(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()
	registerAdminAndAuth(t, base)

	// Raw upload via X-Filename header.
	body := strings.Repeat("solder", 1000)
	req, _ := http.NewRequest(http.MethodPost, base+"/api/files", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-Filename", "blob.txt")
	if testToken != "" {
		req.Header.Set("Authorization", "Bearer "+testToken)
	}
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

func TestFileDownloadHeaders(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()
	registerAdminAndAuth(t, base)

	upload := func(name, mimeType string) string {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, base+"/api/files", strings.NewReader("<script>alert(1)</script>"))
		req.Header.Set("Content-Type", mimeType)
		req.Header.Set("X-Filename", name)
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("upload: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		var meta struct{ ID string }
		if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil || resp.StatusCode != 201 {
			t.Fatalf("upload %s code=%d err=%v", name, resp.StatusCode, err)
		}
		return meta.ID
	}
	download := func(id string) http.Header {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, base+"/api/files/"+id, nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		_ = resp.Body.Close()
		return resp.Header
	}

	for _, c := range []struct {
		name, mimeType, disposition string
	}{
		{`page "one".html`, "text/html", "attachment"},
		{"drawing.svg", "image/svg+xml", "attachment"},
		{"photo.png", "image/png", "inline"},
		{"notes.txt", "text/plain; charset=utf-8", "inline"},
	} {
		h := download(upload(c.name, c.mimeType))
		if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options=%q, want nosniff", c.name, got)
		}
		disp, params, err := mime.ParseMediaType(h.Get("Content-Disposition"))
		if err != nil {
			t.Errorf("%s: unparsable Content-Disposition %q: %v", c.name, h.Get("Content-Disposition"), err)
			continue
		}
		if disp != c.disposition || params["filename"] != c.name {
			t.Errorf("%s: got %s filename=%q, want %s", c.name, disp, params["filename"], c.disposition)
		}
	}
}

func doRaw(t *testing.T, method, url string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	if testToken != "" {
		req.Header.Set("Authorization", "Bearer "+testToken)
	}
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
	token := registerAdminAndAuth(t, base)

	// Set up the collection first.
	if code, body := doJSON(t, http.MethodPost, base+"/api/collections", map[string]any{
		"name":   "live",
		"fields": []map[string]any{{"name": "x", "type": "text", "required": true}},
	}); code != 201 {
		t.Fatalf("setup coll code=%d body=%s", code, body)
	}

	// Open SSE stream, EventSource-style uses token query param.
	req, _ := http.NewRequest(http.MethodGet, base+"/api/realtime?topic=coll:live&token="+token, nil)
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

// readSSEUntil reads the stream until done reports true for everything read
// so far, or fails the test after a timeout. It returns the text read.
func readSSEUntil(t *testing.T, body io.Reader, done func(string) bool) string {
	t.Helper()
	result := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		var carry string
		for {
			n, err := body.Read(buf)
			carry += string(buf[:n])
			if done(carry) || err != nil {
				result <- carry
				return
			}
		}
	}()
	select {
	case got := <-result:
		if !done(got) {
			t.Fatalf("stream ended early, got:\n%s", got)
		}
		return got
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for SSE events")
		return ""
	}
}

func TestSSEFiltersEventsByAccess(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	// Mirror main.go: activity log entries are published on the hub.
	srv.logs.SetPublisher(func(e logs.Entry) {
		srv.hub.Publish("logs", realtime.Event{Kind: realtime.EventCreate, Data: e, Timestamp: e.Timestamp})
	})
	base := "http://" + srv.Addr()
	adminTok := registerAdminAndAuth(t, base)
	userTok := registerUser(t, base, "user@test.local")

	for _, c := range []map[string]any{
		{"name": "secret", "listRule": "admin", "viewRule": "admin",
			"fields": []map[string]any{{"name": "x", "type": "text"}}},
		{"name": "open",
			"fields": []map[string]any{{"name": "x", "type": "text"}}},
	} {
		if code, body := doJSON(t, http.MethodPost, base+"/api/collections", c); code != 201 {
			t.Fatalf("create %v code=%d body=%s", c["name"], code, body)
		}
	}

	// A regular user subscribes to everything.
	resp, err := http.Get(base + "/api/realtime?topic=*&token=" + userTok)
	if err != nil {
		t.Fatalf("sse: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("sse code=%d", resp.StatusCode)
	}
	readSSEUntil(t, resp.Body, func(s string) bool { return strings.Contains(s, ": connected") })

	// A new registration writes a _users record (with its password hash).
	registerUser(t, base, "other@test.local")
	testToken = adminTok
	for _, coll := range []string{"secret", "open"} {
		if code, body := doJSON(t, http.MethodPost, base+"/api/collections/"+coll+"/records", map[string]any{"x": coll}); code != 201 {
			t.Fatalf("insert into %s code=%d body=%s", coll, code, body)
		}
	}

	got := readSSEUntil(t, resp.Body, func(s string) bool { return strings.Contains(s, `"collection":"open"`) })
	for _, leak := range []string{`"collection":"_users"`, "password_hash", `"collection":"secret"`, `"method":`} {
		if strings.Contains(got, leak) {
			t.Fatalf("regular user received %s over SSE:\n%s", leak, got)
		}
	}
}

func TestLogEntriesRecordUser(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()
	registerAdminAndAuth(t, base)

	if code, _ := doJSON(t, http.MethodGet, base+"/api/collections", nil); code != 200 {
		t.Fatalf("list collections code=%d", code)
	}
	code, body := doJSON(t, http.MethodGet, base+"/api/logs", nil)
	if code != 200 {
		t.Fatalf("logs code=%d body=%s", code, body)
	}
	var entries []logs.Entry
	if err := json.Unmarshal(body, &entries); err != nil {
		t.Fatalf("decode logs: %v", err)
	}
	for _, e := range entries {
		if e.Path == "/api/collections" {
			if e.User != "admin@test.local" {
				t.Fatalf("expected log entry user admin@test.local, got %q", e.User)
			}
			return
		}
	}
	t.Fatalf("no log entry for /api/collections in %s", body)
}

func TestCollectionRulesEnforced(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()
	adminTok := registerAdminAndAuth(t, base)

	// Create a collection.
	if code, body := doJSON(t, http.MethodPost, base+"/api/collections", map[string]any{
		"name":   "posts",
		"fields": []map[string]any{{"name": "title", "type": "text", "required": true}},
	}); code != 201 {
		t.Fatalf("create code=%d body=%s", code, body)
	}

	// Make list public, create admin-only.
	code, _ := doJSON(t, http.MethodPatch, base+"/api/collections/posts", map[string]any{
		"listRule":   "public",
		"createRule": "admin",
	})
	if code != 200 {
		t.Fatalf("patch rules code=%d", code)
	}

	// Anonymous can list (drop the token momentarily).
	testToken = ""
	if code, _ := doJSON(t, http.MethodGet, base+"/api/collections/posts/records", nil); code != 200 {
		t.Fatalf("public list should return 200, got %d", code)
	}
	// Anonymous create should fail.
	if code, _ := doJSON(t, http.MethodPost, base+"/api/collections/posts/records", map[string]any{"title": "x"}); code != 401 {
		t.Fatalf("anonymous create should 401, got %d", code)
	}

	// Non-admin user can't create either.
	_, body := doJSON(t, http.MethodPost, base+"/api/auth/register", map[string]any{
		"email": "user2@test.local", "password": "anothersecret",
	})
	var sess struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(body, &sess)
	testToken = sess.Token
	if code, _ := doJSON(t, http.MethodPost, base+"/api/collections/posts/records", map[string]any{"title": "x"}); code != 403 {
		t.Fatalf("non-admin create should 403, got %d", code)
	}

	// Admin can.
	testToken = adminTok
	if code, _ := doJSON(t, http.MethodPost, base+"/api/collections/posts/records", map[string]any{"title": "x"}); code != 201 {
		t.Fatalf("admin create should 201, got %d", code)
	}
}

func TestInternalCollectionsBlocked(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()
	registerAdminAndAuth(t, base)

	// Even as admin, hitting _users records via the public endpoint is forbidden.
	if code, _ := doJSON(t, http.MethodGet, base+"/api/collections/_users/records", nil); code != 403 {
		t.Fatalf("_users access should 403, got %d", code)
	}
}

func TestAuthFlow(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()

	// Register first user, should become admin.
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

// testToken is set by tests that want every doJSON call to carry a bearer.
// Tests can set it after registering an admin and clear it to test 401s.
var testToken string

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
	if testToken != "" {
		req.Header.Set("Authorization", "Bearer "+testToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

// registerAdminAndAuth registers a fresh admin and sets testToken so
// subsequent doJSON calls carry the bearer header.
func registerAdminAndAuth(t *testing.T, base string) string {
	t.Helper()
	code, body := doJSON(t, http.MethodPost, base+"/api/auth/register", map[string]any{
		"email":    "admin@test.local",
		"password": "supersecret",
	})
	if code != 201 {
		t.Fatalf("register admin code=%d body=%s", code, body)
	}
	var sess struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &sess); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	testToken = sess.Token
	return sess.Token
}

func TestAuthEnforcementOnProtectedRoutes(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()
	// No token set, protected routes must reject.

	if code, _ := doJSON(t, http.MethodGet, base+"/api/collections", nil); code != 401 {
		t.Fatalf("expected 401 listing collections without token, got %d", code)
	}
	if code, _ := doJSON(t, http.MethodGet, base+"/api/stats", nil); code != 401 {
		t.Fatalf("expected 401 on stats without token, got %d", code)
	}
	// Public still works.
	if code, _ := doJSON(t, http.MethodGet, base+"/api/health", nil); code != 200 {
		t.Fatalf("expected 200 on health, got %d", code)
	}

	// Register a non-admin user.
	_ = registerAdminAndAuth(t, base) // admin slot taken
	code, body := doJSON(t, http.MethodPost, base+"/api/auth/register", map[string]any{
		"email": "user@test.local", "password": "anothersecret",
	})
	if code != 201 {
		t.Fatalf("register user code=%d body=%s", code, body)
	}
	var sess struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(body, &sess)
	testToken = sess.Token

	// Non-admin cannot create a collection.
	code, _ = doJSON(t, http.MethodPost, base+"/api/collections", map[string]any{
		"name": "x", "fields": []map[string]any{{"name": "n", "type": "text"}},
	})
	if code != 403 {
		t.Fatalf("expected 403 for non-admin create, got %d", code)
	}
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
	registerAdminAndAuth(t, base)

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
	registerAdminAndAuth(t, base)

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

// registerUser registers a non-admin account (an admin must already exist)
// and returns its token without touching testToken.
func registerUser(t *testing.T, base, email string) string {
	t.Helper()
	saved := testToken
	testToken = ""
	code, body := doJSON(t, http.MethodPost, base+"/api/auth/register", map[string]any{
		"email": email, "password": "anothersecret",
	})
	testToken = saved
	if code != 201 {
		t.Fatalf("register %s code=%d body=%s", email, code, body)
	}
	var sess struct {
		Token string `json:"token"`
		User  struct {
			Role string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &sess); err != nil || sess.User.Role != "user" {
		t.Fatalf("expected a regular user session, got %s (err %v)", body, err)
	}
	return sess.Token
}

func TestKVReadsRequireAdmin(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()
	adminTok := registerAdminAndAuth(t, base)
	userTok := registerUser(t, base, "user@test.local")

	// Look up the admin's own user record key, which holds the bcrypt hash.
	testToken = adminTok
	code, body := doJSON(t, http.MethodGet, base+"/api/auth/me", nil)
	if code != 200 {
		t.Fatalf("me code=%d", code)
	}
	var me struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &me)
	key := "_coll:rec:_users:" + me.ID

	testToken = userTok
	if code, body := doJSON(t, http.MethodGet, base+"/api/kv/"+key, nil); code != 403 {
		t.Fatalf("non-admin raw KV read should 403, got %d body=%s", code, body)
	}
	testToken = adminTok
	if code, _ := doJSON(t, http.MethodGet, base+"/api/kv/"+key, nil); code != 200 {
		t.Fatalf("admin raw KV read should 200, got %d", code)
	}
}

// repeatByte is an endless reader of one byte value.
type repeatByte byte

func (b repeatByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}

func TestDecodeBodyRejectsOversizedBody(t *testing.T) {
	body := io.MultiReader(
		strings.NewReader(`{"value":"`),
		io.LimitReader(repeatByte('x'), maxJSONBody),
		strings.NewReader(`"}`),
	)
	req := httptest.NewRequest(http.MethodPut, "/api/kv/big", body)
	rec := httptest.NewRecorder()
	var v struct {
		Value string `json:"value"`
	}
	err := decodeBody(rec, req, &v)
	if err == nil {
		t.Fatalf("expected an error for a body over %d bytes", maxJSONBody)
	}
	writeDecodeError(rec, err)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rec.Code, rec.Body.String())
	}

	// A small body still decodes.
	req = httptest.NewRequest(http.MethodPut, "/api/kv/small", strings.NewReader(`{"value":"ok"}`))
	if err := decodeBody(httptest.NewRecorder(), req, &v); err != nil || v.Value != "ok" {
		t.Fatalf("small body: value=%q err=%v", v.Value, err)
	}
}

func TestKVEndpoints(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()
	base := "http://" + srv.Addr()
	registerAdminAndAuth(t, base)

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
