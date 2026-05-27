// Package files implements blob storage for SolderDB.
//
// Layout on disk:
//
//	<dataDir>/files/<id>           — the blob (raw bytes, no envelope)
//	<dataDir>/files/<id>.tmp       — staging file during upload (renamed on success)
//
// Metadata lives in the internal `_files` collection so blobs get all the
// engine benefits — snapshots, realtime events, listing. The blob itself
// is excluded from snapshots in this version; we treat the metadata as
// the index and the on-disk blob as the payload. (A future Snapshot()
// extension can copy the files dir too.)
package files

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"solderdb/internal/collections"
)

const (
	FilesCollection = "_files"
	MaxUploadSize   = 100 * 1024 * 1024 // 100 MB hard cap per upload (v1)
)

// FileMeta is the public view of a file.
type FileMeta struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	SHA256   string `json:"sha256"`
	Created  string `json:"created"`
}

type Service struct {
	colls *collections.Service
	dir   string
}

// New ensures the storage directory and the metadata collection exist.
func New(colls *collections.Service, dataDir string) (*Service, error) {
	dir := filepath.Join(dataDir, "files")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("files mkdir: %w", err)
	}
	s := &Service{colls: colls, dir: dir}
	if err := s.ensureCollection(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) ensureCollection() error {
	if _, err := s.colls.GetCollection(FilesCollection); err == nil {
		return nil
	}
	_, err := s.colls.CreateCollection(collections.CollectionMeta{
		Name: FilesCollection,
		Fields: []collections.Field{
			{Name: "name", Type: collections.FieldText, Required: true},
			{Name: "size", Type: collections.FieldNumber, Required: true},
			{Name: "mime_type", Type: collections.FieldText, Required: true},
			{Name: "sha256", Type: collections.FieldText, Required: true},
			{Name: "blob_id", Type: collections.FieldText, Required: true},
		},
	})
	return err
}

// Upload streams the request body into a temp file, hashes as it goes,
// then renames atomically and writes the metadata record.
func (s *Service) Upload(name, mimeType string, r io.Reader) (FileMeta, error) {
	if strings.TrimSpace(name) == "" {
		return FileMeta{}, errors.New("file name required")
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	blobID := newBlobID()
	tmpPath := filepath.Join(s.dir, blobID+".tmp")
	finalPath := filepath.Join(s.dir, blobID)

	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return FileMeta{}, fmt.Errorf("create temp: %w", err)
	}

	hasher := sha256.New()
	limited := io.LimitReader(r, MaxUploadSize+1)
	written, err := io.Copy(io.MultiWriter(out, hasher), limited)
	closeErr := out.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return FileMeta{}, fmt.Errorf("write blob: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return FileMeta{}, fmt.Errorf("close blob: %w", closeErr)
	}
	if written > MaxUploadSize {
		_ = os.Remove(tmpPath)
		return FileMeta{}, fmt.Errorf("file exceeds %d bytes", MaxUploadSize)
	}
	if written == 0 {
		_ = os.Remove(tmpPath)
		return FileMeta{}, errors.New("empty upload")
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return FileMeta{}, fmt.Errorf("finalize blob: %w", err)
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	rec, err := s.colls.Insert(FilesCollection, map[string]interface{}{
		"name":      filepath.Base(name),
		"size":      float64(written),
		"mime_type": mimeType,
		"sha256":    sum,
		"blob_id":   blobID,
	})
	if err != nil {
		_ = os.Remove(finalPath)
		return FileMeta{}, fmt.Errorf("write meta: %w", err)
	}
	return recToMeta(rec), nil
}

// Stream returns an open reader for the file's bytes plus its metadata.
// Caller must close the reader.
func (s *Service) Stream(id string) (FileMeta, io.ReadCloser, error) {
	meta, err := s.Meta(id)
	if err != nil {
		return FileMeta{}, nil, err
	}
	rec, err := s.colls.GetRecord(FilesCollection, id)
	if err != nil {
		return FileMeta{}, nil, err
	}
	blobID, _ := rec.Data["blob_id"].(string)
	f, err := os.Open(filepath.Join(s.dir, blobID))
	if err != nil {
		return FileMeta{}, nil, fmt.Errorf("open blob: %w", err)
	}
	return meta, f, nil
}

func (s *Service) Meta(id string) (FileMeta, error) {
	rec, err := s.colls.GetRecord(FilesCollection, id)
	if err != nil {
		return FileMeta{}, err
	}
	return recToMeta(rec), nil
}

func (s *Service) List(after string, limit int) ([]FileMeta, string, error) {
	if limit <= 0 {
		limit = 50
	}
	res, err := s.colls.ListRecords(FilesCollection, after, limit)
	if err != nil {
		return nil, "", err
	}
	out := make([]FileMeta, 0, len(res.Records))
	for _, r := range res.Records {
		out = append(out, recToMeta(r))
	}
	return out, res.NextAfter, nil
}

func (s *Service) Delete(id string) error {
	rec, err := s.colls.GetRecord(FilesCollection, id)
	if err != nil {
		return err
	}
	blobID, _ := rec.Data["blob_id"].(string)
	if err := s.colls.DeleteRecord(FilesCollection, id); err != nil {
		return err
	}
	// Best-effort blob removal — meta deletion is the source of truth.
	if blobID != "" {
		_ = os.Remove(filepath.Join(s.dir, blobID))
	}
	return nil
}

// ---------------- helpers ----------------

func recToMeta(rec collections.Record) FileMeta {
	name, _ := rec.Data["name"].(string)
	mime, _ := rec.Data["mime_type"].(string)
	sum, _ := rec.Data["sha256"].(string)
	var size int64
	if f, ok := rec.Data["size"].(float64); ok {
		size = int64(f)
	}
	return FileMeta{
		ID: rec.ID, Name: name, MimeType: mime, SHA256: sum, Size: size, Created: rec.Created,
	}
}

func newBlobID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%d_%s", time.Now().UnixNano(), hex.EncodeToString(b[:]))
}

// DetectMime sniffs the first 512 bytes of r and returns its MIME type
// using net/http's content-type detector. Returns the original reader
// with the sniffed bytes prepended so the caller can keep streaming.
func DetectMime(r io.Reader) (string, io.Reader, error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", nil, err
	}
	buf = buf[:n]
	mime := http.DetectContentType(buf)
	combined := io.MultiReader(strings.NewReader(string(buf)), r)
	return mime, combined, nil
}
