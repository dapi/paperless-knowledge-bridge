package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dapi/paperless-knowledge-bridge/internal/api"
	"github.com/dapi/paperless-knowledge-bridge/internal/bridge"
	"github.com/dapi/paperless-knowledge-bridge/internal/embed"
	"github.com/dapi/paperless-knowledge-bridge/internal/paperless"
)

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func main() {
	pilot, err := paperless.PilotLimit(os.Getenv("PILOT_LIMIT"))
	if err != nil {
		log.Fatal(err)
	}
	store, err := bridge.OpenStore(getenv("INDEX_PATH", "/data/index.json"))
	if err != nil {
		log.Fatal(err)
	}
	sourceURL := os.Getenv("PAPERLESS_URL")
	source := paperless.Client{BaseURL: sourceURL, Token: os.Getenv("PAPERLESS_API_TOKEN"), HTTP: &http.Client{Timeout: 60 * time.Second}}
	embedder := embed.Client{BaseURL: getenv("LITELLM_URL", "http://litellm.litellm.svc.cluster.local:4000/v1"), APIKey: os.Getenv("LITELLM_API_KEY"), Model: getenv("EMBEDDING_MODEL", "paperless-embedding"), HTTP: &http.Client{Timeout: 120 * time.Second}}
	syncer := bridge.Syncer{Source: source, Embedder: embedder, SourceURL: sourceURL}
	sync := func(ctx context.Context) (int, error) { return syncer.Reconcile(ctx, store, pilot) }
	if _, err := sync(context.Background()); err != nil {
		log.Printf("initial synchronization failed: %v", err)
	}
	interval, err := time.ParseDuration(getenv("RECONCILE_INTERVAL", "15m"))
	if err != nil || interval <= 0 {
		log.Fatal("RECONCILE_INTERVAL must be a positive duration")
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), interval)
			if _, err := sync(ctx); err != nil {
				log.Printf("scheduled reconciliation failed: %v", err)
			}
			cancel()
		}
	}()
	server := api.Server{Store: store, Embedder: embedder, ConsumerTokens: map[string]string{
		"open-webui": os.Getenv("BRIDGE_OPENWEBUI_TOKEN"),
		"codex":      os.Getenv("BRIDGE_CODEX_TOKEN"),
		"hermes":     os.Getenv("BRIDGE_HERMES_TOKEN"),
	}, WebhookToken: os.Getenv("PAPERLESS_WEBHOOK_TOKEN"), Sync: sync}
	log.Printf("paperless knowledge bridge listening on %s (pilot_limit=%d)", getenv("LISTEN_ADDR", ":8080"), pilot)
	log.Fatal(http.ListenAndServe(getenv("LISTEN_ADDR", ":8080"), server.Handler()))
}
