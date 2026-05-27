package bridge

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// DocsService serves the markdown files embedded at compile time so the
// admin GUI can render them inline. Source lives in /docs/*.md at the repo
// root.
type DocsService struct {
	fs embed.FS
}

func NewDocsService(docs embed.FS) *DocsService {
	return &DocsService{fs: docs}
}

// DocMeta is a slug + title pair for the sidebar.
type DocMeta struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Order int    `json:"order"`
}

// List returns the documentation table of contents, ordered by the front-matter
// `order:` field (then alphabetically by slug).
func (s *DocsService) List() ([]DocMeta, error) {
	var out []DocMeta
	err := fs.WalkDir(s.fs, "docs", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		body, err := s.fs.ReadFile(path)
		if err != nil {
			return err
		}
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
		title, order := parseFrontMatter(body, slug)
		out = append(out, DocMeta{Slug: slug, Title: title, Order: order})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Slug < out[j].Slug
	})
	return out, nil
}

// Read returns the raw markdown body for a slug (without the front-matter).
func (s *DocsService) Read(slug string) (string, error) {
	if strings.ContainsAny(slug, "/\\.") {
		return "", fmt.Errorf("invalid slug")
	}
	body, err := s.fs.ReadFile("docs/" + slug + ".md")
	if err != nil {
		return "", fmt.Errorf("doc not found: %s", slug)
	}
	return stripFrontMatter(body), nil
}

// parseFrontMatter extracts `title:` and `order:` from a YAML-ish front matter.
// Falls back to the slug as title and 999 as order if absent.
func parseFrontMatter(body []byte, fallbackSlug string) (title string, order int) {
	title = fallbackSlug
	order = 999
	s := string(body)
	if !strings.HasPrefix(s, "---") {
		return
	}
	rest := s[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return
	}
	for _, line := range strings.Split(rest[:end], "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "title:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
		}
		if strings.HasPrefix(line, "order:") {
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(line, "order:")), "%d", &n); err == nil {
				order = n
			}
		}
	}
	return
}

func stripFrontMatter(body []byte) string {
	s := string(body)
	if !strings.HasPrefix(s, "---") {
		return s
	}
	rest := s[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return s
	}
	return strings.TrimLeft(rest[end+4:], "\n")
}
