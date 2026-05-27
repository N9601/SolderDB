// Package solderdb is a Go client library for SolderDB.
//
// Quick start:
//
//	c := solderdb.New("http://localhost:8787")
//	sess, _ := c.Auth.Login(ctx, "you@example.com", "secret")
//	_ = sess
//
//	notes := c.Collection("notes")
//	doc, _ := notes.Create(ctx, map[string]any{"title": "hello"})
//	list, _ := notes.List(ctx, solderdb.ListOptions{Limit: 50})
//
//	stop, _ := notes.Subscribe(ctx, func(evt solderdb.Event) {
//	    fmt.Println(evt.Kind, evt.ID)
//	})
//	defer stop()
//
// Stdlib-only; no third-party deps.
package solderdb

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// ---------------- Types ----------------

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

type User struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Role    Role   `json:"role"`
	Created string `json:"created"`
	Updated string `json:"updated"`
}

type Session struct {
	Token   string `json:"token"`
	User    User   `json:"user"`
	Expires string `json:"expires"`
}

type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Unique   bool   `json:"unique,omitempty"`
}

type CollectionMeta struct {
	Name    string  `json:"name"`
	Fields  []Field `json:"fields"`
	Created string  `json:"created,omitempty"`
	Updated string  `json:"updated,omitempty"`
}

type Document struct {
	ID      string                 `json:"id"`
	Created string                 `json:"created"`
	Updated string                 `json:"updated"`
	Data    map[string]interface{} `json:"data"`
}

type ListResult struct {
	Records   []Document `json:"records"`
	NextAfter string     `json:"nextAfter"`
}

type ListOptions struct {
	After string
	Limit int
}

type FileMeta struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	SHA256   string `json:"sha256"`
	Created  string `json:"created"`
}

type Event struct {
	Kind       string                 `json:"kind"`
	Collection string                 `json:"collection,omitempty"`
	ID         string                 `json:"id,omitempty"`
	Key        string                 `json:"key,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Timestamp  string                 `json:"timestamp"`
}

// APIError is returned when the server responds with a non-2xx status.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("solderdb: %d %s", e.Status, e.Message) }

// ---------------- Client ----------------

type Client struct {
	BaseURL string
	HTTP    *http.Client

	mu    sync.RWMutex
	token string

	Auth   *AuthAPI
	Admin  *AdminAPI
	Files  *FilesAPI
}

func New(baseURL string) *Client {
	c := &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    http.DefaultClient,
	}
	c.Auth = &AuthAPI{c: c}
	c.Admin = &AdminAPI{c: c}
	c.Files = &FilesAPI{c: c}
	return c
}

func (c *Client) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

func (c *Client) SetToken(t string) {
	c.mu.Lock()
	c.token = t
	c.mu.Unlock()
}

// Collection returns a typed handle for one collection.
func (c *Client) Collection(name string) *CollectionAPI {
	return &CollectionAPI{c: c, Name: name}
}

// URL returns the absolute URL for an API path; pass auth=true to include
// ?token= (required for EventSource / file links).
func (c *Client) URL(path string, auth bool) string {
	u := c.BaseURL + path
	if !auth {
		return u
	}
	tok := c.Token()
	if tok == "" {
		return u
	}
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	return u + sep + "token=" + url.QueryEscape(tok)
}

// do is the low-level request helper. body may be nil, a []byte, or any
// JSON-marshallable value. out, when non-nil, receives the decoded response.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	var contentType string

	switch v := body.(type) {
	case nil:
		// no body
	case io.Reader:
		rdr = v
	case []byte:
		rdr = bytes.NewReader(v)
	default:
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
		rdr = bytes.NewReader(b)
		contentType = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if tok := c.Token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := ""
		var errObj struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errObj) == nil {
			msg = errObj.Error
		}
		if msg == "" {
			msg = strings.TrimSpace(string(respBody))
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// ---------------- Auth ----------------

type AuthAPI struct{ c *Client }

func (a *AuthAPI) Register(ctx context.Context, email, password string) (Session, error) {
	var s Session
	err := a.c.do(ctx, http.MethodPost, "/api/auth/register", map[string]string{
		"email": email, "password": password,
	}, &s)
	if err == nil {
		a.c.SetToken(s.Token)
	}
	return s, err
}

func (a *AuthAPI) Login(ctx context.Context, email, password string) (Session, error) {
	var s Session
	err := a.c.do(ctx, http.MethodPost, "/api/auth/login", map[string]string{
		"email": email, "password": password,
	}, &s)
	if err == nil {
		a.c.SetToken(s.Token)
	}
	return s, err
}

func (a *AuthAPI) Me(ctx context.Context) (User, error) {
	var u User
	return u, a.c.do(ctx, http.MethodGet, "/api/auth/me", nil, &u)
}

func (a *AuthAPI) Logout() { a.c.SetToken("") }

// ---------------- Collections ----------------

type CollectionAPI struct {
	c    *Client
	Name string
}

func (col *CollectionAPI) List(ctx context.Context, opts ListOptions) (ListResult, error) {
	q := url.Values{}
	if opts.After != "" {
		q.Set("after", opts.After)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	path := "/api/collections/" + url.PathEscape(col.Name) + "/records"
	if s := q.Encode(); s != "" {
		path += "?" + s
	}
	var out ListResult
	return out, col.c.do(ctx, http.MethodGet, path, nil, &out)
}

func (col *CollectionAPI) Get(ctx context.Context, id string) (Document, error) {
	var d Document
	return d, col.c.do(ctx, http.MethodGet, "/api/collections/"+url.PathEscape(col.Name)+"/records/"+url.PathEscape(id), nil, &d)
}

func (col *CollectionAPI) Create(ctx context.Context, data map[string]any) (Document, error) {
	var d Document
	return d, col.c.do(ctx, http.MethodPost, "/api/collections/"+url.PathEscape(col.Name)+"/records", data, &d)
}

func (col *CollectionAPI) Update(ctx context.Context, id string, patch map[string]any) (Document, error) {
	var d Document
	return d, col.c.do(ctx, http.MethodPatch, "/api/collections/"+url.PathEscape(col.Name)+"/records/"+url.PathEscape(id), patch, &d)
}

func (col *CollectionAPI) Delete(ctx context.Context, id string) error {
	return col.c.do(ctx, http.MethodDelete, "/api/collections/"+url.PathEscape(col.Name)+"/records/"+url.PathEscape(id), nil, nil)
}

// Subscribe streams realtime events for this collection until ctx is
// cancelled or the connection closes. handler runs synchronously per event.
// Returns a stop function that cancels the subscription.
func (col *CollectionAPI) Subscribe(ctx context.Context, handler func(Event)) (func(), error) {
	subCtx, cancel := context.WithCancel(ctx)
	go col.streamLoop(subCtx, "coll:"+col.Name, handler)
	return cancel, nil
}

func (col *CollectionAPI) streamLoop(ctx context.Context, topic string, handler func(Event)) {
	u := col.c.URL("/api/realtime?topic="+url.QueryEscape(topic), true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return
	}
	if tok := col.c.Token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := col.c.HTTP.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()

	br := bufio.NewReader(resp.Body)
	var data strings.Builder
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if data.Len() > 0 {
				var evt Event
				if json.Unmarshal([]byte(data.String()), &evt) == nil {
					handler(evt)
				}
				data.Reset()
			}
		case strings.HasPrefix(line, "data: "):
			data.WriteString(strings.TrimPrefix(line, "data: "))
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// ---------------- Admin ----------------

type AdminAPI struct{ c *Client }

func (a *AdminAPI) ListCollections(ctx context.Context) ([]CollectionMeta, error) {
	var out []CollectionMeta
	return out, a.c.do(ctx, http.MethodGet, "/api/collections", nil, &out)
}

func (a *AdminAPI) CreateCollection(ctx context.Context, meta CollectionMeta) (CollectionMeta, error) {
	var out CollectionMeta
	return out, a.c.do(ctx, http.MethodPost, "/api/collections", meta, &out)
}

func (a *AdminAPI) UpdateCollection(ctx context.Context, name string, fields []Field) (CollectionMeta, error) {
	var out CollectionMeta
	body := map[string]any{"fields": fields}
	return out, a.c.do(ctx, http.MethodPatch, "/api/collections/"+url.PathEscape(name), body, &out)
}

func (a *AdminAPI) DeleteCollection(ctx context.Context, name string) error {
	return a.c.do(ctx, http.MethodDelete, "/api/collections/"+url.PathEscape(name), nil, nil)
}

func (a *AdminAPI) Stats(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	return out, a.c.do(ctx, http.MethodGet, "/api/stats", nil, &out)
}

// ---------------- Files ----------------

type FilesAPI struct{ c *Client }

type fileListResult struct {
	Files     []FileMeta `json:"files"`
	NextAfter string     `json:"nextAfter"`
}

func (f *FilesAPI) List(ctx context.Context, opts ListOptions) ([]FileMeta, string, error) {
	q := url.Values{}
	if opts.After != "" {
		q.Set("after", opts.After)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	path := "/api/files"
	if s := q.Encode(); s != "" {
		path += "?" + s
	}
	var out fileListResult
	if err := f.c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, "", err
	}
	return out.Files, out.NextAfter, nil
}

// Upload streams r to the server as a multipart upload.
func (f *FilesAPI) Upload(ctx context.Context, name string, r io.Reader, mimeType string) (FileMeta, error) {
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)

	hdr := make(map[string][]string)
	hdr["Content-Disposition"] = []string{`form-data; name="file"; filename="` + name + `"`}
	if mimeType != "" {
		hdr["Content-Type"] = []string{mimeType}
	}
	part, err := mw.CreatePart(hdr)
	if err != nil {
		return FileMeta{}, fmt.Errorf("multipart part: %w", err)
	}
	if _, err := io.Copy(part, r); err != nil {
		return FileMeta{}, fmt.Errorf("multipart copy: %w", err)
	}
	if err := mw.Close(); err != nil {
		return FileMeta{}, fmt.Errorf("multipart close: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.c.BaseURL+"/api/files", buf)
	if err != nil {
		return FileMeta{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if tok := f.c.Token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := f.c.HTTP.Do(req)
	if err != nil {
		return FileMeta{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errObj struct{ Error string }
		_ = json.Unmarshal(body, &errObj)
		if errObj.Error == "" {
			errObj.Error = string(body)
		}
		return FileMeta{}, &APIError{Status: resp.StatusCode, Message: errObj.Error}
	}
	var meta FileMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return FileMeta{}, fmt.Errorf("decode meta: %w", err)
	}
	return meta, nil
}

func (f *FilesAPI) Delete(ctx context.Context, id string) error {
	return f.c.do(ctx, http.MethodDelete, "/api/files/"+url.PathEscape(id), nil, nil)
}

// URL returns a full file URL with the auth token in the query so it works
// in browsers (<img src=...>).
func (f *FilesAPI) URL(id string) string {
	return f.c.URL("/api/files/"+url.PathEscape(id), true)
}

// Compile-time assertion that nothing inadvertently changes the package surface.
var _ error = (*APIError)(nil)
var _ = errors.New
