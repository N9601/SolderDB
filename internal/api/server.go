// Package api exposes an HTTP/REST surface over the engine + collections
// services. Built on net/http only — no third-party routers.
//
// Routing is path-based with small handler-dispatch helpers. Every response
// is JSON. Errors use a consistent shape: { "error": "message" }.
//
// Listens on 127.0.0.1 by default so the Wails app + a local CLI/SDK can
// hit it, but external networks cannot, unless the user overrides Addr.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"solderdb/internal/auth"
	"solderdb/internal/collections"
	"solderdb/internal/engine"
	"solderdb/internal/files"
	"solderdb/internal/logs"
	"solderdb/internal/realtime"
)

type Config struct {
	// Addr to listen on (e.g. "127.0.0.1:8787"). Default: "127.0.0.1:8787".
	Addr string
	// AllowOrigin sets the Access-Control-Allow-Origin response header.
	// Empty string disables CORS. "*" allows everything (use only for local dev).
	AllowOrigin string
}

type Server struct {
	cfg    Config
	db     *engine.DB
	colls  *collections.Service
	auth   *auth.Service
	hub    *realtime.Hub
	files  *files.Service
	logs   *logs.Buffer
	mux    *http.ServeMux
	srv    *http.Server
	mu     sync.Mutex
	listen net.Listener
}

func New(db *engine.DB, colls *collections.Service, authSvc *auth.Service, hub *realtime.Hub, fileSvc *files.Service, logBuf *logs.Buffer, cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8787"
	}
	s := &Server{cfg: cfg, db: db, colls: colls, auth: authSvc, hub: hub, files: fileSvc, logs: logBuf, mux: http.NewServeMux()}
	s.routes()
	s.srv = &http.Server{
		Handler:           s.withMiddleware(s.logMiddleware(s.authMiddleware(s.mux))),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// Addr returns the actual bound address (useful when port 0 was requested).
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listen == nil {
		return s.cfg.Addr
	}
	return s.listen.Addr().String()
}

// Start binds the listener and serves in a goroutine. Returns when bound or
// on bind failure.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.listen != nil {
		s.mu.Unlock()
		return errors.New("api: already started")
	}
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("api listen: %w", err)
	}
	s.listen = ln
	s.mu.Unlock()
	go func() {
		_ = s.srv.Serve(ln)
	}()
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	srv := s.srv
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// ---------------- Routes ----------------

func (s *Server) routes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/stats", s.handleStats)
	s.mux.HandleFunc("/api/collections", s.handleCollections)
	s.mux.HandleFunc("/api/collections/", s.handleCollectionItem)
	s.mux.HandleFunc("/api/kv/", s.handleKV)
	s.mux.HandleFunc("/api/auth/register", s.handleRegister)
	s.mux.HandleFunc("/api/auth/login", s.handleLogin)
	s.mux.HandleFunc("/api/auth/me", s.handleMe)
	s.mux.HandleFunc("/api/auth/password", s.handleChangePassword)
	s.mux.HandleFunc("/api/realtime", s.handleRealtime)
	s.mux.HandleFunc("/api/files", s.handleFiles)
	s.mux.HandleFunc("/api/files/", s.handleFileItem)
	s.mux.HandleFunc("/api/logs", s.handleLogs)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	if s.logs == nil {
		writeJSON(w, 200, []any{})
		return
	}
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	writeJSON(w, 200, s.logs.Tail(limit))
}

// ---------------- File handlers ----------------

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	if s.files == nil {
		writeError(w, 500, "files not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		after := r.URL.Query().Get("after")
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		list, next, err := s.files.List(after, limit)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"files": list, "nextAfter": next})
	case http.MethodPost:
		s.handleFileUpload(w, r)
	default:
		writeError(w, 405, "method not allowed")
	}
}

// handleFileUpload accepts either multipart/form-data (one file under field
// "file") or a raw body. For the raw path, set X-Filename and Content-Type.
func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(files.MaxUploadSize)+1024)

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(int64(files.MaxUploadSize)); err != nil {
			writeError(w, 400, "parse multipart: "+err.Error())
			return
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			writeError(w, 400, "missing 'file' field")
			return
		}
		defer func() { _ = f.Close() }()
		mime := hdr.Header.Get("Content-Type")
		meta, err := s.files.Upload(hdr.Filename, mime, f)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 201, meta)
		return
	}

	name := r.Header.Get("X-Filename")
	if name == "" {
		writeError(w, 400, "X-Filename header required for raw upload")
		return
	}
	mime := ct
	if mime == "" {
		mime = "application/octet-stream"
	}
	meta, err := s.files.Upload(name, mime, r.Body)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, meta)
}

func (s *Server) handleFileItem(w http.ResponseWriter, r *http.Request) {
	if s.files == nil {
		writeError(w, 500, "files not configured")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/files/")
	if rest == "" {
		writeError(w, 404, "not found")
		return
	}
	id := rest
	meta := false
	if strings.HasSuffix(rest, "/meta") {
		id = strings.TrimSuffix(rest, "/meta")
		meta = true
	}

	switch r.Method {
	case http.MethodGet:
		if meta {
			m, err := s.files.Meta(id)
			if err != nil {
				writeError(w, 404, err.Error())
				return
			}
			writeJSON(w, 200, m)
			return
		}
		m, rc, err := s.files.Stream(id)
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		defer func() { _ = rc.Close() }()
		w.Header().Set("Content-Type", m.MimeType)
		w.Header().Set("Content-Length", strconv.FormatInt(m.Size, 10))
		w.Header().Set("Content-Disposition", `inline; filename="`+m.Name+`"`)
		w.Header().Set("X-File-SHA256", m.SHA256)
		w.WriteHeader(200)
		_, _ = io.Copy(w, rc)
	case http.MethodDelete:
		if err := s.files.Delete(id); err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": id})
	default:
		writeError(w, 405, "method not allowed")
	}
}

// ---------------- Realtime (SSE) ----------------

// handleRealtime upgrades the request to a Server-Sent Events stream.
// Query params:
//
//	?topic=coll:notes&topic=kv:*
//
// Multiple ?topic= values are OR'd. If none given, defaults to "*".
//
// Wire format:
//
//	event: <kind>
//	data: <json>
//	\n
func (s *Server) handleRealtime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	if s.hub == nil {
		writeError(w, 500, "realtime not configured")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming unsupported")
		return
	}

	topics := r.URL.Query()["topic"]
	if len(topics) == 0 {
		topics = []string{"*"}
	}
	ch, unsub := s.hub.Subscribe(topics...)
	defer unsub()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	_, _ = fmt.Fprintf(w, ": connected topics=%v\n\n", topics)
	flusher.Flush()

	// Keep-alive ticker so proxies / clients don't decide we're dead.
	ka := time.NewTicker(20 * time.Second)
	defer ka.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ka.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case evt, ok := <-ch:
			if !ok {
				return
			}
			b, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Kind, b)
			flusher.Flush()
		}
	}
}

// ---------------- Auth handlers ----------------

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	if s.auth == nil {
		writeError(w, 500, "auth not configured")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	sess, err := s.auth.Register(body.Email, body.Password)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, sess)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	if s.auth == nil {
		writeError(w, 500, "auth not configured")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	sess, err := s.auth.Login(body.Email, body.Password)
	if err != nil {
		writeError(w, 401, err.Error())
		return
	}
	writeJSON(w, 200, sess)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	user, ok := s.currentUser(r)
	if !ok {
		writeError(w, 401, "authentication required")
		return
	}
	if s.auth == nil {
		writeError(w, 500, "auth not configured")
		return
	}
	var body struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	u, err := s.auth.ChangePassword(user.ID, body.Current, body.Next)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, u)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	user, ok := s.currentUser(r)
	if !ok {
		writeError(w, 401, "unauthenticated")
		return
	}
	writeJSON(w, 200, user)
}

// allowsQueryToken reports whether ?token=… is an acceptable auth fallback
// for this request. We restrict it to cases where the browser-initiated
// request (EventSource, <img>, <a download>) cannot set a custom header.
func allowsQueryToken(r *http.Request) bool {
	if r.URL.Path == "/api/realtime" {
		return true
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/files/") {
		return true
	}
	return false
}

// currentUser parses the Authorization header and returns the user, if any.
// For the SSE endpoint (/api/realtime) it also accepts ?token=... because
// EventSource cannot set custom headers in the browser.
func (s *Server) currentUser(r *http.Request) (auth.User, bool) {
	if s.auth == nil {
		return auth.User{}, false
	}
	token := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token = strings.TrimPrefix(h, "Bearer ")
	} else if allowsQueryToken(r) {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return auth.User{}, false
	}
	u, err := s.auth.VerifyToken(token)
	if err != nil {
		return auth.User{}, false
	}
	return u, true
}

// /api/collections/<name>[/records[/<id>]]
func (s *Server) handleCollectionItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/collections/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.SplitN(rest, "/", 3)
	name := parts[0]
	if name == "" {
		http.NotFound(w, r)
		return
	}
	switch {
	case len(parts) == 1:
		s.handleCollection(w, r, name)
	case len(parts) >= 2 && parts[1] == "records":
		if len(parts) == 2 {
			s.handleRecords(w, r, name)
		} else {
			s.handleRecord(w, r, name, parts[2])
		}
	default:
		http.NotFound(w, r)
	}
}

// ---------------- Handlers ----------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "service": "solderdb"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	st, err := s.db.Stats()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) handleCollections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.colls.ListCollections()
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		if list == nil {
			list = []collections.CollectionMeta{}
		}
		writeJSON(w, 200, list)
	case http.MethodPost:
		var meta collections.CollectionMeta
		if err := decodeBody(r, &meta); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		created, err := s.colls.CreateCollection(meta)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 201, created)
	default:
		writeError(w, 405, "method not allowed")
	}
}

func (s *Server) handleCollection(w http.ResponseWriter, r *http.Request, name string) {
	switch r.Method {
	case http.MethodGet:
		m, err := s.colls.GetCollection(name)
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, m)
	case http.MethodPatch:
		var body struct {
			Fields     []collections.Field `json:"fields"`
			ListRule   string              `json:"listRule"`
			ViewRule   string              `json:"viewRule"`
			CreateRule string              `json:"createRule"`
			UpdateRule string              `json:"updateRule"`
			DeleteRule string              `json:"deleteRule"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		patch := collections.CollectionPatch{Fields: body.Fields}
		if body.ListRule != "" {
			r := collections.Rule(body.ListRule)
			patch.ListRule = &r
		}
		if body.ViewRule != "" {
			r := collections.Rule(body.ViewRule)
			patch.ViewRule = &r
		}
		if body.CreateRule != "" {
			r := collections.Rule(body.CreateRule)
			patch.CreateRule = &r
		}
		if body.UpdateRule != "" {
			r := collections.Rule(body.UpdateRule)
			patch.UpdateRule = &r
		}
		if body.DeleteRule != "" {
			r := collections.Rule(body.DeleteRule)
			patch.DeleteRule = &r
		}
		m, err := s.colls.UpdateCollection(name, patch)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, m)
	case http.MethodDelete:
		if err := s.colls.DeleteCollection(name); err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": name})
	default:
		writeError(w, 405, "method not allowed")
	}
}

func (s *Server) handleRecords(w http.ResponseWriter, r *http.Request, coll string) {
	if strings.HasPrefix(coll, "_") {
		// Internal collections (e.g. _users, _files) are always admin-managed
		// over the API, no matter what rules someone tried to set.
		writeError(w, 403, "internal collection")
		return
	}
	meta, err := s.colls.GetCollection(coll)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	switch r.Method {
	case http.MethodGet:
		if _, ok := s.enforceRule(w, r, meta.ListRule); !ok {
			return
		}
		after := r.URL.Query().Get("after")
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		res, err := s.colls.ListRecords(coll, after, limit)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, res)
	case http.MethodPost:
		if _, ok := s.enforceRule(w, r, meta.CreateRule); !ok {
			return
		}
		var data map[string]any
		if err := decodeBody(r, &data); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		rec, err := s.colls.Insert(coll, data)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 201, rec)
	default:
		writeError(w, 405, "method not allowed")
	}
}

func (s *Server) handleRecord(w http.ResponseWriter, r *http.Request, coll, id string) {
	if strings.HasPrefix(coll, "_") {
		writeError(w, 403, "internal collection")
		return
	}
	meta, err := s.colls.GetCollection(coll)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	switch r.Method {
	case http.MethodGet:
		if _, ok := s.enforceRule(w, r, meta.ViewRule); !ok {
			return
		}
		rec, err := s.colls.GetRecord(coll, id)
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, rec)
	case http.MethodPatch:
		if _, ok := s.enforceRule(w, r, meta.UpdateRule); !ok {
			return
		}
		var patch map[string]any
		if err := decodeBody(r, &patch); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		rec, err := s.colls.UpdateRecord(coll, id, patch)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, rec)
	case http.MethodDelete:
		if _, ok := s.enforceRule(w, r, meta.DeleteRule); !ok {
			return
		}
		if err := s.colls.DeleteRecord(coll, id); err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": id})
	default:
		writeError(w, 405, "method not allowed")
	}
}

// /api/kv/<key> — raw KV access (for the dev/admin scenario; not the primary API).
func (s *Server) handleKV(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/kv/")
	if key == "" {
		writeError(w, 400, "key required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		val, ok := s.db.Get(key)
		if !ok {
			writeError(w, 404, "key not found")
			return
		}
		writeJSON(w, 200, map[string]any{"key": key, "value": val})
	case http.MethodPut:
		var body struct {
			Value string `json:"value"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		if err := s.db.Set(key, body.Value); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"key": key})
	case http.MethodDelete:
		if err := s.db.Delete(key); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": key})
	default:
		writeError(w, 405, "method not allowed")
	}
}

// ---------------- Middleware + helpers ----------------

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AllowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", s.cfg.AllowOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(204)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeBody(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
