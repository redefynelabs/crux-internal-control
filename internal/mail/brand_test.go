package mail

import (
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
)

var testVars = map[string]string{
	"first_name": "Asha", "name": "Asha Rao", "company": "Rao Traders", "demo_url": "https://abc.demo.example.com",
	"login_email": "asha@x.example", "login_password": "Xk7mNp2qRt9wBc", "signature": "Sam\nCrux", "site_url": "https://site.example",
}

func compose(content string, vars map[string]string) Rendered {
	return Composer{LogoURL: "https://api.example.com/v1/brand/logo.png"}.Compose(FormatHTML, "Hi {{first_name}}", content, vars)
}

func TestADesignedMessageHasTheBrandedLayoutLogoAndBothParts(t *testing.T) {
	r := compose(`<h1>Welcome, {{first_name}}</h1><p>Your demo is ready and waiting for you right now.</p>[[button: Open your demo | {{demo_url}}]]`, testVars)
	for _, want := range []string{`<!doctype html>`, `src="https://api.example.com/v1/brand/logo.png"`, ">Crux<", "Welcome, Asha", `href="https://abc.demo.example.com"`, "Open your demo", "Just reply to this email"} {
		if !strings.Contains(r.HTML, want) {
			t.Errorf("the HTML lacks %q", want)
		}
	}
	if r.Subject != "Hi Asha" {
		t.Errorf("subject: %q", r.Subject)
	}
	if strings.Contains(r.Text, "<") || !strings.Contains(r.Text, "Welcome, Asha") || !strings.Contains(r.Text, "Open your demo: https://abc.demo.example.com") {
		t.Errorf("the plain-text copy must be readable text with the button as a labelled link, got:\n%s", r.Text)
	}
	// The grey preview line after the subject is the first real sentence, not the heading.
	if !strings.Contains(r.HTML, "Your demo is ready and waiting for you right now.&nbsp;") {
		t.Errorf("the preheader should be the first sentence")
	}
}

func TestPlaceholderValuesCanNeverBecomeMarkup(t *testing.T) {
	vars := map[string]string{"first_name": `<script>alert(1)</script>`, "name": `A "B" & C`, "signature": "Line one\nLine two",
		"demo_url": "https://abc.example.com", "login_password": "[[button: Pwn | https://evil.example]]"}
	r := compose(`<p>Hi {{first_name}}, {{name}}</p><p>{{signature}}</p>[[details: Password = {{login_password}}]]`, vars)
	if strings.Contains(r.HTML, "<script>") || !strings.Contains(r.HTML, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("markup in a value must be escaped")
	}
	if !strings.Contains(r.HTML, "A &#34;B&#34; &amp; C") || !strings.Contains(r.HTML, "Line one<br>Line two") {
		t.Errorf("quotes are escaped and line breaks kept")
	}
	if strings.Contains(r.HTML, "evil.example") && strings.Contains(r.HTML, `href="https://evil.example`) {
		t.Errorf("a shortcode typed into a value must not run")
	}
	if !strings.Contains(r.Text, "<script>alert(1)</script>") {
		t.Errorf("the text copy shows values as typed")
	}
}

func TestShortcodesAndTheirTextCopies(t *testing.T) {
	r := compose(`[[details: Email = {{login_email}} | Password = {{login_password}}]]
[[steps: Open the dashboard | Ask Mei a question]]
[[button: Go | {{demo_url}}]]`, testVars)
	for _, want := range []string{"asha@x.example", "Xk7mNp2qRt9wBc", "Open the dashboard", "Ask Mei a question"} {
		if !strings.Contains(r.HTML, want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
	for _, want := range []string{"Email: asha@x.example", "Password: Xk7mNp2qRt9wBc", "1. Open the dashboard", "2. Ask Mei a question", "Go: https://abc.demo.example.com"} {
		if !strings.Contains(r.Text, want) {
			t.Errorf("text lacks %q, got:\n%s", want, r.Text)
		}
	}
}

func TestAButtonWithNoUsableAddressIsLeftOut(t *testing.T) {
	for _, url := range []string{"{{site_url}}/demo", "javascript:alert(1)", "ftp://x.example"} {
		vars := map[string]string{"site_url": ""}
		r := compose(`<p>Hello there, this is a long enough sentence.</p>[[button: Try it | `+url+`]]`, vars)
		if strings.Contains(r.HTML, "Try it") || strings.Contains(r.Text, "Try it") {
			t.Errorf("%q: a dead or unsafe button must be dropped", url)
		}
	}
	r := compose(`[[button: Reply | mailto:{{sender_email}}?subject=Hi]]`, map[string]string{"sender_email": "sales@x.example"})
	if !strings.Contains(r.HTML, `href="mailto:sales@x.example?subject=Hi"`) {
		t.Errorf("a mailto button is fine")
	}
}

func TestAPlainTemplateStaysPlain(t *testing.T) {
	r := Composer{}.Compose(FormatText, "Hi {{first_name}}", "Hello {{first_name}}\n\n{{signature}}", testVars)
	if r.HTML != "" || r.Text != "Hello Asha\n\nSam\nCrux" {
		t.Errorf("got %+v", r)
	}
}

func TestRenderExpandsOnlyOnce(t *testing.T) {
	got := Render("{{a}} {{b}} {{c}}", map[string]string{"a": "{{b}}", "b": "B"})
	if got != "{{b}} B {{c}}" {
		t.Errorf("a value must not be expanded again, and unknown placeholders stay: %q", got)
	}
}

func TestHTMLToText(t *testing.T) {
	got := htmlToText(`<h1>Title</h1><p>One <b>bold</b> &amp; <a href="https://x.example/p">link</a>.</p><ul><li>a</li><li>b</li></ul><p>Line<br>break</p>`)
	for _, want := range []string{"Title", "One bold & link (https://x.example/p).", "- a", "- b", "Line\nbreak"} {
		if !strings.Contains(got, want) {
			t.Errorf("text lacks %q, got:\n%s", want, got)
		}
	}
}

// The message must really be multipart/alternative with a plain part and an HTML part, whatever the content, so mail apps
// and spam filters read it correctly.
func TestTheMessageIsMultipartAlternative(t *testing.T) {
	from := mail.Address{Name: "Crux", Address: "hello@x.example"}
	r := compose(`<h1>Café — ready</h1><p>`+strings.Repeat("A long line of text. ", 80)+`</p>[[button: Open | https://x.example/a?b=c&d=e]]`, testVars)
	raw := build(from, "reply@x.example", "asha@x.example", "Hi café", r.Text, r.HTML)
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("content type: %q %v", mt, err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var types []string
	var htmlPart string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ct, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		types = append(types, ct)
		body, _ := io.ReadAll(quotedprintable.NewReader(p))
		if ct == "text/html" {
			htmlPart = string(body)
		}
	}
	if len(types) != 2 || types[0] != "text/plain" || types[1] != "text/html" {
		t.Fatalf("parts: %v (plain first, the richest last)", types)
	}
	if !strings.Contains(htmlPart, "Café") || !strings.Contains(htmlPart, "b=c&amp;d=e") {
		t.Errorf("the HTML part must survive encoding intact")
	}
	for _, line := range strings.Split(string(raw), "\r\n") {
		if len(line) > 998 {
			t.Fatalf("a line of %d characters breaks the SMTP limit", len(line))
		}
	}
	if msg.Header.Get("Reply-To") != "reply@x.example" {
		t.Errorf("reply-to lost")
	}
}

func TestAPlainMessageIsStillPlain(t *testing.T) {
	raw := build(mail.Address{Address: "a@x.example"}, "", "b@x.example", "Hello", "Just text", "")
	if !strings.Contains(string(raw), "Content-Type: text/plain") || strings.Contains(string(raw), "multipart") {
		t.Errorf("a plain message must stay plain:\n%s", raw)
	}
}

func TestTheLogoIsAPNG(t *testing.T) {
	if b := LogoPNG(); len(b) < 1000 || string(b[1:4]) != "PNG" {
		t.Errorf("the embedded logo must be a PNG, got %d bytes", len(b))
	}
}

func TestAnEyebrowIsAnUppercaseLabelAboveTheHeading(t *testing.T) {
	r := compose(`[[eyebrow: Demo ready]]<h1>Your own copy is live</h1><p>This is a sentence that is long enough to preview.</p>`, testVars)
	if !strings.Contains(r.HTML, "text-transform:uppercase") || !strings.Contains(r.HTML, ">Demo ready</p>") {
		t.Errorf("the HTML shows the label in small uppercase")
	}
	if !strings.HasPrefix(r.Text, "DEMO READY\n") {
		t.Errorf("the text copy leads with the label in capitals, got %q", r.Text[:20])
	}
	if r := compose(`[[eyebrow:    ]]<p>Only a body, long enough to be a sentence.</p>`, testVars); strings.Contains(r.HTML, "uppercase;color") {
		t.Errorf("an empty label is left out")
	}
}

// The look follows the website: a pill button, square bordered boxes and the site's own accent blue.
func TestTheDesignFollowsTheWebsite(t *testing.T) {
	r := compose(`[[button: Go | https://x.example]][[details: A = b]][[steps: one]]`, testVars)
	for _, want := range []string{"border-radius:999px", "#2563eb", "border:1px solid #e5e5e5"} {
		if !strings.Contains(r.HTML, want) {
			t.Errorf("the HTML should contain %q", want)
		}
	}
	if strings.Contains(r.HTML, "border-radius:6px") || strings.Contains(r.HTML, "border-radius:12px") {
		t.Errorf("only the button is rounded; boxes are square like the site")
	}
}
