// Package demo turns a request into a running demo instance and removes it when its time is up.
package demo

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/config"
	"github.com/redefynelabs/erp-control-plane/internal/entitle"
	cpmail "github.com/redefynelabs/erp-control-plane/internal/mail"
	"github.com/redefynelabs/erp-control-plane/internal/notify"
	"github.com/redefynelabs/erp-control-plane/internal/provision"
	"github.com/redefynelabs/erp-control-plane/internal/store"
)

// Errors a visitor can be told about.
var (
	ErrInvalid     = errors.New("invalid request")
	ErrRateLimited = errors.New("too many demo requests")
	ErrAtCapacity  = errors.New("no demo capacity right now")
)

// ValidationError names the fields that were wrong.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "invalid request" }
func (e *ValidationError) Unwrap() error { return ErrInvalid }

// Request is what a visitor submits.
type Request struct {
	Name, Email, Phone, Company string
	// TeamSize and Focus are optional: how big the visitor's team is and what they want to see first (see TeamSizes and
	// Focuses). They are kept on the lead's history so a follow-up can start from them.
	TeamSize, Focus string
	// Modules are the app groups the visitor wants to see (see ModuleGroups). The demo is built with only these; empty
	// means everything.
	Modules []string
	// Edition is services or goods. Empty is goods when the visitor picked inventory without projects, else services.
	Edition string
	IP      string
	// Attribution is where the visitor came from (utm_* values, referrer, landing page), kept on the lead's history.
	Attribution map[string]string
}

// Ready is what the visitor gets back.
type Ready struct {
	URL       string    `json:"url"`
	Email     string    `json:"login_email"`
	Password  string    `json:"login_password"`
	ExpiresAt time.Time `json:"expires_at"`

	// Queued is true when every slot was busy: the visitor's request waits in line (Position, 1 being next) and the
	// demo is started, and its login mailed, when a slot is free. The other fields are then empty.
	Queued   bool `json:"-"`
	Position int  `json:"-"`
}

// Mailer sends the visitor their login when a queued demo starts. Without one nobody can be queued.
type Mailer interface {
	Ready(ctx context.Context) bool
	Queue(ctx context.Context, leadID, sentBy, to, subject, body string, after time.Time) (store.Email, error)
	QueueRendered(ctx context.Context, leadID, sentBy, to string, r cpmail.Rendered, after time.Time) (store.Email, error)
	// ComposeTemplate renders the editable template with this key; it fails with cpmail.ErrNoTemplate when there is none.
	ComposeTemplate(ctx context.Context, key string, vars map[string]string) (cpmail.Rendered, error)
}

// Intake is told about arriving leads and ended demos so the sales team's follow-up can start. Nil means nothing happens.
type Intake interface {
	NewLead(ctx context.Context, l store.Lead, created, wrote bool)
	DemoEnded(ctx context.Context, l store.Lead)
}

// Entitler issues the entitlement a demo is given. Nil means demos get none.
type Entitler interface {
	ForDemo(ctx context.Context, installationID, template string, until time.Time, apps, features []string) error
}

// Service provisions and removes demos.
type Service struct {
	intake   Intake
	entitler Entitler
	mailer   Mailer
	cfg      config.Demo
	store    *store.Store
	dbs      provision.Databases
	stacks   provision.Stacks
	log      *slog.Logger
	notify   notify.Notifier
	now      func() time.Time
	// ready is how long to wait for a new instance to answer.
	ready time.Duration
	// inflight lets one demo per email provision at a time.
	mu       sync.Mutex
	inflight map[string]bool
	// dispatchMu lets one dispatch pass run at a time.
	dispatchMu sync.Mutex
}

// New returns the service.
func New(cfg config.Demo, st *store.Store, dbs provision.Databases, stacks provision.Stacks, log *slog.Logger) *Service {
	return &Service{cfg: cfg, store: st, dbs: dbs, stacks: stacks, log: log, notify: notify.Nop{}, now: time.Now, ready: 90 * time.Second, inflight: map[string]bool{}}
}

// WithIntake sets who starts the follow-up for new leads and ended demos.
func (s *Service) WithIntake(i Intake) *Service { s.intake = i; return s }

// WithEntitlements sets who issues each demo's entitlement.
func (s *Service) WithEntitlements(e Entitler) *Service { s.entitler = e; return s }

// WithMailer sets who mails a queued visitor their login. Without it a full house turns visitors away.
func (s *Service) WithMailer(m Mailer) *Service { s.mailer = m; return s }

// WithNotifier sets who is told about new leads and demos.
func (s *Service) WithNotifier(n notify.Notifier) *Service { s.notify = n; return s }

// Validate checks a request and returns what is wrong with it, by field.
func (r Request) Validate() error {
	f := map[string]string{}
	if n := strings.TrimSpace(r.Name); n == "" || len(n) > 120 {
		f["name"] = "Enter your name"
	}
	if a, err := mail.ParseAddress(strings.TrimSpace(r.Email)); err != nil || a.Address != strings.TrimSpace(r.Email) || len(r.Email) > 254 {
		f["email"] = "Enter a valid email address"
	}
	if !validPhone(r.Phone) {
		f["phone"] = "Enter a valid phone number"
	}
	if len(r.Company) > 160 {
		f["company"] = "Too long"
	}
	if r.TeamSize != "" && !slices.Contains(TeamSizes, r.TeamSize) {
		f["team_size"] = "Choose one of the options"
	}
	if r.Focus != "" && !slices.Contains(Focuses, r.Focus) {
		f["focus"] = "Choose one of the options"
	}
	for _, m := range r.Modules {
		if _, ok := ModuleGroups[m]; !ok {
			f["modules"] = "Choose from the apps listed"
		}
	}
	if r.Edition != "" && r.Edition != entitle.Services && r.Edition != entitle.Goods {
		f["edition"] = "Choose services or goods"
	}
	if len(f) > 0 {
		return &ValidationError{Fields: f}
	}
	return nil
}

// ModuleGroups maps what the website lets a visitor pick to the catalog's onboarding groups it switches on.
var ModuleGroups = map[string][]string{
	"sales":     {"crm", "finance"},
	"finance":   {"finance"},
	"projects":  {"projects"},
	"people":    {"people"},
	"payroll":   {"payroll", "people"},
	"inventory": {"inventory"},
}

// editionOf is the edition a demo shows: the one asked for, or goods when the visitor picked inventory and not projects.
func editionOf(asked string, modules []string) string {
	if asked != "" {
		return asked
	}
	if slices.Contains(modules, "inventory") && !slices.Contains(modules, "projects") {
		return entitle.Goods
	}
	return entitle.Services
}

// ofEdition keeps the picks that are part of the edition, so a demo never mixes the two products. A goods demo with
// nothing picked shows everything of goods.
func ofEdition(edition string, modules []string) []string {
	other := "inventory"
	if edition == entitle.Goods {
		other = "projects"
		if len(modules) == 0 {
			for m := range ModuleGroups {
				modules = append(modules, m)
			}
			slices.Sort(modules)
		}
	}
	return slices.DeleteFunc(slices.Clone(modules), func(m string) bool { return m == other })
}

// selection turns the visitor's picks into the apps to install, the features to switch off among those apps, and the
// apps and features to put in the entitlement. Nil everywhere means everything.
func selection(modules []string) (installed, disabled, apps, features []string) {
	if len(modules) == 0 {
		return nil, nil, nil, nil
	}
	var groups []string
	for _, m := range modules {
		groups = append(groups, ModuleGroups[m]...)
	}
	// Accounting's add-ons of the demo's edition come with Accounting.
	edition := editionOf("", modules)
	if slices.Contains(groups, "finance") {
		if edition == entitle.Goods {
			groups = append(groups, "goodsbilling")
		} else {
			groups = append(groups, "projectbilling")
		}
	}
	apps, features = entitle.Select(groups)
	for _, a := range entitle.Catalog.Apps {
		if !slices.Contains(apps, a.Name) {
			continue
		}
		for _, f := range a.Features {
			if len(f.Editions) > 0 && !slices.Contains(f.Editions, edition) {
				continue // the entitlement leaves it out; it is not part of this demo's product
			}
			if !slices.Contains(features, f.Name) {
				disabled = append(disabled, f.Name)
			}
		}
	}
	return apps, disabled, apps, features
}

// TeamSizes, Focuses and Timelines are the choices the website offers; empty means "not said".
var (
	TeamSizes = []string{"1-10", "11-25", "26-100", "101-300", "300+"}
	Focuses   = []string{"people", "projects", "crm", "finance", "mei"}
	Timelines = []string{"now", "month", "quarter", "exploring"}
)

const slugAlphabet = "abcdefghijklmnopqrstuvwxyz234567"

func newSlug() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToLower(base32.NewEncoding(slugAlphabet).WithPadding(base32.NoPadding).EncodeToString(b)), nil // 8 characters
}

func secret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// passwordAlphabet leaves out look-alikes (0 O 1 l I) because the visitor reads the password off the screen.
const passwordAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// loginPassword is a password made for one demo: 14 characters, about 80 bits.
func loginPassword() (string, error) {
	b := make([]byte, 14)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = passwordAlphabet[int(b[i])%len(passwordAlphabet)]
	}
	return string(b), nil
}

func dbName(slug string) string { return "demo_" + slug }

// Request creates a demo for the visitor and returns where to open it. It checks limits first, records the lead, then
// provisions; if provisioning fails everything it made is removed.
func (s *Service) Request(ctx context.Context, r Request) (Ready, error) {
	var zero Ready
	if err := r.Validate(); err != nil {
		return zero, err
	}
	r.Edition = editionOf(r.Edition, r.Modules)
	r.Modules = ofEdition(r.Edition, r.Modules)
	email := strings.ToLower(strings.TrimSpace(r.Email))

	s.mu.Lock()
	if s.inflight[email] {
		s.mu.Unlock()
		return zero, ErrRateLimited
	}
	s.inflight[email] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.inflight, email); s.mu.Unlock() }()

	since := s.now().Add(-24 * time.Hour)
	if n, err := s.store.CountDemosByEmail(ctx, email, since); err != nil {
		return zero, err
	} else if n >= s.cfg.MaxPerEmail {
		return zero, ErrRateLimited
	}
	if r.IP != "" {
		if n, err := s.store.CountDemosByIP(ctx, r.IP, since); err != nil {
			return zero, err
		} else if n >= s.cfg.MaxPerIP {
			return zero, ErrRateLimited
		}
	}
	live, err := s.store.CountLive(ctx)
	if err != nil {
		return zero, err
	}
	waiting, err := s.store.CountQueued(ctx)
	if err != nil {
		return zero, err
	}
	// Nobody jumps the line: while others wait, a newcomer joins the back even if a slot has just opened.
	if live >= s.cfg.MaxLive || waiting > 0 {
		return s.enqueue(ctx, r, email, waiting)
	}

	lead, created, err := s.store.UpsertLead(ctx, store.LeadInput{Name: strings.TrimSpace(r.Name), Email: email, Phone: strings.TrimSpace(r.Phone), Company: strings.TrimSpace(r.Company), Source: "website", TeamSize: r.TeamSize})
	if err != nil {
		return zero, err
	}
	slug, err := newSlug()
	if err != nil {
		return zero, err
	}
	expires := s.now().Add(s.cfg.TTL)
	d, err := s.store.CreateDemo(ctx, lead.ID, slug, r.IP, expires)
	if err != nil {
		return zero, err
	}

	// Each demo signs its visitor in as themselves, with a password made for this demo alone.
	password, err := loginPassword()
	if err != nil {
		return zero, err
	}
	url, err := s.provision(ctx, d, expires, email, password, r.Modules)
	if err != nil {
		s.log.Error("demo provisioning failed", "slug", slug, "error", err)
		_ = s.store.MarkDemoFailed(context.WithoutCancel(ctx), d.ID, err.Error())
		s.teardown(context.WithoutCancel(ctx), d) // do not wait for the reaper
		return zero, fmt.Errorf("could not start the demo: %w", err)
	}
	if err := s.store.MarkDemoActive(ctx, d.ID, url); err != nil {
		return zero, err
	}
	s.issueEntitlement(ctx, d, lead, expires, r.Modules)
	_ = s.store.SetLeadStatus(ctx, lead.ID, store.LeadDemoActive, store.LeadNew, store.LeadDemoEnded)
	if s.intake != nil {
		s.intake.NewLead(ctx, lead, created, false)
	}
	_ = s.store.AddEvent(ctx, lead.ID, "demo_started", map[string]any{"slug": slug, "expires_at": expires, "new_lead": created, "team_size": r.TeamSize, "focus": r.Focus, "modules": r.Modules, "attribution": CleanAttribution(r.Attribution)})
	s.log.Info("demo started", "slug", slug, "expires_at", expires)
	s.notify.Send(ctx, fmt.Sprintf("Demo started: %s <%s>%s", lead.Name, lead.Email, companySuffix(lead.Company)))
	return Ready{URL: url, Email: email, Password: password, ExpiresAt: expires}, nil
}

func (s *Service) provision(ctx context.Context, d store.Demo, expires time.Time, adminEmail, adminPassword string, modules []string) (string, error) {
	db := dbName(d.Slug)
	if err := s.dbs.Clone(ctx, s.cfg.Template, db, s.cfg.PGUser); err != nil {
		return "", fmt.Errorf("database: %w", err)
	}
	// The copy shows only the apps the visitor asked to see.
	if installed, disabled, _, _ := selection(modules); installed != nil {
		if err := s.dbs.SetModules(ctx, db, installed, disabled); err != nil {
			return "", fmt.Errorf("modules: %w", err)
		}
	}
	jwt, err := secret(48)
	if err != nil {
		return "", err
	}
	secrets, err := secret(48)
	if err != nil {
		return "", err
	}
	host := d.Slug + "." + s.cfg.Domain
	url := s.cfg.Scheme + "://" + host
	env := map[string]string{
		"APP_ENV": "production", "APP_PORT": "8080",
		"DEPLOYMENT_MODE": "demo", "DEMO_EXPIRES_AT": expires.UTC().Format(time.RFC3339),
		"DEMO_ADMIN_EMAIL": adminEmail, "DEMO_ADMIN_PASSWORD": adminPassword,
		"DATABASE_URL": fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", s.cfg.PGUser, s.cfg.PGPassword, s.cfg.PGHost, db),
		"REDIS_URL":    s.cfg.RedisURL, "REDIS_PREFIX": "demo-" + d.Slug,
		"JWT_SECRET": jwt, "SECRETS_KEY": secrets,
		"PUBLIC_BASE_URL": url, "WEB_BASE_URL": url,
		"COOKIE_SECURE": fmt.Sprint(s.cfg.Scheme == "https"),
		// Only the web container, on this instance's private network, reaches the API.
		"TRUSTED_PROXIES":  "172.16.0.0/12,10.0.0.0/8,192.168.0.0/16",
		"MIGRATE_ON_START": "true", "LOG_LEVEL": "info",
		// Many demos share one Postgres (100 connections by default): a demo needs only a few.
		"DB_MAX_OPEN_CONNS": "6", "DB_MAX_IDLE_CONNS": "2",
	}
	if s.cfg.AIKey != "" {
		// Mei in the demo answers with our own key, for a limited number of questions.
		env["DEMO_AI_PROVIDER"], env["DEMO_AI_MODEL"], env["DEMO_AI_API_KEY"] = s.cfg.AIProvider, s.cfg.AIModel, s.cfg.AIKey
		env["DEMO_AI_MAX_QUESTIONS"] = fmt.Sprint(s.cfg.AIMaxQuestions)
	}
	spec := provision.StackSpec{Slug: d.Slug, ServerImage: s.cfg.ServerImage, WebImage: s.cfg.WebImage, ServerEnv: env, Host: host, Memory: s.cfg.Memory, CPUs: s.cfg.CPUs}
	if err := s.stacks.Start(ctx, spec); err != nil {
		return "", fmt.Errorf("containers: %w", err)
	}
	if err := s.stacks.WaitReady(ctx, d.Slug, s.ready); err != nil {
		return "", err
	}
	return url, nil
}

// teardown removes a demo's containers and database. It reports nothing: callers retry through the reaper.
func (s *Service) teardown(ctx context.Context, d store.Demo) error {
	return errors.Join(s.stacks.Stop(ctx, d.Slug), s.dbs.Drop(ctx, dbName(d.Slug)))
}

// End removes a demo now, whatever its state.
func (s *Service) End(ctx context.Context, id string) error {
	d, err := s.store.GetDemo(ctx, id)
	if err != nil {
		return err
	}
	return s.finish(ctx, d)
}

func (s *Service) finish(ctx context.Context, d store.Demo) error {
	if d.Status == store.DemoQueued {
		// Never had an instance: cancel the place in line.
		if ok, err := s.store.AbandonQueued(ctx, d.ID, "cancelled"); err != nil || !ok {
			return err
		}
		_ = s.store.AddEvent(ctx, d.LeadID, "demo_cancelled", map[string]any{"slug": d.Slug})
		return nil
	}
	if err := s.teardown(ctx, d); err != nil {
		return err
	}
	if d.Status == store.DemoExpired {
		return nil
	}
	if err := s.store.MarkDemoExpired(ctx, d.ID); err != nil {
		return err
	}
	// A visitor whose demo ran out moves on to "demo ended" unless someone has already worked the lead.
	_ = s.store.SetLeadStatus(ctx, d.LeadID, store.LeadDemoEnded, store.LeadDemoActive)
	_ = s.store.AddEvent(ctx, d.LeadID, "demo_ended", map[string]any{"slug": d.Slug})
	if s.intake != nil {
		if l, err := s.store.GetLead(ctx, d.LeadID); err == nil {
			s.intake.DemoEnded(ctx, l)
		}
	}
	s.log.Info("demo removed", "slug", d.Slug)
	return nil
}

// Extend gives an active demo more time, up to a day in total.
func (s *Service) Extend(ctx context.Context, id string, by time.Duration) (store.Demo, error) {
	d, err := s.store.GetDemo(ctx, id)
	if err != nil {
		return d, err
	}
	until := d.ExpiresAt.Add(by)
	if max := d.CreatedAt.Add(24 * time.Hour); until.After(max) {
		until = max
	}
	return s.store.ExtendDemo(ctx, id, until)
}

func companySuffix(company string) string {
	if company == "" {
		return ""
	}
	return " from " + company
}

// attributionKeys are the only attribution values kept; anything else a browser sends is dropped.
var attributionKeys = []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "referrer", "landing"}

// CleanAttribution keeps the known keys, each cut to 200 characters.
func CleanAttribution(in map[string]string) map[string]string {
	out := map[string]string{}
	for _, k := range attributionKeys {
		if v := strings.TrimSpace(in[k]); v != "" {
			if len(v) > 200 {
				v = v[:200]
			}
			out[k] = v
		}
	}
	return out
}

// validPhone accepts the ways people write a number: an optional +, then digits with spaces, dots, dashes or brackets,
// seven to fifteen digits in all (the E.164 limit). A contact number is required, so empty is not valid.
func validPhone(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" || len(p) > 40 {
		return false
	}
	digits := 0
	for i, c := range p {
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == '+' && i == 0, c == ' ', c == '-', c == '.', c == '(', c == ')':
		default:
			return false
		}
	}
	return digits >= 7 && digits <= 15
}

// issueEntitlement records the demo as an installation and signs it an entitlement for the demo template. A failure is
// logged and does not stop the demo: the demo runs on the copied database either way.
func (s *Service) issueEntitlement(ctx context.Context, d store.Demo, lead store.Lead, expires time.Time, modules []string) {
	if s.entitler == nil {
		return
	}
	name := "Demo: " + lead.Name
	if lead.Company != "" {
		name = "Demo: " + lead.Company
	}
	inst, err := s.store.CreateInstallation(ctx, name, "demo", lead.ID, d.ID, "")
	if err == nil {
		_, _, apps, features := selection(modules)
		// The picks were narrowed to one edition; its template follows.
		template := s.cfg.EntitlementTemplate
		if editionOf("", modules) == entitle.Goods && len(modules) > 0 {
			template = s.cfg.GoodsTemplate
		}
		err = s.entitler.ForDemo(ctx, inst.ID, template, expires, apps, features)
	}
	if err != nil {
		s.log.Error("could not issue the demo's entitlement", "slug", d.Slug, "error", err)
	}
}
