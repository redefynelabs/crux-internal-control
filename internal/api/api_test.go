package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/config"
	"github.com/redefynelabs/erp-control-plane/internal/demo"
	"github.com/redefynelabs/erp-control-plane/internal/mail"
	"github.com/redefynelabs/erp-control-plane/internal/provision"
	"github.com/redefynelabs/erp-control-plane/internal/sales"
	"github.com/redefynelabs/erp-control-plane/internal/store"
	"github.com/redefynelabs/erp-control-plane/internal/testutil"
)

type okDBs struct{}

func (okDBs) Clone(context.Context, string, string, string) error          { return nil }
func (okDBs) Drop(context.Context, string) error                           { return nil }
func (okDBs) SetModules(context.Context, string, []string, []string) error { return nil }

type okStacks struct{}

func (okStacks) Start(context.Context, provision.StackSpec) error       { return nil }
func (okStacks) WaitReady(context.Context, string, time.Duration) error { return nil }
func (okStacks) Stop(context.Context, string) error                     { return nil }
func (okStacks) List(context.Context) ([]string, error)                 { return nil, nil }

type fixedCaptcha struct{ ok bool }

func (c fixedCaptcha) Verify(context.Context, string, string) (bool, error) { return c.ok, nil }

// token is the session of an admin created for each test server.
var token string

func newServer(t *testing.T, captcha Captcha) http.Handler {
	t.Helper()
	cfg := &config.Config{AllowedOrigins: []string{"https://site.example"}, TrustProxy: true,
		Demo: config.Demo{TTL: time.Hour, MaxLive: 10, MaxPerEmail: 1, MaxPerIP: 10, Domain: "demo.example.com", Scheme: "https"}}
	st := testutil.NewStore(t)
	token = adminSession(t, st)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mailer := mail.New(st, "test-secret", log)
	return New(cfg, st, demo.New(cfg.Demo, st, okDBs{}, okStacks{}, log), mailer, sales.New(st, mailer, log), captcha, log)
}

func do(h http.Handler, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func bearer() []string { return []string{"Authorization", "Bearer " + token} }

func data(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out struct {
		Data  any `json:"data"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("not JSON: %s", w.Body)
	}
	m, _ := out.Data.(map[string]any)
	return m
}

const good = `{"name":"Asha Rao","phone":"+91 98765 43210","email":"asha@rao.example","company":"Rao Traders"}`

func TestADemoRequestReturnsWhereToOpenIt(t *testing.T) {
	h := newServer(t, nil)
	w := do(h, "POST", "/v1/demo-requests", good)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	d := data(t, w)
	if !strings.HasSuffix(d["url"].(string), ".demo.example.com") || d["login_email"] != "asha@rao.example" || len(d["login_password"].(string)) != 14 || d["expires_at"] == nil {
		t.Fatalf("unexpected body: %v", d)
	}
}

func TestBadRequestsGetClearAnswers(t *testing.T) {
	h := newServer(t, nil)
	if w := do(h, "POST", "/v1/demo-requests", `{"name":"","email":"nope"}`); w.Code != 422 || !strings.Contains(w.Body.String(), `"email"`) {
		t.Fatalf("validation: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/v1/demo-requests", `{not json`); w.Code != 400 {
		t.Fatalf("bad JSON: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/demo-requests", `{"name":"A","phone":"+91 98765 43210","email":"a@b.example","admin":true}`); w.Code != 400 {
		t.Fatalf("unknown fields are refused: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/demo-requests", `{"name":"`+strings.Repeat("x", 20000)+`"}`); w.Code != 400 {
		t.Fatalf("an oversize body is refused: %d", w.Code)
	}
}

func TestAHoneypotFilledInStartsNothing(t *testing.T) {
	h := newServer(t, nil)
	w := do(h, "POST", "/v1/demo-requests", `{"name":"Bot","email":"bot@x.example","website":"http://spam"}`)
	if w.Code != 202 {
		t.Fatalf("a bot is told it worked: %d", w.Code)
	}
	if w := do(h, "GET", "/v1/demos", "", bearer()...); !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatalf("no demo may exist: %s", w.Body)
	}
}

func TestTheSameEmailIsLimited(t *testing.T) {
	h := newServer(t, nil)
	if w := do(h, "POST", "/v1/demo-requests", good); w.Code != 201 {
		t.Fatal(w.Body)
	}
	if w := do(h, "POST", "/v1/demo-requests", good); w.Code != 429 {
		t.Fatalf("want 429, got %d %s", w.Code, w.Body)
	}
}

func TestCaptchaMustPassWhenConfigured(t *testing.T) {
	if w := do(newServer(t, fixedCaptcha{false}), "POST", "/v1/demo-requests", good); w.Code != 422 {
		t.Fatalf("a failed captcha: %d", w.Code)
	}
	if w := do(newServer(t, fixedCaptcha{true}), "POST", "/v1/demo-requests", good); w.Code != 201 {
		t.Fatalf("a passed captcha: %d", w.Code)
	}
}

func TestTheBackOfficeNeedsTheToken(t *testing.T) {
	h := newServer(t, nil)
	for _, p := range []string{"/v1/leads", "/v1/demos", "/v1/stats"} {
		if w := do(h, "GET", p, ""); w.Code != 401 {
			t.Errorf("GET %s without a token = %d", p, w.Code)
		}
		if w := do(h, "GET", p, "", "Authorization", "Bearer wrong"); w.Code != 401 {
			t.Errorf("GET %s with a wrong token = %d", p, w.Code)
		}
		if w := do(h, "GET", p, "", bearer()...); w.Code != 200 {
			t.Errorf("GET %s with the token = %d %s", p, w.Code, w.Body)
		}
	}
}

func TestLeadsCanBeWorkedAndTheirHistoryIsKept(t *testing.T) {
	h := newServer(t, nil)
	do(h, "POST", "/v1/demo-requests", good)
	var list struct{ Data []map[string]any }
	_ = json.Unmarshal(do(h, "GET", "/v1/leads", "", bearer()...).Body.Bytes(), &list)
	if len(list.Data) != 1 || list.Data[0]["status"] != "demo_active" {
		t.Fatalf("leads: %+v", list.Data)
	}
	id := list.Data[0]["id"].(string)
	if w := do(h, "PATCH", "/v1/leads/"+id, `{"status":"qualified","notes":"wants finance + CRM"}`, bearer()...); w.Code != 200 {
		t.Fatalf("patch: %d %s", w.Code, w.Body)
	}
	if w := do(h, "PATCH", "/v1/leads/"+id, `{"status":"maybe"}`, bearer()...); w.Code != 422 {
		t.Fatalf("unknown status: %d", w.Code)
	}
	w := do(h, "GET", "/v1/leads/"+id, "", bearer()...)
	if !strings.Contains(w.Body.String(), "status_changed") || !strings.Contains(w.Body.String(), "demo_started") {
		t.Fatalf("history should show both events: %s", w.Body)
	}
	if w := do(h, "GET", "/v1/leads/not-a-uuid", "", bearer()...); w.Code != 404 {
		t.Fatalf("a bad id is a 404: %d", w.Code)
	}
	if w := do(h, "GET", "/v1/leads?status=bogus", "", bearer()...); w.Code != 400 {
		t.Fatalf("unknown filter: %d", w.Code)
	}
}

func TestDemosCanBeExtendedAndEnded(t *testing.T) {
	h := newServer(t, nil)
	do(h, "POST", "/v1/demo-requests", good)
	var list struct{ Data []map[string]any }
	_ = json.Unmarshal(do(h, "GET", "/v1/demos?status=active", "", bearer()...).Body.Bytes(), &list)
	id := list.Data[0]["id"].(string)
	if w := do(h, "POST", "/v1/demos/"+id+"/extend", `{"minutes":30}`, bearer()...); w.Code != 200 {
		t.Fatalf("extend: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/v1/demos/"+id+"/extend", `{"minutes":0}`, bearer()...); w.Code != 422 {
		t.Fatalf("zero minutes: %d", w.Code)
	}
	if w := do(h, "DELETE", "/v1/demos/"+id, "", bearer()...); w.Code != 204 {
		t.Fatalf("end: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/v1/demos/"+id+"/extend", `{"minutes":30}`, bearer()...); w.Code != 404 {
		t.Fatalf("an ended demo: %d", w.Code)
	}
	st := data(t, do(h, "GET", "/v1/stats", "", bearer()...))
	if st["demos"].(map[string]any)["expired"] != float64(1) {
		t.Fatalf("stats: %v", st)
	}
}

func TestOnlyTheMarketingSiteMayCallTheDemoEndpointFromABrowser(t *testing.T) {
	h := newServer(t, nil)
	w := do(h, "OPTIONS", "/v1/demo-requests", "", "Origin", "https://site.example")
	if w.Header().Get("Access-Control-Allow-Origin") != "https://site.example" {
		t.Fatal("the marketing site must be allowed")
	}
	w = do(h, "OPTIONS", "/v1/demo-requests", "", "Origin", "https://evil.example")
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("other origins must not be allowed")
	}
	w = do(h, "OPTIONS", "/v1/leads", "", "Origin", "https://site.example")
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("the back office is never opened to a browser origin")
	}
}

func TestTheVisitorAddressComesFromTheProxyHeader(t *testing.T) {
	h := newServer(t, nil)
	do(h, "POST", "/v1/demo-requests", good, "X-Forwarded-For", "198.51.100.9, 10.0.0.1")
	var list struct{ Data []map[string]any }
	_ = json.Unmarshal(do(h, "GET", "/v1/demos", "", bearer()...).Body.Bytes(), &list)
	if list.Data[0]["ip"] != "198.51.100.9" {
		t.Fatalf("ip: %v", list.Data[0]["ip"])
	}
}

func TestAContactMessageIsStoredOnTheLead(t *testing.T) {
	h := newServer(t, nil)
	body := `{"name":"Ravi","phone":"+91 98765 43210","email":"ravi@kumar.example","company":"Kumar & Sons","interest":"on_prem","message":"We want to host it ourselves. Price for 80 people?"}`
	if w := do(h, "POST", "/v1/contact-requests", body); w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var list struct{ Data []map[string]any }
	_ = json.Unmarshal(do(h, "GET", "/v1/leads", "", bearer()...).Body.Bytes(), &list)
	if len(list.Data) != 1 || list.Data[0]["status"] != "new" || list.Data[0]["source"] != "website-contact" {
		t.Fatalf("lead: %+v", list.Data)
	}
	w := do(h, "GET", "/v1/leads/"+list.Data[0]["id"].(string), "", bearer()...)
	if !strings.Contains(w.Body.String(), "contact_message") || !strings.Contains(w.Body.String(), "Price for 80 people") {
		t.Fatalf("the message belongs in the lead's history: %s", w.Body)
	}
	if w := do(h, "GET", "/v1/demos", "", bearer()...); !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatalf("a contact message starts no demo: %s", w.Body)
	}
}

func TestContactValidationHoneypotAndLimit(t *testing.T) {
	h := newServer(t, nil)
	if w := do(h, "POST", "/v1/contact-requests", `{"name":"A","phone":"+91 98765 43210","email":"a@b.example","message":""}`); w.Code != 422 || !strings.Contains(w.Body.String(), `"message"`) {
		t.Fatalf("a message is required: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/v1/contact-requests", `{"name":"A","phone":"+91 98765 43210","email":"a@b.example","message":"hi","interest":"free-stuff"}`); w.Code != 422 {
		t.Fatalf("unknown interest: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/contact-requests", `{"name":"Bot","email":"b@b.example","message":"buy","website":"http://x"}`); w.Code != 202 {
		t.Fatalf("honeypot: %d", w.Code)
	}
	ok := `{"name":"A","phone":"+91 98765 43210","email":"a@b.example","message":"hi"}`
	for i := 0; i < 5; i++ {
		if w := do(h, "POST", "/v1/contact-requests", ok, "X-Forwarded-For", "198.51.100.20"); w.Code != 201 {
			t.Fatalf("message %d: %d %s", i+1, w.Code, w.Body)
		}
	}
	if w := do(h, "POST", "/v1/contact-requests", ok, "X-Forwarded-For", "198.51.100.20"); w.Code != 429 {
		t.Fatalf("a sixth message in an hour from one address must be limited: %d", w.Code)
	}
}

func TestAttributionIsKeptOnTheLeadHistoryAndStrangersAreDropped(t *testing.T) {
	h := newServer(t, nil)
	body := `{"name":"Asha Rao","phone":"+91 98765 43210","email":"asha@rao.example","attribution":{"utm_source":"google","utm_campaign":"spring","referrer":"https://www.google.com/","evil":"<script>"}}`
	if w := do(h, "POST", "/v1/demo-requests", body); w.Code != 201 {
		t.Fatal(w.Body)
	}
	var list struct{ Data []map[string]any }
	_ = json.Unmarshal(do(h, "GET", "/v1/leads", "", bearer()...).Body.Bytes(), &list)
	w := do(h, "GET", "/v1/leads/"+list.Data[0]["id"].(string), "", bearer()...)
	if !strings.Contains(w.Body.String(), `"utm_source":"google"`) || !strings.Contains(w.Body.String(), "spring") {
		t.Fatalf("attribution should be kept: %s", w.Body)
	}
	if strings.Contains(w.Body.String(), "script") {
		t.Fatalf("unknown keys must be dropped: %s", w.Body)
	}
}

// adminSession creates an admin and returns a live session token for them.
func adminSession(t *testing.T, st *store.Store) string {
	t.Helper()
	u, err := st.CreateUser(context.Background(), "Test Admin", "admin@test.example", store.RoleAdmin, "x")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateSession(context.Background(), u.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

type okMailer struct{ ready bool }

func (m okMailer) Ready(context.Context) bool { return m.ready }
func (okMailer) Queue(context.Context, string, string, string, string, string, time.Time) (store.Email, error) {
	return store.Email{}, nil
}

// fullServer has one demo slot and a waiting line; the first request takes the slot.
func fullServer(t *testing.T, mailReady bool) http.Handler {
	t.Helper()
	cfg := &config.Config{TrustProxy: true,
		Demo: config.Demo{TTL: time.Hour, MaxLive: 1, MaxQueue: 1, QueueWait: time.Hour, MaxPerEmail: 3, MaxPerIP: 10, Domain: "demo.example.com", Scheme: "https"}}
	st := testutil.NewStore(t)
	token = adminSession(t, st)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mailer := mail.New(st, "test-secret", log)
	demos := demo.New(cfg.Demo, st, okDBs{}, okStacks{}, log).WithMailer(okMailer{mailReady})
	return New(cfg, st, demos, mailer, sales.New(st, mailer, log), nil, log)
}

const second = `{"name":"Ben Rao","phone":"+91 98765 43211","email":"ben@rao.example"}`
const third = `{"name":"Cy Rao","phone":"+91 98765 43212","email":"cy@rao.example"}`

func TestAFullHouseQueuesTheVisitorAndSaysSo(t *testing.T) {
	h := fullServer(t, true)
	if w := do(h, "POST", "/v1/demo-requests", good); w.Code != 201 {
		t.Fatal(w.Body)
	}
	w := do(h, "POST", "/v1/demo-requests", second)
	if w.Code != 202 {
		t.Fatalf("want 202, got %d %s", w.Code, w.Body)
	}
	d := data(t, w)
	if d["queued"] != true || d["position"] != float64(1) || d["login_password"] != nil || d["url"] != nil {
		t.Fatalf("a queued answer must carry a place and no login: %v", d)
	}
	// The visitor is a lead for sales even though no demo exists yet.
	if w := do(h, "GET", "/v1/leads?q=ben%40rao.example", "", bearer()...); w.Code != 200 || !strings.Contains(w.Body.String(), "ben@rao.example") {
		t.Fatalf("the queued visitor must be a lead: %d %s", w.Code, w.Body)
	}
	// The line holds one; the next visitor is turned away.
	if w := do(h, "POST", "/v1/demo-requests", third); w.Code != 503 || !strings.Contains(w.Body.String(), "at_capacity") {
		t.Fatalf("want 503 at_capacity, got %d %s", w.Code, w.Body)
	}
}

func TestAFullHouseWithoutMailTurnsVisitorsAway(t *testing.T) {
	h := fullServer(t, false)
	_ = do(h, "POST", "/v1/demo-requests", good)
	if w := do(h, "POST", "/v1/demo-requests", second); w.Code != 503 {
		t.Fatalf("want 503, got %d %s", w.Code, w.Body)
	}
}
