// Package notify tells the sales team when something happens. It is best effort: a failure is logged and never reaches
// the visitor.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Notifier sends a one-line message.
type Notifier interface {
	Send(ctx context.Context, text string)
}

// Nop sends nothing.
type Nop struct{}

// Send implements Notifier.
func (Nop) Send(context.Context, string) {}

// Webhook posts {"text": "..."} to a URL; that is the format Slack, Mattermost and Discord (with /slack) accept.
type Webhook struct {
	URL  string
	Log  *slog.Logger
	HTTP *http.Client
}

// Send implements Notifier. It never blocks the caller for long and never returns an error.
func (w Webhook) Send(ctx context.Context, text string) {
	client := w.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	body, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		if w.Log != nil {
			w.Log.Warn("notification not sent")
		}
		return
	}
	_ = resp.Body.Close()
}
