package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookPostsTheTextAsJSON(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
	}))
	defer srv.Close()
	Webhook{URL: srv.URL}.Send(context.Background(), "New lead: Asha")
	if got["text"] != "New lead: Asha" {
		t.Fatalf("got %v", got)
	}
}

func TestAnUnreachableWebhookIsSilent(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	Webhook{URL: url}.Send(context.Background(), "x") // must not panic or block
}
