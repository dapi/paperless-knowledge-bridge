package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dapi/paperless-knowledge-bridge/internal/bridge"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Embed(context.Context, []string) ([][]float64, error) {
	return [][]float64{{1, 0}}, nil
}

func TestSearchRequiresKnownConsumerIdentity(t *testing.T) {
	store, _ := bridge.OpenStore("")
	if err := store.ReplaceDocument(1, []bridge.Chunk{{DocumentID: 1, Embedding: []float64{1, 0}}}); err != nil {
		t.Fatal(err)
	}
	server := Server{Store: store, Embedder: fakeEmbedder{}, ConsumerTokens: map[string]string{"open-webui": "test-token"}}
	request := httptest.NewRequest(http.MethodPost, "/v1/search", strings.NewReader(`{"query":"test"}`))
	request.Header.Set("Authorization", "Bearer test-token")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want %d", response.Code, http.StatusUnauthorized)
	}
	request.Header.Set("X-Paperless-Knowledge-Consumer", "open-webui")
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("got %d, want %d", response.Code, http.StatusOK)
	}
}

func TestOpenAPIDescribesExplicitSearchTool(t *testing.T) {
	server := Server{}
	request := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "search_paperless_knowledge") {
		t.Fatalf("unexpected OpenAPI response: status=%d body=%s", response.Code, response.Body.String())
	}
}
