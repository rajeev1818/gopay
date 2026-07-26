package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type WebhookEvent struct {
	EventType string      `json:"event_type"`
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
}

type WebhookDispatcher struct {
	queue      chan WebhookEvent
	signingKey []byte
	client     *http.Client
	endpoints  []string
}

func NewWebookDispatcher(signingKey []byte, workers int) *WebhookDispatcher {
	d := &WebhookDispatcher{
		queue:      make(chan WebhookEvent, 1000),
		signingKey: signingKey,
		client:     &http.Client{Timeout: 10 * time.Second},
	}

	for i := 0; i < workers; i++ {
		go d.worker(i)
	}
	return d
}

func (d *WebhookDispatcher) worker(id int) {
	for event := range d.queue { // range over channel — blocks when empty, exits when closed
		for _, endpoint := range d.endpoints {
			d.deliver(event, endpoint, 3) // max 3 retries
		}
	}
}

func (d *WebhookDispatcher) deliver(event WebhookEvent, url string, maxRetries int) {
	body, _ := json.Marshal(event)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
		mac := hmac.New(sha256.New, d.signingKey)
		mac.Write(body)
		signature := hex.EncodeToString(mac.Sum(nil))

		req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Signature", signature)
		req.Header.Set("X-Webhook-Timestamp", event.Timestamp.Format(time.RFC3339))

		resp, err := d.client.Do(req)

		if err != nil {
			slog.Warn("webhook delivery failed", "url", url, "attempt", attempt, "error", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return
		}
		slog.Warn("webhook non-2xx", "url", url, "status", resp.StatusCode, "attempt", attempt)
	}
	slog.Error("webhook delivery exhausted retries", "url", url, "event", event.EventType)

}

func (d *WebhookDispatcher) Emit(ctx context.Context, eventType string, payload interface{}) {
	select {
	case d.queue <- WebhookEvent{
		EventType: eventType,
		Payload:   payload,
		Timestamp: time.Now(),
	}:
	default:
		slog.Warn("webhook queue full, dropping event", "type", eventType)
	}
}
