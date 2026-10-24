// Package api is the control plane's HTTP surface: one public endpoint for demo requests, and the back-office API.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/config"
	"github.com/redefynelabs/erp-control-plane/internal/demo"
	"github.com/redefynelabs/erp-control-plane/internal/entitle"
	"github.com/redefynelabs/erp-control-plane/internal/mail"
	"github.com/redefynelabs/erp-control-plane/internal/sales"
	"github.com/redefynelabs/erp-control-plane/internal/store"
)

// Captcha verifies a token from the visitor's browser. Nil means no captcha is required.
type Captcha interface {
	Verify(ctx context.Context, token, ip string) (bool, error)
}

// API serves the endpoints.
type API struct {
	cfg     *config.Config
	store   *store.Store
	demos   *demo.Service
	log     *slog.Logger
	captcha Captcha
	mail    *mail.Service
	sales   *sales.Sales
	logins  *loginGuard
	issuer  *entitle.Issuer
}

// Option adds an optional part to the API.
type Option func(*API)

// WithEntitlements sets who signs and keeps entitlements. Without it the entitlement endpoints answer 503.
func WithEntitlements(i *entitle.Issuer) Option { return func(a *API) { a.issuer = i } }

// New returns the handler tree.
func New(cfg *config.Config, st *store.Store, demos *demo.Service, mailer *mail.Service, sl *sales.Sales, captcha Captcha, log *slog.Logger, opts ...Option) http.Handler {
	a := &API{cfg: cfg, store: st, demos: demos, log: log, captcha: captcha, mail: mailer, sales: sl, logins: newLoginGuard()}
	for _, o := range opts {
		o(a)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /v1/brand/logo.png", a.brandLogo)
	mux.HandleFunc("POST /v1/demo-requests", a.requestDemo)
	mux.HandleFunc("POST /v1/contact-requests", a.requestContact)

	a.routes(mux)
	return a.cors(limitBody(mux))
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		next.ServeHTTP(w, r)
	})
}

// cors lets the marketing site call the public endpoint from a browser; the back-office API is never opened to a
// browser origin.
func (a *API) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (r.URL.Path == "/v1/demo-requests" || r.URL.Path == "/v1/contact-requests") && slices.Contains(a.cfg.AllowedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func writeError(w http.ResponseWriter, status int, code, msg string, fields ...map[string]string) {
	body := map[string]any{"code": code, "message": msg}
	if len(fields) > 0 {
		body["fields"] = fields[0]
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": body})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, 400, "bad_request", "The request body is not valid JSON")
		return false
	}
	return true
}

// clientIP is the visitor's address. Behind a proxy it is the first X-Forwarded-For entry the proxy added.
func (a *API) clientIP(r *http.Request) string {
	if a.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type demoRequestJSON struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Company string `json:"company"`
	// TeamSize and Focus are optional answers from the demo form.
	TeamSize string `json:"team_size"`
	Focus    string `json:"focus"`
	// Modules are the app groups the visitor wants in the demo; empty means all of them.
	Modules []string `json:"modules"`
	// Edition is services or goods; empty picks it from the modules.
	Edition string `json:"edition"`
	// Website is a honeypot: the form hides it, so only a bot fills it in.
	Website      string            `json:"website"`
	CaptchaToken string            `json:"captcha_token"`
	Attribution  map[string]string `json:"attribution"`
}

func (a *API) requestDemo(w http.ResponseWriter, r *http.Request) {
	var in demoRequestJSON
	if !decode(w, r, &in) {
		return
	}
	ip := a.clientIP(r)
	if in.Website != "" {
		// Look successful to a bot, and do nothing.
		writeJSON(w, 202, map[string]string{"status": "accepted"})
		return
	}
	if a.captcha != nil {
		ok, err := a.captcha.Verify(r.Context(), in.CaptchaToken, ip)
		if err != nil {
			a.log.Error("captcha check failed", "error", err)
			writeError(w, 503, "captcha_unavailable", "We could not check that you are human. Please try again")
			return
		}
		if !ok {
			writeError(w, 422, "captcha_failed", "Please complete the check and try again")
			return
		}
	}
	// A demo takes several seconds to start; do not let the visitor's browser dropping the connection abandon it half way.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Minute)
	defer cancel()
	ready, err := a.demos.Request(ctx, demo.Request{Name: in.Name, Email: in.Email, Phone: in.Phone, Company: in.Company, TeamSize: in.TeamSize, Focus: in.Focus, Modules: in.Modules, Edition: in.Edition, IP: ip, Attribution: in.Attribution})
	var ve *demo.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, 422, "validation_failed", "Please check the highlighted fields", ve.Fields)
	case errors.Is(err, demo.ErrRateLimited):
		writeError(w, 429, "rate_limited", "You have asked for a demo recently. Check your email, or try again tomorrow")
	case errors.Is(err, demo.ErrAtCapacity):
		writeError(w, 503, "at_capacity", "All our demo rooms are busy and the waiting list is full. Please try again in a few minutes")
	case err != nil:
		a.log.Error("demo request failed", "error", err)
		writeError(w, 500, "demo_failed", "We could not start your demo. Please try again in a moment")
	case ready.Queued:
		writeJSON(w, 202, map[string]any{"status": "queued", "queued": true, "position": ready.Position})
	default:
		writeJSON(w, 201, ready)
	}
}

func (a *API) internal(w http.ResponseWriter, err error) {
	a.log.Error("request failed", "error", err)
	writeError(w, 500, "internal", "Something went wrong")
}

// queryLimit reads ?limit=; zero (absent or invalid) lets the store apply its default and maximum.
func queryLimit(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (a *API) listDemos(w http.ResponseWriter, r *http.Request) {
	ds, err := a.store.ListDemos(r.Context(), r.URL.Query().Get("status"), queryLimit(r))
	if err != nil {
		a.internal(w, err)
		return
	}
	if ds == nil {
		ds = []store.Demo{}
	}
	writeJSON(w, 200, ds)
}

type extendJSON struct {
	Minutes int `json:"minutes"`
}

func (a *API) extendDemo(w http.ResponseWriter, r *http.Request) {
	var in extendJSON
	if !decode(w, r, &in) {
		return
	}
	if in.Minutes < 1 || in.Minutes > 24*60 {
		writeError(w, 422, "validation_failed", "Minutes must be between 1 and 1440", map[string]string{"minutes": "Between 1 and 1440"})
		return
	}
	d, err := a.demos.Extend(r.Context(), r.PathValue("id"), time.Duration(in.Minutes)*time.Minute)
	if errors.Is(err, store.ErrNotFound) || isBadID(err) {
		writeError(w, 404, "not_found", "No such active demo")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (a *API) endDemo(w http.ResponseWriter, r *http.Request) {
	err := a.demos.End(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) || isBadID(err) {
		writeError(w, 404, "not_found", "No such demo")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// isBadID is true for a path id that is not a UUID, which Postgres reports as an error rather than "no rows".
func isBadID(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid input syntax for type uuid")
}

// Turnstile verifies Cloudflare Turnstile tokens.
type Turnstile struct {
	Secret string
	HTTP   *http.Client
	URL    string
}

// Verify implements Captcha.
func (t Turnstile) Verify(ctx context.Context, token, ip string) (bool, error) {
	if token == "" {
		return false, nil
	}
	endpoint := t.URL
	if endpoint == "" {
		endpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	}
	client := t.HTTP
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	form := url.Values{"secret": {t.Secret}, "response": {token}, "remoteip": {ip}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return false, err
	}
	return out.Success, nil
}

type contactJSON struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Company  string `json:"company"`
	Interest string `json:"interest"`
	// Plan is the cloud plan they looked at on the pricing page.
	Plan string `json:"plan"`
	// TeamSize and Timeline are optional answers from the contact form.
	TeamSize string `json:"team_size"`
	Timeline string `json:"timeline"`
	Message  string `json:"message"`
	// Website is a honeypot, as on the demo form.
	Website      string            `json:"website"`
	CaptchaToken string            `json:"captcha_token"`
	Attribution  map[string]string `json:"attribution"`
}

func (a *API) requestContact(w http.ResponseWriter, r *http.Request) {
	var in contactJSON
	if !decode(w, r, &in) {
		return
	}
	ip := a.clientIP(r)
	if in.Website != "" {
		writeJSON(w, 202, map[string]string{"status": "accepted"})
		return
	}
	if a.captcha != nil {
		ok, err := a.captcha.Verify(r.Context(), in.CaptchaToken, ip)
		if err != nil {
			a.log.Error("captcha check failed", "error", err)
			writeError(w, 503, "captcha_unavailable", "We could not check that you are human. Please try again")
			return
		}
		if !ok {
			writeError(w, 422, "captcha_failed", "Please complete the check and try again")
			return
		}
	}
	err := a.demos.Contact(r.Context(), demo.Contact{Name: in.Name, Email: in.Email, Phone: in.Phone, Company: in.Company, Interest: in.Interest, Plan: in.Plan, TeamSize: in.TeamSize, Timeline: in.Timeline, Message: in.Message, IP: ip, Attribution: in.Attribution})
	var ve *demo.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, 422, "validation_failed", "Please check the highlighted fields", ve.Fields)
	case errors.Is(err, demo.ErrRateLimited):
		writeError(w, 429, "rate_limited", "You have sent several messages recently. Please try again later")
	case err != nil:
		a.internal(w, err)
	default:
		writeJSON(w, 201, map[string]string{"status": "received"})
	}
}
