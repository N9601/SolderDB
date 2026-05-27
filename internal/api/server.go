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
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"solderdb/internal/collections"
	"solderdb/internal/engine"
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
	mux    *http.ServeMux
	srv    *http.Server
	mu     sync.Mutex
	listen net.Listener
}

func New(db *engine.DB, colls *collections.Service, cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8787"
	}
	s := &Server{cfg: cfg, db: db, colls: colls, mux: http.NewServeMux()}
	s.routes()
	s.srv = &http.Server{
		Handler:           s.withMiddleware(s.mux),
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
			Fields []collections.Field `json:"fields"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		m, err := s.colls.UpdateCollection(name, body.Fields)
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
	switch r.Method {
	case http.MethodGet:
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
	switch r.Method {
	case http.MethodGet:
		rec, err := s.colls.GetRecord(coll, id)
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, rec)
	case http.MethodPatch:
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
