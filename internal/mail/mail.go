// Package mail sends the control plane's email: follow-ups from the sales team and the automatic messages. Settings live
// in the database (Settings → Email); the SMTP password is encrypted with a key from the environment.
package mail

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/store"
)

// ErrNotConfigured is returned when email has not been set up or is switched off.
var ErrNotConfigured = errors.New("email is not set up")

const settingsKey = "email"

// Settings is how mail is sent. PasswordEnc is never sent to a browser.
type Settings struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Security    string `json:"security"` // starttls, tls or none
	Username    string `json:"username"`
	PasswordEnc string `json:"password_enc"`
	FromName    string `json:"from_name"`
	FromEmail   string `json:"from_email"`
	ReplyTo     string `json:"reply_to"`
	Signature   string `json:"signature"`
}

// Securities lists the connection modes.
var Securities = []string{"starttls", "tls", "none"}

// Validate checks the fields that must be right for mail to work.
func (s Settings) Validate() map[string]string {
	f := map[string]string{}
	if strings.TrimSpace(s.Host) == "" {
		f["host"] = "Enter the SMTP server"
	}
	if s.Port < 1 || s.Port > 65535 {
		f["port"] = "Enter a port between 1 and 65535"
	}
	ok := false
	for _, v := range Securities {
		ok = ok || v == s.Security
	}
	if !ok {
		f["security"] = "Choose one of the options"
	}
	if a, err := mail.ParseAddress(s.FromEmail); err != nil || a.Address != strings.TrimSpace(s.FromEmail) {
		f["from_email"] = "Enter a valid email address"
	}
	if s.ReplyTo != "" {
		if a, err := mail.ParseAddress(s.ReplyTo); err != nil || a.Address != strings.TrimSpace(s.ReplyTo) {
			f["reply_to"] = "Enter a valid email address"
		}
	}
	if len(s.Signature) > 2000 {
		f["signature"] = "Too long"
	}
	return f
}

// Sender delivers one message. The real one speaks SMTP; tests use a fake.
type Sender interface {
	// html is empty for a plain-text message; otherwise the message carries both parts and the mail app picks one.
	Send(ctx context.Context, s Settings, password, to, subject, text, html string) error
}

// Service sends and queues mail.
type Service struct {
	store  *store.Store
	key    [32]byte
	log    *slog.Logger
	sender Sender
	logo   string
	site   string
}

// New returns the service. secret is any long string from the environment; the encryption key is derived from it.
func New(st *store.Store, secret string, log *slog.Logger) *Service {
	return &Service{store: st, key: sha256.Sum256([]byte("cp-mail:" + secret)), log: log, sender: SMTP{}}
}

// WithPublicURL tells designed emails where the control plane is reachable, so they can link to the logo it serves.
func (m *Service) WithPublicURL(u string) *Service {
	if u = strings.TrimRight(strings.TrimSpace(u), "/"); u != "" {
		m.logo = u + "/v1/brand/logo.png"
	}
	return m
}

// WithSiteURL sets the marketing site's address, available to templates as {{site_url}} (for "start a demo" and "pricing" links).
func (m *Service) WithSiteURL(u string) *Service {
	m.site = strings.TrimRight(strings.TrimSpace(u), "/")
	return m
}

// Enrich adds the values every template can use that do not depend on the person: {{site_url}} and {{sender_email}} (where
// replies go). Values already in vars win.
func (m *Service) Enrich(ctx context.Context, vars map[string]string) map[string]string {
	if vars == nil {
		vars = map[string]string{}
	}
	if _, ok := vars["site_url"]; !ok {
		vars["site_url"] = m.site
	}
	if _, ok := vars["sender_email"]; !ok {
		s, _ := m.Settings(ctx)
		vars["sender_email"] = firstNonEmpty(s.ReplyTo, s.FromEmail)
	}
	return vars
}

// Composer returns the one that renders templates with this service's logo.
func (m *Service) Composer() Composer { return Composer{LogoURL: m.logo} }

// WithSender replaces the SMTP sender (tests).
func (m *Service) WithSender(s Sender) *Service { m.sender = s; return m }

func (m *Service) encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(m.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (m *Service) decrypt(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(m.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("mail: the saved password cannot be read")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("mail: the saved password cannot be read; enter it again")
	}
	return string(plain), nil
}

// Settings returns the saved settings, or defaults.
func (m *Service) Settings(ctx context.Context) (Settings, error) {
	s := Settings{Port: 587, Security: "starttls"}
	_, err := m.store.GetSetting(ctx, settingsKey, &s)
	return s, err
}

// Save stores new settings. A non-empty password replaces the saved one; empty keeps it.
func (m *Service) Save(ctx context.Context, in Settings, password string) (Settings, error) {
	cur, err := m.Settings(ctx)
	if err != nil {
		return in, err
	}
	in.PasswordEnc = cur.PasswordEnc
	if password != "" {
		if in.PasswordEnc, err = m.encrypt(password); err != nil {
			return in, err
		}
	}
	if err := m.store.PutSetting(ctx, settingsKey, in); err != nil {
		return in, err
	}
	return in, nil
}

// Ready says whether mail can go out now.
func (m *Service) Ready(ctx context.Context) bool {
	s, err := m.Settings(ctx)
	return err == nil && s.Enabled && len(s.Validate()) == 0
}

// SendNow sends one message straight away with the saved settings (used by the test button).
func (m *Service) SendNow(ctx context.Context, to, subject, body string) error {
	return m.SendNowHTML(ctx, to, subject, body, "")
}

// SendNowHTML is SendNow for a designed message that also has an HTML part.
func (m *Service) SendNowHTML(ctx context.Context, to, subject, text, html string) error {
	s, err := m.Settings(ctx)
	if err != nil {
		return err
	}
	if len(s.Validate()) > 0 {
		return ErrNotConfigured
	}
	pw, err := m.decrypt(s.PasswordEnc)
	if err != nil {
		return err
	}
	return m.sender.Send(ctx, s, pw, to, subject, text, html)
}

// Vars builds the placeholder values for a lead.
func Vars(name, company, demoURL, owner, signature string) map[string]string {
	first := strings.Fields(name)
	f := name
	if len(first) > 0 {
		f = first[0]
	}
	return map[string]string{"name": name, "first_name": f, "company": company, "demo_url": demoURL, "owner": owner, "signature": signature}
}

// Queue adds a message for a lead to the outbox. It fails with ErrNotConfigured when mail is not ready, so the person
// sees the problem at once instead of a message that never leaves.
func (m *Service) Queue(ctx context.Context, leadID, sentBy, to, subject, body string, after time.Time) (store.Email, error) {
	if !m.Ready(ctx) {
		return store.Email{}, ErrNotConfigured
	}
	return m.store.QueueEmail(ctx, leadID, sentBy, to, subject, body, after)
}

// QueueHTML is Queue for a designed message: text is what shows in the lead's history and in plain-text mail apps, html
// is the designed version.
func (m *Service) QueueHTML(ctx context.Context, leadID, sentBy, to, subject, text, html string, after time.Time) (store.Email, error) {
	if !m.Ready(ctx) {
		return store.Email{}, ErrNotConfigured
	}
	return m.store.QueueEmailHTML(ctx, leadID, sentBy, to, subject, text, html, after)
}

// QueueRendered queues a composed message, designed or plain.
func (m *Service) QueueRendered(ctx context.Context, leadID, sentBy, to string, r Rendered, after time.Time) (store.Email, error) {
	return m.QueueHTML(ctx, leadID, sentBy, to, r.Subject, r.Text, r.HTML, after)
}

// ErrNoTemplate is returned by ComposeTemplate when the template does not exist.
var ErrNoTemplate = errors.New("no such email template")

// ComposeTemplate renders the template with this key for one person. vars should come from Vars, plus any extra values.
func (m *Service) ComposeTemplate(ctx context.Context, key string, vars map[string]string) (Rendered, error) {
	t, err := m.store.TemplateByKey(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		return Rendered{}, ErrNoTemplate
	}
	if err != nil {
		return Rendered{}, err
	}
	return m.Composer().Compose(t.Format, t.Subject, t.Body, m.Enrich(ctx, vars)), nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// RunWorker sends queued mail until ctx ends.
func (m *Service) RunWorker(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Work(ctx)
		}
	}
}

const maxAttempts = 3

// Work sends every message that is due. A failure is recorded on the message and on the lead's history; after three
// attempts the message is marked failed.
func (m *Service) Work(ctx context.Context) {
	s, err := m.Settings(ctx)
	if err != nil || !s.Enabled || len(s.Validate()) > 0 {
		return
	}
	due, err := m.store.ClaimDueEmails(ctx, 20)
	if err != nil {
		m.log.Error("mail: claiming", "error", err)
		return
	}
	if len(due) == 0 {
		return
	}
	pw, perr := m.decrypt(s.PasswordEnc)
	for _, e := range due {
		err := perr
		if err == nil {
			err = m.sender.Send(ctx, s, pw, e.ToEmail, e.Subject, e.Body, e.BodyHTML)
		}
		if err == nil {
			_ = m.store.MarkEmailSent(ctx, e.ID)
			if e.LeadID != nil {
				_ = m.store.TouchContact(ctx, *e.LeadID)
				_ = m.store.AddEventBy(ctx, *e.LeadID, store.Actor{Name: e.SentByName}, "email_sent", map[string]any{"subject": e.Subject, "to": e.ToEmail})
			}
			continue
		}
		final := e.Attempts >= maxAttempts
		_ = m.store.MarkEmailFailed(ctx, e.ID, err.Error(), final)
		m.log.Warn("mail: not sent", "attempt", e.Attempts, "error", err)
		if final && e.LeadID != nil {
			_ = m.store.AddEvent(ctx, *e.LeadID, "email_failed", map[string]any{"subject": e.Subject, "error": err.Error()})
		}
	}
}

// SMTP sends over a real SMTP connection.
type SMTP struct{}

func clean(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(strings.TrimSpace(s))
}

// Send implements Sender. Errors say what failed without echoing the password.
func (SMTP) Send(ctx context.Context, s Settings, password, to, subject, text, html string) error {
	if _, err := mail.ParseAddress(to); err != nil {
		return errors.New("the recipient address is not valid")
	}
	addr := net.JoinHostPort(strings.TrimSpace(s.Host), strconv.Itoa(s.Port))
	d := net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if s.Security == "tls" {
		conn, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("could not connect to %s: %w", addr, unwrapNet(err))
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("the mail server did not greet us: %w", err)
	}
	defer c.Close()
	if s.Security == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("the mail server does not offer STARTTLS; choose another security setting")
		}
		if err := c.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("could not start an encrypted connection: %w", err)
		}
	}
	if s.Username != "" {
		if err := c.Auth(loginAuth{user: s.Username, pass: password, host: s.Host, secure: s.Security != "none"}); err != nil {
			return fmt.Errorf("the mail server refused the username or password: %w", err)
		}
	}
	from := mail.Address{Name: clean(s.FromName), Address: s.FromEmail}
	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("the sender address was refused: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("the recipient was refused: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(build(from, s.ReplyTo, to, subject, text, html)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("the mail server rejected the message: %w", err)
	}
	return c.Quit()
}

func unwrapNet(err error) error {
	var op *net.OpError
	if errors.As(err, &op) && op.Err != nil {
		return op.Err
	}
	return err
}

// build makes a UTF-8 message: plain text, or (with html) multipart/alternative with both parts. Header values are
// stripped of line breaks, which closes header injection.
func build(from mail.Address, replyTo, to, subject, text, html string) []byte {
	var b strings.Builder
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", from.String())
	h("To", clean(to))
	if replyTo != "" {
		h("Reply-To", clean(replyTo))
	}
	h("Subject", mime.QEncoding.Encode("utf-8", clean(subject)))
	h("Date", time.Now().Format(time.RFC1123Z))
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	domain := "localhost"
	if _, d, ok := strings.Cut(from.Address, "@"); ok {
		domain = d
	}
	h("Message-ID", fmt.Sprintf("<%x@%s>", id, domain))
	h("MIME-Version", "1.0")
	if html == "" {
		h("Content-Type", "text/plain; charset=utf-8")
		h("Content-Transfer-Encoding", "8bit")
		b.WriteString("\r\n")
		b.WriteString(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n"))
		b.WriteString("\r\n")
		return []byte(b.String())
	}
	boundary := fmt.Sprintf("=_alt_%x", id)
	h("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	part := func(ctype, body string) {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + ctype + "; charset=utf-8\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		w := quotedprintable.NewWriter(&qpBuf{&b})
		_, _ = w.Write([]byte(strings.ReplaceAll(body, "\r\n", "\n")))
		_ = w.Close()
		b.WriteString("\r\n")
	}
	// The plain part first and the richest last: mail apps show the last part they understand.
	part("text/plain", text)
	part("text/html", html)
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}

// qpBuf lets the quoted-printable writer append to the message being built.
type qpBuf struct{ b *strings.Builder }

func (q *qpBuf) Write(p []byte) (int, error) { return q.b.Write(p) }

// loginAuth tries PLAIN, which nearly every server accepts; it refuses to send a password over an unencrypted link.
type loginAuth struct {
	user, pass, host string
	secure           bool
}

func (a loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !a.secure && !server.TLS && a.host != "localhost" && a.host != "127.0.0.1" {
		return "", nil, errors.New("refusing to send a password without encryption")
	}
	return "PLAIN", []byte("\x00" + a.user + "\x00" + a.pass), nil
}

func (a loginAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("unexpected reply from the mail server")
	}
	return nil, nil
}
