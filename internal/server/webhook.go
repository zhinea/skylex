package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type WebhookEvent string

const (
	WebhookEventBackupFailed    WebhookEvent = "backup.failed"
	WebhookEventBackupCompleted WebhookEvent = "backup.completed"
	WebhookEventFailover        WebhookEvent = "cluster.failover"
	WebhookEventNodeOffline     WebhookEvent = "node.offline"
	WebhookEventNodeOnline      WebhookEvent = "node.online"
)

type WebhookPayload struct {
	Event     WebhookEvent `json:"event"`
	Timestamp time.Time    `json:"timestamp"`
	Data      interface{}  `json:"data"`
}

type WebhookClient struct {
	urls   []string
	client *http.Client
	log    *slog.Logger
}

func NewWebhookClient(urls []string, timeout time.Duration, log *slog.Logger) *WebhookClient {
	if len(urls) == 0 {
		return nil
	}

	return &WebhookClient{
		urls: urls,
		client: &http.Client{
			Timeout: timeout,
		},
		log: log,
	}
}

func (w *WebhookClient) Send(ctx context.Context, event WebhookEvent, data interface{}) {
	if w == nil {
		return
	}

	payload := WebhookPayload{
		Event:     event,
		Timestamp: time.Now(),
		Data:      data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		w.log.Error("webhook marshal failed", "event", event, "error", err)
		return
	}

	for _, url := range w.urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			w.log.Error("webhook request creation failed", "url", url, "event", event, "error", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "skylex-webhook/1.0")

		resp, err := w.client.Do(req)
		if err != nil {
			w.log.Warn("webhook delivery failed", "url", url, "event", event, "error", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			w.log.Warn("webhook returned non-2xx", "url", url, "event", event, "status", resp.StatusCode)
		}
	}

	w.log.Info("webhook sent", "event", event, "urls", len(w.urls))
}

func (w *WebhookClient) NotifyBackupFailed(ctx context.Context, clusterID, backupID, errMsg string) {
	w.Send(ctx, WebhookEventBackupFailed, map[string]string{
		"cluster_id": clusterID,
		"backup_id":  backupID,
		"error":      errMsg,
	})
}

func (w *WebhookClient) NotifyBackupCompleted(ctx context.Context, clusterID, backupID string) {
	w.Send(ctx, WebhookEventBackupCompleted, map[string]string{
		"cluster_id": clusterID,
		"backup_id":  backupID,
	})
}

func (w *WebhookClient) NotifyFailover(ctx context.Context, clusterID, oldPrimary, newPrimary string) {
	w.Send(ctx, WebhookEventFailover, map[string]string{
		"cluster_id":  clusterID,
		"old_primary": oldPrimary,
		"new_primary": newPrimary,
	})
}

func (w *WebhookClient) NotifyNodeOffline(ctx context.Context, nodeID string) {
	w.Send(ctx, WebhookEventNodeOffline, map[string]string{
		"node_id": nodeID,
	})
}

func (w *WebhookClient) NotifyNodeOnline(ctx context.Context, nodeID string) {
	w.Send(ctx, WebhookEventNodeOnline, map[string]string{
		"node_id": nodeID,
	})
}

type WebhookConfig struct {
	URLs    []string      `koanf:"urls"`
	Timeout time.Duration `koanf:"timeout"`
}

func (c *WebhookConfig) setDefaults() {
	if c.Timeout == 0 {
		c.Timeout = 10 * time.Second
	}
}

func DefaultWebhookConfig() WebhookConfig {
	return WebhookConfig{
		Timeout: 10 * time.Second,
	}
}

func (c *WebhookConfig) Enabled() bool {
	return len(c.URLs) > 0
}

func validateWebhookURL(url string) error {
	if url == "" {
		return fmt.Errorf("webhook URL cannot be empty")
	}
	return nil
}