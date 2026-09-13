package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Client talks to LiteLLM's OpenAI-compatible embeddings endpoint. The model
// alias is intentionally dedicated to Paperless and is not OpenViking's alias.
type Client struct {
	BaseURL, APIKey, Model string
	HTTP                   *http.Client
}
type request struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}
type response struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (c Client) Embed(ctx context.Context, input []string) ([][]float64, error) {
	b, err := json.Marshal(request{Model: c.Model, Input: input})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/embeddings", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embeddings: %s", resp.Status)
	}
	var out response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	vectors := make([][]float64, len(out.Data))
	for i := range out.Data {
		vectors[i] = out.Data[i].Embedding
	}
	return vectors, nil
}
