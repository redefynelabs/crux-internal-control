package mail

import (
	_ "embed"
	"fmt"
	"html"
	"regexp"
	"strings"
)

//go:embed assets/crux-mark.png
var logoPNG []byte

// LogoPNG is the Crux mark shown at the top of every designed email. The API serves it so the emails can link to it.
func LogoPNG() []byte { return logoPNG }

// Brand is how designed emails look. Crux is the product the visitor tried, so it is the name on every message.
const (
	BrandName  = "Crux"
	brandColor = "#0f5af1"
	fontStack  = "-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif"
)

// Template formats.
const (
	FormatText = "text"
	FormatHTML = "html"
)

// Rendered is one message ready to queue. HTML is empty for a plain-text template.
type Rendered struct {
	Subject, Text, HTML string
}

// Composer turns templates into messages. LogoURL is the public address of the logo (empty: the name is shown alone).
type Composer struct{ LogoURL string }

var placeholder = regexp.MustCompile(`\{\{(\w+)\}\}`)

// Render fills {{placeholders}} in text in one pass, so a value that happens to contain "{{x}}" is never expanded again.
// Unknown placeholders are left as they are.
func Render(text string, vars map[string]string) string {
	return placeholder.ReplaceAllStringFunc(text, func(m string) string {
		if v, ok := vars[m[2:len(m)-2]]; ok {
			return v
		}
		return m
	})
}

// escapeHTML makes a value safe to put in an HTML message: markup is shown as typed and line breaks are kept.
func escapeHTML(v string) string {
	return strings.ReplaceAll(html.EscapeString(strings.ReplaceAll(v, "\r\n", "\n")), "\n", "<br>")
}

func renderEscaped(text string, vars map[string]string) string {
	return placeholder.ReplaceAllStringFunc(text, func(m string) string {
		if v, ok := vars[m[2:len(m)-2]]; ok {
			return escapeHTML(v)
		}
		return m
	})
}

// Compose renders a template for one person. A "text" template is plain text and has no HTML part. An "html" template
// holds an HTML fragment (headings, paragraphs, lists, links and the [[button]], [[details]] and [[steps]] shortcodes);
// it is wrapped in the branded layout and gets an automatic plain-text copy. Placeholder values are escaped in the HTML
// and used as typed in the text, so a name like "<script>" can never become markup.
func (c Composer) Compose(format, subject, content string, vars map[string]string) Rendered {
	subject = strings.TrimSpace(Render(subject, vars))
	if format != FormatHTML {
		return Rendered{Subject: subject, Text: Render(content, vars)}
	}
	body := renderEscaped(expandShortcodes(content, vars, false), vars)
	text := Render(htmlToText(expandShortcodes(content, vars, true)), vars)
	return Rendered{Subject: subject, Text: text, HTML: c.layout(body, preheader(text), subject)}
}

var shortcode = regexp.MustCompile(`(?s)\[\[(button|steps|details):\s*(.*?)\]\]`)

// expandShortcodes turns [[button: Label | URL]], [[steps: a | b | c]] and [[details: Label = value | ...]] into email-safe
// markup (table based, inline styles) or, for the plain-text copy, into plain lines. A button whose address is empty or
// not http(s)/mailto is left out, so an email never carries a dead button.
func expandShortcodes(content string, vars map[string]string, text bool) string {
	return shortcode.ReplaceAllStringFunc(content, func(m string) string {
		parts := shortcode.FindStringSubmatch(m)
		kind, args := parts[1], parts[2]
		switch kind {
		case "button":
			label, url, _ := strings.Cut(args, "|")
			label, url = strings.TrimSpace(label), strings.TrimSpace(Render(strings.TrimSpace(url), vars))
			if label == "" || !safeLink(url) {
				return ""
			}
			if text {
				return "\n\n" + label + ": " + url + "\n\n"
			}
			return fmt.Sprintf(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:28px 0"><tr><td align="center" bgcolor="%[1]s" style="border-radius:6px"><a href="%[2]s" target="_blank" style="display:inline-block;padding:14px 26px;font-family:%[3]s;font-size:15px;font-weight:600;line-height:1;color:#ffffff;text-decoration:none;border-radius:6px;background:%[1]s">%[4]s</a></td></tr></table>`,
				brandColor, html.EscapeString(url), fontStack, label)
		case "steps":
			var b strings.Builder
			if !text {
				b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:10px 0 20px">`)
			} else {
				b.WriteString("\n\n")
			}
			for i, s := range splitArgs(args) {
				if text {
					fmt.Fprintf(&b, "%d. %s\n", i+1, s)
					continue
				}
				fmt.Fprintf(&b, `<tr><td width="30" valign="top" style="padding:6px 0;font-family:%s;font-size:15px;font-weight:700;line-height:1.55;color:%s">%d.</td><td valign="top" style="padding:6px 0;font-family:%s;font-size:15px;line-height:1.55;color:#374151">%s</td></tr>`,
					fontStack, brandColor, i+1, fontStack, s)
			}
			if !text {
				b.WriteString(`</table>`)
			}
			return b.String()
		case "details":
			// Label and value rows between two hairlines: no tinted box, so it reads as part of the page, not a widget.
			var b strings.Builder
			if !text {
				b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:22px 0;border-top:1px solid #e5e7eb;border-bottom:1px solid #e5e7eb">`)
			} else {
				b.WriteString("\n\n")
			}
			rows := splitArgs(args)
			for i, row := range rows {
				label, value, _ := strings.Cut(row, "=")
				label, value = strings.TrimSpace(label), strings.TrimSpace(value)
				if text {
					fmt.Fprintf(&b, "%s: %s\n", label, value)
					continue
				}
				line := "border-bottom:1px solid #f0f1f4"
				if i == len(rows)-1 {
					line = "border-bottom:0"
				}
				fmt.Fprintf(&b, `<tr><td width="32%%" valign="top" style="padding:12px 0;font-family:%s;font-size:13px;color:#6b7280;%s">%s</td><td valign="top" style="padding:12px 0;font-family:'SFMono-Regular',Menlo,Consolas,monospace;font-size:15px;font-weight:600;color:#111827;%s;overflow-wrap:anywhere;word-break:normal">%s</td></tr>`,
					fontStack, line, label, line, value)
			}
			if !text {
				b.WriteString(`</table>`)
			}
			return b.String()
		}
		return m
	})
}

func splitArgs(args string) []string {
	var out []string
	for _, s := range strings.Split(args, "|") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func safeLink(u string) bool {
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "mailto:")
}

var (
	reBreak  = regexp.MustCompile(`(?i)<br\s*/?>`)
	reBlock  = regexp.MustCompile(`(?i)</(p|h[1-6]|div|tr|ul|ol|table)>`)
	reItem   = regexp.MustCompile(`(?i)<li[^>]*>`)
	reLink   = regexp.MustCompile(`(?is)<a\s[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	reTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	reBlanks = regexp.MustCompile(`\n{3,}`)
)

// htmlToText makes the plain-text copy of an HTML fragment: block ends become line breaks, list items get a dash, links
// show their address, and everything else is stripped.
func htmlToText(h string) string {
	h = reLink.ReplaceAllStringFunc(h, func(m string) string {
		p := reLink.FindStringSubmatch(m)
		label := strings.TrimSpace(reTag.ReplaceAllString(p[2], ""))
		if label == "" || strings.Contains(label, p[1]) || strings.TrimPrefix(p[1], "mailto:") == label {
			return label + " " + p[1]
		}
		return label + " (" + p[1] + ")"
	})
	h = reBreak.ReplaceAllString(h, "\n")
	h = reItem.ReplaceAllString(h, "\n- ")
	h = reBlock.ReplaceAllString(h, "\n\n")
	h = html.UnescapeString(reTag.ReplaceAllString(h, ""))
	lines := strings.Split(h, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(reBlanks.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

// preheader is the grey preview line mail apps show after the subject: the first real sentence of the message.
func preheader(text string) string {
	for _, l := range strings.Split(text, "\n") {
		if len(l) > 30 && !strings.Contains(l, "://") {
			if len(l) > 110 {
				l = l[:110] + "…"
			}
			return l
		}
	}
	return ""
}

// layout wraps a message in the branded shell: the logo centred above a plain white page, the message in a single
// readable column with no card around it, and a quiet centred footer. It is built for mail apps, not browsers: tables,
// inline styles, a 560px column that shrinks on a phone, and a name beside the logo so the header still reads when images
// are blocked.
func (c Composer) layout(content, pre, title string) string {
	logo := ""
	if c.LogoURL != "" {
		logo = fmt.Sprintf(`<img src="%s" width="40" height="40" alt="" style="display:inline-block;vertical-align:middle;border:0;outline:none;margin-right:10px">`, html.EscapeString(c.LogoURL))
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light"><meta name="supported-color-schemes" content="light"><title>%[1]s</title>
<style>
body{margin:0;padding:0;background:#ffffff;-webkit-text-size-adjust:100%%}
p{margin:0 0 16px;font-size:16px;line-height:1.65;color:#374151}
h1{margin:0 0 18px;font-size:24px;line-height:1.3;color:#111827;font-weight:700;letter-spacing:-0.01em}
h2{margin:24px 0 8px;font-size:16px;line-height:1.3;color:#111827;font-weight:700}
ul,ol{margin:0 0 16px;padding-left:20px;font-size:16px;line-height:1.65;color:#374151}li{margin-bottom:6px}
a{color:%[2]s}strong{color:#111827}.muted{color:#6b7280!important;font-size:13px!important;line-height:1.55!important}
@media only screen and (max-width:600px){.col{padding-left:22px!important;padding-right:22px!important}h1{font-size:22px!important}}
</style></head>
<body style="margin:0;padding:0;background:#ffffff">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;mso-hide:all;font-size:1px;line-height:1px;color:#ffffff">%[3]s&nbsp;&zwnj;&nbsp;&zwnj;&nbsp;&zwnj;&nbsp;&zwnj;&nbsp;&zwnj;</div>
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" bgcolor="#ffffff"><tr><td align="center" style="padding:44px 0 40px">
<table role="presentation" width="560" cellpadding="0" cellspacing="0" border="0" style="width:100%%;max-width:560px">
<tr><td class="col" align="center" style="padding:0 0 34px;font-family:%[4]s;font-size:22px;font-weight:800;color:#111827;letter-spacing:-0.01em;text-align:center">%[5]s<span style="vertical-align:middle">%[6]s</span></td></tr>
<tr><td class="col" style="padding:0;font-family:%[4]s;font-size:16px;line-height:1.65;color:#374151">
%[7]s
</td></tr>
<tr><td class="col" style="padding:34px 0 0"><table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0"><tr><td style="border-top:1px solid #e5e7eb;padding-top:18px;text-align:center;font-family:%[4]s;font-size:12px;line-height:1.7;color:#9ca3af">You are getting this because you asked about %[6]s.<br>Just reply to this email: it goes to a real person.</td></tr></table></td></tr>
</table></td></tr></table></body></html>`,
		html.EscapeString(title), brandColor, html.EscapeString(pre), fontStack, logo, BrandName, content)
}

// NoteHTML turns a plain-text note typed by a person (a line or two put above a template) into paragraphs.
func NoteHTML(note string) string {
	var b strings.Builder
	for _, para := range strings.Split(strings.ReplaceAll(strings.TrimSpace(note), "\r\n", "\n"), "\n\n") {
		if para = strings.TrimSpace(para); para != "" {
			b.WriteString("<p>" + escapeHTML(para) + "</p>\n")
		}
	}
	return b.String()
}

// SampleVars are the values the template preview and the test email use.
func SampleVars() map[string]string {
	return map[string]string{
		"name": "Asha Rao", "first_name": "Asha", "company": "Rao Traders", "owner": "Sam Carter",
		"signature": "Sam Carter\n" + BrandName, "demo_url": "https://x7k2m9qa.demo.example.com",
		"login_email": "asha@raotraders.example", "login_password": "Xk7mNp2qRt9wBc", "ends_at": "14:30 UTC", "minutes": "60",
	}
}
