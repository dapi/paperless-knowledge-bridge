package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/dapi/paperless-knowledge-bridge/internal/bridge"
)

type Server struct {
	Store          *bridge.Store
	Embedder       bridge.Embedder
	ConsumerTokens map[string]string
	WebhookToken   string
	Sync           func(context.Context) (int, error)
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/openapi.json", s.openAPI)
	mux.HandleFunc("/v1/search", s.search)
	mux.HandleFunc("/v1/hooks/paperless", s.hook)
	return mux
}

func (s Server) consumerAuthorized(r *http.Request) bool {
	consumer := r.Header.Get("X-Paperless-Knowledge-Consumer")
	token, found := s.ConsumerTokens[consumer]
	if !found {
		return false
	}
	return s.authorized(r, token)
}

func (s Server) authorized(r *http.Request, token string) bool {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func (s Server) search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.consumerAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&body); err != nil || strings.TrimSpace(body.Query) == "" {
		http.Error(w, "query required", http.StatusBadRequest)
		return
	}
	vectors, err := s.Embedder.Embed(r.Context(), []string{body.Query})
	if err != nil || len(vectors) != 1 {
		http.Error(w, "query embedding unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Results       []bridge.Result `json:"results"`
		RetrievalGate string          `json:"retrieval_gate"`
	}{Results: s.Store.Search(vectors[0], body.Limit), RetrievalGate: "explicit_consumer_tool_call"})
}

func (s Server) openAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	schema := map[string]any{
		"type":     "object",
		"required": []string{"query"},
		"properties": map[string]any{
			"query": map[string]string{"type": "string", "description": "Search question or terms."},
			"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": bridge.MaxResults, "default": bridge.MaxResults},
		},
	}
	operation := map[string]any{
		"operationId": "search_paperless_knowledge",
		"summary":     "Search personal Paperless knowledge",
		"description": "Use only when the user explicitly asks to search their Paperless archive. Returns at most five short excerpts and source URLs; it cannot access files or change documents.",
		"requestBody": map[string]any{
			"required": true,
			"content":  map[string]any{"application/json": map[string]any{"schema": schema}},
		},
		"responses": map[string]any{"200": map[string]string{"description": "Bounded search results"}},
	}
	document := map[string]any{
		"openapi":    "3.0.3",
		"info":       map[string]string{"title": "Paperless Knowledge Bridge", "version": "v1", "description": "Read-only bounded retrieval from the personal Paperless archive."},
		"paths":      map[string]any{"/v1/search": map[string]any{"post": operation}},
		"components": map[string]any{"securitySchemes": map[string]any{"bearerAuth": map[string]string{"type": "http", "scheme": "bearer"}}},
		"security":   []map[string][]string{{"bearerAuth": []string{}}},
	}
	_ = json.NewEncoder(w).Encode(document)
}

func (s Server) hook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r, s.WebhookToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 14*time.Minute)
		defer cancel()
		_, _ = s.Sync(ctx)
	}()
	w.WriteHeader(http.StatusAccepted)
}
