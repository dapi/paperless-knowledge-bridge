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
	Store         *bridge.Store
	Embedder      bridge.Embedder
	ConsumerToken string
	WebhookToken  string
	Sync          func(context.Context) (int, error)
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/v1/search", s.search)
	mux.HandleFunc("/v1/hooks/paperless", s.hook)
	return mux
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
	if !s.authorized(r, s.ConsumerToken) {
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
