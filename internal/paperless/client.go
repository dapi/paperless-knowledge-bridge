package paperless

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dapi/paperless-knowledge-bridge/internal/bridge"
)

// Client uses only Paperless' read-only documents API.
type Client struct {
	BaseURL, Token string
	HTTP           *http.Client
}

type listResponse struct {
	Next    string `json:"next"`
	Results []struct {
		ID       int    `json:"id"`
		Title    string `json:"title"`
		Content  string `json:"content"`
		Modified string `json:"modified"`
	} `json:"results"`
}

func (c Client) ListDocuments(ctx context.Context, limit int) ([]bridge.Document, error) {
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	next := strings.TrimRight(c.BaseURL, "/") + "/api/documents/?page_size=100&ordering=id"
	var documents []bridge.Document
	for next != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Token "+c.Token)
		req.Header.Set("Accept", "application/json; version=10")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("paperless documents: %s", resp.Status)
		}
		var page listResponse
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, item := range page.Results {
			documents = append(documents, bridge.Document{ID: item.ID, Title: item.Title, Content: item.Content, Modified: item.Modified})
			if limit > 0 && len(documents) >= limit {
				return documents, nil
			}
		}
		if page.Next == "" {
			break
		}
		u, err := url.Parse(page.Next)
		if err != nil {
			return nil, err
		}
		if !u.IsAbs() {
			next = strings.TrimRight(c.BaseURL, "/") + "/" + strings.TrimLeft(page.Next, "/")
		} else {
			next = u.String()
		}
	}
	return documents, nil
}

func PilotLimit(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > 20 {
		return 0, fmt.Errorf("PILOT_LIMIT must be from 1 to 20")
	}
	return n, nil
}
