package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"solderdb/internal/auth"
	"solderdb/internal/logs"
)

// Authorization model:
//
//   public  — no token required (auth/login, auth/register, health)
//   authed  — any valid user token
//   admin   — token with role=admin
//
// Policy is decided per route below in routePolicy().

type policy int

const (
	policyPublic policy = iota
	policyAuthed
	policyAdmin
)

type ctxKey int

const ctxKeyUser ctxKey = 1

// authMiddleware wraps the mux. It looks up the policy for the request path
// and either allows it through, attaches the authenticated user to context,
// or rejects with 401/403.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pol := s.routePolicy(r)

		// Always allow CORS preflight unchanged — origin/method headers are
		// applied earlier in withMiddleware.
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		if pol == policyPublic || s.auth == nil {
			next.ServeHTTP(w, r)
			return
		}

		user, ok := s.currentUser(r)
		if !ok {
			writeError(w, 401, "authentication required")
			return
		}
		if pol == policyAdmin && user.Role != auth.RoleAdmin {
			writeError(w, 403, "admin required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyUser, user)))
	})
}

// statusRecorder captures status code so the log middleware can record it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// logMiddleware records every request into the in-memory ring buffer. Skips
// the SSE endpoint itself (long-lived; would spam the log every keepalive).
func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.logs == nil || r.URL.Path == "/api/realtime" {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		start := time.Now()
		next.ServeHTTP(rec, r)
		var user string
		if v := r.Context().Value(ctxKeyUser); v != nil {
			if u, ok := v.(auth.User); ok {
				user = u.Email
			}
		}
		s.logs.Append(logs.Entry{
			Method:     r.Method,
			Path:       r.URL.Path,
			Status:     rec.status,
			DurationMs: time.Since(start).Milliseconds(),
			User:       user,
			Remote:     r.RemoteAddr,
		})
	})
}

// routePolicy maps an incoming request to the required auth level.
// Defaults to authed so adding new endpoints fails closed.
func (s *Server) routePolicy(r *http.Request) policy {
	p := r.URL.Path
	m := r.Method

	switch p {
	case "/api/health", "/api/auth/login", "/api/auth/register":
		return policyPublic
	case "/api/realtime":
		return policyAuthed // valid user can subscribe to anything (v1)
	}

	// Collection schema mutations require admin; reads and record CRUD do not.
	if p == "/api/collections" {
		if m == http.MethodPost {
			return policyAdmin
		}
		return policyAuthed
	}
	if strings.HasPrefix(p, "/api/collections/") {
		rest := strings.TrimPrefix(p, "/api/collections/")
		parts := strings.SplitN(rest, "/", 3)
		// /api/collections/<name>          — admin for PATCH/DELETE, authed for GET
		// /api/collections/<name>/records* — authed
		if len(parts) == 1 {
			if m == http.MethodPatch || m == http.MethodDelete {
				return policyAdmin
			}
			return policyAuthed
		}
		return policyAuthed
	}

	// Raw KV: any write is admin; reads authed.
	if strings.HasPrefix(p, "/api/kv/") {
		if m == http.MethodGet {
			return policyAuthed
		}
		return policyAdmin
	}

	// Files: read/upload authed, delete admin.
	if p == "/api/files" || strings.HasPrefix(p, "/api/files/") {
		if m == http.MethodDelete {
			return policyAdmin
		}
		return policyAuthed
	}

	// Stats: admin (carries data dir paths).
	if p == "/api/stats" {
		return policyAdmin
	}

	// Logs: admin only (contains user emails + paths).
	if p == "/api/logs" {
		return policyAdmin
	}

	// /api/auth/me — any authed user.
	if p == "/api/auth/me" {
		return policyAuthed
	}

	return policyAuthed
}
