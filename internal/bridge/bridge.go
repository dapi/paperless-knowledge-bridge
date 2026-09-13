// Package bridge implements the private, read-only Paperless retrieval boundary.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	MaxResults      = 5
	MaxExcerptRunes = 900
)

type Document struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Modified string `json:"modified"`
}

type Chunk struct {
	DocumentID int       `json:"document_id"`
	Title      string    `json:"title"`
	Text       string    `json:"text"`
	SourceURL  string    `json:"source_url"`
	Hash       string    `json:"hash"`
	Embedding  []float64 `json:"embedding"`
}

type Result struct {
	DocumentID int     `json:"document_id"`
	Title      string  `json:"title"`
	Excerpt    string  `json:"excerpt"`
	SourceURL  string  `json:"source_url"`
	Score      float64 `json:"score"`
}

type Index struct {
	Chunks map[string]Chunk `json:"chunks"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	data Index
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, data: Index{Chunks: map[string]Chunk{}}}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}
	if s.data.Chunks == nil {
		s.data.Chunks = map[string]Chunk{}
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func key(id, part int) string { return fmt.Sprintf("%d:%d", id, part) }

func (s *Store) ReplaceDocument(id int, chunks []Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.data.Chunks {
		if v.DocumentID == id {
			delete(s.data.Chunks, k)
		}
	}
	for part, chunk := range chunks {
		s.data.Chunks[key(id, part)] = chunk
	}
	return s.saveLocked()
}

func (s *Store) DeleteMissing(known map[int]bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.data.Chunks {
		if !known[v.DocumentID] {
			delete(s.data.Chunks, k)
		}
	}
	return s.saveLocked()
}

func (s *Store) Search(query []float64, limit int) []Result {
	if limit < 1 {
		return nil
	}
	if limit > MaxResults {
		limit = MaxResults
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	results := make([]Result, 0, len(s.data.Chunks))
	for _, c := range s.data.Chunks {
		results = append(results, Result{DocumentID: c.DocumentID, Title: c.Title, Excerpt: excerpt(c.Text), SourceURL: c.SourceURL, Score: cosine(query, c.Embedding)})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	// A document may have many chunks; retain only its best matching excerpt.
	unique := results[:0]
	seen := map[int]bool{}
	for _, r := range results {
		if !seen[r.DocumentID] {
			unique = append(unique, r)
			seen[r.DocumentID] = true
			if len(unique) == limit {
				break
			}
		}
	}
	return unique
}

func excerpt(value string) string {
	r := []rune(strings.TrimSpace(value))
	if len(r) <= MaxExcerptRunes {
		return string(r)
	}
	return string(r[:MaxExcerptRunes]) + "…"
}

func cosine(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return -1
	}
	var dot, aa, bb float64
	for i := range a {
		dot += a[i] * b[i]
		aa += a[i] * a[i]
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return -1
	}
	return dot / math.Sqrt(aa*bb)
}

func Split(content string, size, overlap int) []string {
	r := []rune(strings.TrimSpace(content))
	if len(r) == 0 {
		return nil
	}
	if size <= 0 {
		size = 1800
	}
	if overlap < 0 || overlap >= size {
		overlap = 150
	}
	var out []string
	for start := 0; start < len(r); {
		end := start + size
		if end > len(r) {
			end = len(r)
		}
		out = append(out, string(r[start:end]))
		if end == len(r) {
			break
		}
		start = end - overlap
	}
	return out
}

func Hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type Source interface {
	ListDocuments(context.Context, int) ([]Document, error)
}
type Embedder interface {
	Embed(context.Context, []string) ([][]float64, error)
}

type Syncer struct {
	Source    Source
	Embedder  Embedder
	SourceURL string
}

func (s Syncer) Reconcile(ctx context.Context, store *Store, limit int) (int, error) {
	docs, err := s.Source.ListDocuments(ctx, limit)
	if err != nil {
		return 0, err
	}
	known := make(map[int]bool, len(docs))
	changed := 0
	for _, doc := range docs {
		known[doc.ID] = true
		parts := Split(doc.Content, 1800, 150)
		if len(parts) == 0 {
			continue
		}
		vectors, err := s.Embedder.Embed(ctx, parts)
		if err != nil {
			return changed, fmt.Errorf("embed document %d: %w", doc.ID, err)
		}
		if len(vectors) != len(parts) {
			return changed, fmt.Errorf("embed document %d: unexpected vector count", doc.ID)
		}
		chunks := make([]Chunk, len(parts))
		for i := range parts {
			chunks[i] = Chunk{DocumentID: doc.ID, Title: doc.Title, Text: parts[i], SourceURL: fmt.Sprintf("%s/documents/%d/details/", strings.TrimRight(s.SourceURL, "/"), doc.ID), Hash: Hash(parts[i]), Embedding: vectors[i]}
		}
		if err := store.ReplaceDocument(doc.ID, chunks); err != nil {
			return changed, err
		}
		changed++
	}
	// Only delete after a successful complete listing. A limited pilot is never a complete snapshot.
	if limit == 0 {
		if err := store.DeleteMissing(known); err != nil {
			return changed, err
		}
	}
	return changed, nil
}
