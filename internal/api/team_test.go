package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/config"
	"github.com/redefynelabs/erp-control-plane/internal/demo"
	"github.com/redefynelabs/erp-control-plane/internal/mail"
	"github.com/redefynelabs/erp-control-plane/internal/sales"
	"github.com/redefynelabs/erp-control-plane/internal/testutil"
)

type sink struct {
	mu    sync.Mutex
	sent  []string
	texts []string
	htmls []string
}

func (s *sink) Send(_ context.Context, _ mail.Settings, _ string, to, subject, text, html string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, to+"|"+subject)
	s.texts = append(s.texts, text)
	s.htmls = append(s.htmls, html)
	return nil
}

func (s *sink) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.sent) }

// teamServer is a server with a mail sink, an admin session.
func teamServer(t *testing.T) (h http.Handler, mailer *mail.Service, out *sink) {
	t.Helper()
	cfg := &config.Config{AllowedOrigins: []string{"https://site.example"}, TrustProxy: true,
		Demo: config.Demo{TTL: time.Hour, MaxLive: 10, MaxPerEmail: 5, MaxPerIP: 10, Domain: "demo.example.com", Scheme: "https"}}
	st := testutil.NewStore(t)
	token = adminSession(t, st)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	out = &sink{}
	mailer = mail.New(st, "test-secret", log).WithSender(out)
	team := sales.New(st, mailer, log)
	demos := demo.New(cfg.Demo, st, okDBs{}, okStacks{}, log).WithIntake(team)
	return New(cfg, st, demos, mailer, team, nil, log), mailer, out
}

func call(t *testing.T, h http.Handler, method, path, body, tok string) (int, map[string]any) {
	t.Helper()
	hdr := []string{}
	if tok != "" {
		hdr = []string{"Authorization", "Bearer " + tok}
	}
	w := do(h, method, path, body, hdr...)
	var env struct {
		Data any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	m, _ := env.Data.(map[string]any)
	return w.Code, m
}

func invite(t *testing.T, h http.Handler, name, email, role string) string {
	t.Helper()
	code, d := call(t, h, "POST", "/v1/users", `{"name":"`+name+`","email":"`+email+`","role":"`+role+`"}`, token)
	if code != 201 {
		t.Fatalf("invite: %d", code)
	}
	link := d["invite_url"].(string)
	code, d = call(t, h, "POST", "/v1/auth/accept-invite", `{"token":"`+strings.TrimPrefix(link, "/invite/")+`","password":"a-long-password-1"}`, "")
	if code != 200 {
		t.Fatalf("accept: %d", code)
	}
	return d["token"].(string)
}

func TestSignInAndRoles(t *testing.T) {
	h, _, _ := teamServer(t)
	salesTok := invite(t, h, "Sia", "sia@x.example", "sales")

	if code, _ := call(t, h, "POST", "/v1/auth/login", `{"email":"sia@x.example","password":"wrong-password"}`, ""); code != 401 {
		t.Fatalf("a wrong password is refused: %d", code)
	}
	if code, d := call(t, h, "POST", "/v1/auth/login", `{"email":"SIA@x.example","password":"a-long-password-1"}`, ""); code != 200 || d["token"] == nil {
		t.Fatalf("sign in: %d", code)
	}
	if code, _ := call(t, h, "GET", "/v1/leads", "", ""); code != 401 {
		t.Fatalf("no token: %d", code)
	}
	for _, c := range [][2]string{{"POST", "/v1/users"}, {"GET", "/v1/settings/email"}, {"PUT", "/v1/settings/general"}} {
		if code, _ := call(t, h, c[0], c[1], `{}`, salesTok); code != 403 {
			t.Errorf("%s %s must be admin only, got %d", c[0], c[1], code)
		}
	}
	if code, _ := call(t, h, "GET", "/v1/leads", "", salesTok); code != 200 {
		t.Fatalf("sales works leads: %d", code)
	}
	code, _ := call(t, h, "POST", "/v1/auth/logout", "", salesTok)
	if code != 204 {
		t.Fatalf("logout: %d", code)
	}
	if code, _ := call(t, h, "GET", "/v1/me", "", salesTok); code != 401 {
		t.Fatalf("a signed-out session is dead: %d", code)
	}
}

func TestTheLastAdminCannotBeRemoved(t *testing.T) {
	h, _, _ := teamServer(t)
	adminTok := invite(t, h, "Ana", "ana@x.example", "admin")
	_, me := call(t, h, "GET", "/v1/me", "", adminTok)
	_, first := call(t, h, "GET", "/v1/me", "", token)
	if code, _ := call(t, h, "PATCH", "/v1/users/"+first["id"].(string), `{"active":false}`, adminTok); code != 200 {
		t.Fatalf("with two admins one may go, got %d", code)
	}
	if code, _ := call(t, h, "PATCH", "/v1/users/"+me["id"].(string), `{"active":false}`, adminTok); code != 409 {
		t.Fatalf("want 409, got %d", code)
	}
}

func TestAPricingRequestIsOwnedFollowedUpAndAcknowledged(t *testing.T) {
	h, mailer, out := teamServer(t)
	invite(t, h, "Sia", "sia@x.example", "sales")
	if _, err := mailer.Save(context.Background(), mail.Settings{Enabled: true, Host: "smtp.x", Port: 587, Security: "starttls", FromEmail: "sales@x.example", Signature: "-- team"}, "pw"); err != nil {
		t.Fatal(err)
	}
	if code, _ := call(t, h, "PUT", "/v1/settings/general", `{"assignment":"round_robin","auto_confirm":true,"auto_demo_followup":false,"followup_hours":4,"demo_followup_delay_minutes":0}`, token); code != 200 {
		t.Fatalf("settings: %d", code)
	}
	body := `{"name":"Priya N","phone":"+91 98765 43210","email":"priya@acme.example","company":"Acme","interest":"cloud","plan":"business","team_size":"26-100","message":"60 people"}`
	if code, _ := call(t, h, "POST", "/v1/contact-requests", body, ""); code != 201 {
		t.Fatalf("contact: %d", code)
	}
	_, list := call(t, h, "GET", "/v1/leads?interest=cloud", "", token)
	_ = list
	w := do(h, "GET", "/v1/leads?interest=cloud", "", bearer()...)
	var env struct{ Data []map[string]any }
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if len(env.Data) != 1 || env.Data[0]["plan_interest"] != "business" || env.Data[0]["owner_name"] != "Sia" || env.Data[0]["next_follow_up_at"] == nil {
		t.Fatalf("the lead should carry the plan, an owner and a first follow-up: %v", env.Data)
	}
	mailer.Work(context.Background())
	if out.count() != 1 {
		t.Fatalf("the visitor gets one acknowledgement, got %d", out.count())
	}
}

func TestEmailNeedsSettingsFirst(t *testing.T) {
	h, _, _ := teamServer(t)
	_, lead := call(t, h, "POST", "/v1/leads", `{"name":"Lee","email":"lee@x.example"}`, token)
	id := lead["id"].(string)
	if code, _ := call(t, h, "POST", "/v1/leads/"+id+"/emails", `{"subject":"Hi","body":"Hello"}`, token); code != 409 {
		t.Fatalf("email without settings must say so: %d", code)
	}
}

func TestTasksCanBeAddedAndFinished(t *testing.T) {
	h, _, _ := teamServer(t)
	tok := invite(t, h, "Sia", "sia@x.example", "sales")
	_, lead := call(t, h, "POST", "/v1/leads", `{"name":"Lee","email":"lee@x.example"}`, tok)
	id := lead["id"].(string)
	due := time.Now().Add(-time.Hour).Format(time.RFC3339)
	code, task := call(t, h, "POST", "/v1/leads/"+id+"/tasks", `{"title":"Call Lee","kind":"call","due_at":"`+due+`"}`, tok)
	if code != 201 {
		t.Fatalf("add task: %d", code)
	}
	_, st := call(t, h, "GET", "/v1/stats", "", tok)
	if st["my_overdue_tasks"].(float64) != 1 {
		t.Fatalf("one overdue task: %v", st)
	}
	if code, _ := call(t, h, "PATCH", "/v1/tasks/"+task["id"].(string), `{"done":true}`, tok); code != 200 {
		t.Fatalf("finish: %d", code)
	}
	_, st = call(t, h, "GET", "/v1/stats", "", tok)
	if st["my_open_tasks"].(float64) != 0 {
		t.Fatalf("nothing open: %v", st)
	}
}
