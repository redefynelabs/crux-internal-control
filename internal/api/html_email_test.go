package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/redefynelabs/erp-control-plane/internal/mail"
)

func mailReady(t *testing.T, mailer *mail.Service) {
	t.Helper()
	if _, err := mailer.Save(context.Background(), mail.Settings{Enabled: true, Host: "smtp.x", Port: 587, Security: "starttls", FromEmail: "sales@x.example", Signature: "Sam"}, "pw"); err != nil {
		t.Fatal(err)
	}
}

func listTemplatesJSON(t *testing.T, h http.Handler) []map[string]any {
	t.Helper()
	w := do(h, "GET", "/v1/templates", "", bearer()...)
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

func TestTheStarterTemplatesAreDesignedAndTheSystemOnesExist(t *testing.T) {
	h, _, _ := teamServer(t)
	w := do(h, "GET", "/v1/templates", "", bearer()...)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	body := w.Body.String()
	for _, key := range []string{"contact_received", "demo_followup", "cloud_plan", "proposal_sent", "check_in", "demo_ready", "demo_queue_sorry"} {
		i := strings.Index(body, `"key":"`+key+`"`)
		if i < 0 {
			t.Fatalf("template %s is missing", key)
		}
		end := strings.Index(body[i:], `"updated_at"`)
		if !strings.Contains(body[i:i+end], `"format":"html"`) {
			t.Errorf("template %s should be a designed (html) template", key)
		}
	}
}

func TestTheEditorPreviewsADesignedTemplateWithSampleValues(t *testing.T) {
	h, _, _ := teamServer(t)
	in := `{"format":"html","subject":"Hello {{first_name}}","body":"<h1>Hi {{first_name}}</h1><p>This is a sentence that is long enough to preview.</p>[[button: Go | {{site_url}}/demo]]"}`
	code, d := call(t, h, "POST", "/v1/templates/preview", in, token)
	if code != 200 {
		t.Fatalf("preview: %d", code)
	}
	if d["subject"] != "Hello Asha" || !strings.Contains(d["html"].(string), "<!doctype html>") || !strings.Contains(d["html"].(string), "Hi Asha") || !strings.Contains(d["text"].(string), "Hi Asha") {
		t.Fatalf("unexpected preview: %v", d)
	}
	// With no site address configured the demo button has nowhere to go and is left out.
	if strings.Contains(d["html"].(string), "Go</a>") {
		t.Errorf("a button with no address must not be shown")
	}
	plain := `{"format":"text","subject":"S","body":"Hello {{first_name}}"}`
	_, p := call(t, h, "POST", "/v1/templates/preview", plain, token)
	if p["html"] != "" || p["text"] != "Hello Asha" {
		t.Errorf("a plain template previews as text only: %v", p)
	}
}

func TestOnlyAnAdminCanPreviewOrTestATemplate(t *testing.T) {
	h, _, _ := teamServer(t)
	sales := invite(t, h, "Sia", "sia@x.example", "sales")
	for _, path := range []string{"/v1/templates/preview", "/v1/templates/test"} {
		if code, _ := call(t, h, "POST", path, `{"format":"html","subject":"S","body":"<p>x</p>"}`, sales); code != 403 {
			t.Errorf("%s: a sales person must get 403, got %d", path, code)
		}
	}
}

func TestATestEmailGoesToTheAdminWithBothParts(t *testing.T) {
	h, mailer, out := teamServer(t)
	in := `{"format":"html","subject":"Welcome","body":"<h1>Welcome</h1><p>This is a sentence that is long enough for a test.</p>"}`
	if code, _ := call(t, h, "POST", "/v1/templates/test", in, token); code != 409 {
		t.Fatalf("without email set up want 409, got %d", code)
	}
	mailReady(t, mailer)
	code, d := call(t, h, "POST", "/v1/templates/test", in, token)
	if code != 200 || d["sent_to"] != "admin@test.example" {
		t.Fatalf("test send: %d %v", code, d)
	}
	if out.count() != 1 || out.sent[0] != "admin@test.example|[Test] Welcome" || !strings.Contains(out.htmls[0], "<!doctype html>") || !strings.Contains(out.texts[0], "Welcome") {
		t.Errorf("one message with an HTML and a text part to the admin, got %v", out.sent)
	}
}

func TestATemplateFormatCanBeChangedAndOnlyToKnownOnes(t *testing.T) {
	h, _, _ := teamServer(t)
	_, _ = call(t, h, "GET", "/v1/templates", "", token)
	w := do(h, "GET", "/v1/templates", "", bearer()...)
	i := strings.Index(w.Body.String(), `"id":"`)
	id := w.Body.String()[i+6 : i+6+36]
	put := func(format string) int {
		code, _ := call(t, h, "PUT", "/v1/templates/"+id, `{"name":"N","subject":"S","body":"<p>x</p>","format":"`+format+`"}`, token)
		return code
	}
	if put("html") != 200 || put("text") != 200 {
		t.Fatal("html and text are both valid")
	}
	if put("markdown") != 422 {
		t.Fatal("an unknown format must be refused")
	}
	if code, _ := call(t, h, "PUT", "/v1/templates/"+id, `{"name":"N","subject":"S","body":"x"}`, token); code != 200 {
		t.Fatal("no format means plain text, as before")
	}
}

func TestAnEmailFromADesignedTemplateIsDesignedWithTheNoteOnTop(t *testing.T) {
	h, mailer, out := teamServer(t)
	mailReady(t, mailer)
	_, lead := call(t, h, "POST", "/v1/leads", `{"name":"Lee Ann","email":"lee@x.example","company":"Lee Co"}`, token)
	id := lead["id"].(string)
	var tid string
	for _, tpl := range listTemplatesJSON(t, h) {
		if tpl["key"] == "check_in" {
			tid = tpl["id"].(string)
		}
	}
	if tid == "" {
		t.Fatal("no check_in template")
	}
	body := `{"template_id":"` + tid + `","subject":"Checking in, {{first_name}}","body":"Saw you opened the demo twice <3"}`
	if code, _ := call(t, h, "POST", "/v1/leads/"+id+"/emails", body, token); code != 201 {
		t.Fatalf("send: %d", code)
	}
	mailer.Work(context.Background())
	if out.count() != 1 || out.sent[0] != "lee@x.example|Checking in, Lee" {
		t.Fatalf("one mail, subject filled in: %v", out.sent)
	}
	html := out.htmls[0]
	note, hello := strings.Index(html, "Saw you opened the demo twice &lt;3"), strings.Index(html, "Hi Lee,")
	if !strings.Contains(html, "<!doctype html>") || note < 0 || hello < 0 || note > hello {
		t.Errorf("the designed email carries the personal note (escaped) above the template")
	}
	if !strings.Contains(out.texts[0], "Saw you opened the demo twice <3") || strings.Contains(out.texts[0], "<p>") {
		t.Errorf("the text part is plain: %q", out.texts[0])
	}
}

func TestADesignedTemplateWithoutANoteIsStillValidButAPlainOneNeedsABody(t *testing.T) {
	h, mailer, _ := teamServer(t)
	mailReady(t, mailer)
	_, lead := call(t, h, "POST", "/v1/leads", `{"name":"Lee","email":"lee@x.example"}`, token)
	id := lead["id"].(string)
	tid := ""
	for _, tpl := range listTemplatesJSON(t, h) {
		if tpl["key"] == "check_in" {
			tid = tpl["id"].(string)
		}
	}
	if code, _ := call(t, h, "POST", "/v1/leads/"+id+"/emails", `{"template_id":"`+tid+`","subject":"Hi","body":""}`, token); code != 201 {
		t.Errorf("a designed template needs no note, got %d", code)
	}
	if code, _ := call(t, h, "POST", "/v1/leads/"+id+"/emails", `{"subject":"Hi","body":""}`, token); code != 422 {
		t.Errorf("a plain message with no body must be refused, got %d", code)
	}
}

func TestTheLogoIsServedPubliclyAndCached(t *testing.T) {
	h, _, _ := teamServer(t)
	w := do(h, "GET", "/v1/brand/logo.png", "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !strings.Contains(w.Header().Get("Cache-Control"), "max-age") || w.Body.Len() < 1000 {
		t.Fatalf("logo: %d %q %q %d bytes", w.Code, w.Header().Get("Content-Type"), w.Header().Get("Cache-Control"), w.Body.Len())
	}
}
