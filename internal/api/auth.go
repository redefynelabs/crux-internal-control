package api

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/redefynelabs/erp-control-plane/internal/store"
)

const (
	sessionTTL = 12 * time.Hour
	inviteTTL  = 7 * 24 * time.Hour
	minPass    = 10
)

// principal is who is calling: a signed-in person and the session token they used.
type principal struct {
	User  store.User
	Token string
}

func (p principal) actor() store.Actor { return store.Actor{ID: p.User.ID, Name: p.User.Name} }

type ctxKey struct{}

func who(r *http.Request) principal {
	p, _ := r.Context().Value(ctxKey{}).(principal)
	return p
}

// authenticate reads the bearer token, which must be a live session of an active person.
func (a *API) authenticate(r *http.Request) (principal, bool) {
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if got == "" {
		return principal{}, false
	}
	u, err := a.store.UserBySession(r.Context(), got)
	if err != nil {
		return principal{}, false
	}
	return principal{User: u, Token: got}, true
}

// need wraps a handler so only signed-in people with one of the roles reach it. With no roles, any signed-in person does.
func (a *API) need(h http.HandlerFunc, roles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := a.authenticate(r)
		if !ok {
			writeError(w, 401, "unauthorized", "Sign in to continue")
			return
		}
		if len(roles) > 0 && !contains(roles, p.User.Role) {
			writeError(w, 403, "forbidden", "You do not have access to this")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)))
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// loginGuard slows password guessing: after a handful of failures for an address or an email, sign-in is refused for a
// while. It lives in memory, which is enough for one control plane process.
type loginGuard struct {
	mu    sync.Mutex
	fails map[string][]time.Time
}

func newLoginGuard() *loginGuard { return &loginGuard{fails: map[string][]time.Time{}} }

const (
	guardWindow = 15 * time.Minute
	guardMax    = 8
)

func (g *loginGuard) blocked(keys ...string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	cut := time.Now().Add(-guardWindow)
	for _, k := range keys {
		recent := g.fails[k][:0]
		for _, t := range g.fails[k] {
			if t.After(cut) {
				recent = append(recent, t)
			}
		}
		if len(recent) == 0 {
			delete(g.fails, k)
		} else {
			g.fails[k] = recent
		}
		if len(recent) >= guardMax {
			return true
		}
	}
	return false
}

func (g *loginGuard) fail(keys ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, k := range keys {
		g.fails[k] = append(g.fails[k], time.Now())
	}
}

// dummyHash makes an unknown email take as long to reject as a wrong password.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

type loginJSON struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in loginJSON
	if !decode(w, r, &in) {
		return
	}
	ip, email := "ip:"+a.clientIP(r), "email:"+strings.ToLower(strings.TrimSpace(in.Email))
	if a.logins.blocked(ip, email) {
		writeError(w, 429, "rate_limited", "Too many attempts. Try again in a few minutes")
		return
	}
	u, hash, err := a.store.Credentials(r.Context(), in.Email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		a.internal(w, err)
		return
	}
	known := err == nil && hash != "" && u.Active
	if !known {
		hash = string(dummyHash)
	}
	okPass := bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) == nil
	if !known || !okPass {
		a.logins.fail(ip, email)
		writeError(w, 401, "invalid_credentials", "That email or password is not right")
		return
	}
	tok, err := a.store.CreateSession(r.Context(), u.ID, sessionTTL)
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok, "user": u})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	p := who(r)
	_ = a.store.DeleteSession(r.Context(), p.Token)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, who(r).User)
}

type passwordJSON struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	p := who(r)
	var in passwordJSON
	if !decode(w, r, &in) {
		return
	}
	if len(in.New) < minPass {
		writeError(w, 422, "validation_failed", "Use at least 10 characters", map[string]string{"new": "At least 10 characters"})
		return
	}
	hash, err := a.store.PasswordHash(r.Context(), p.User.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Current)) != nil {
		writeError(w, 422, "validation_failed", "Your current password is not right", map[string]string{"current": "Not right"})
		return
	}
	nh, err := bcrypt.GenerateFromPassword([]byte(in.New), bcrypt.DefaultCost)
	if err != nil {
		a.internal(w, err)
		return
	}
	if err := a.store.SetPassword(r.Context(), p.User.ID, string(nh)); err != nil {
		a.internal(w, err)
		return
	}
	_ = a.store.DeleteOtherSessions(r.Context(), p.User.ID, p.Token)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) inviteInfo(w http.ResponseWriter, r *http.Request) {
	u, err := a.store.UserByInvite(r.Context(), r.PathValue("token"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "not_found", "This invitation is not valid any more")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"email": u.Email, "name": u.Name})
}

type acceptJSON struct {
	Token    string `json:"token"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (a *API) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var in acceptJSON
	if !decode(w, r, &in) {
		return
	}
	if len(in.Password) < minPass {
		writeError(w, 422, "validation_failed", "Use at least 10 characters", map[string]string{"password": "At least 10 characters"})
		return
	}
	u, err := a.store.UserByInvite(r.Context(), in.Token)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "not_found", "This invitation is not valid any more")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		a.internal(w, err)
		return
	}
	if err := a.store.SetPassword(r.Context(), u.ID, string(hash)); err != nil {
		a.internal(w, err)
		return
	}
	if n := strings.TrimSpace(in.Name); n != "" && len(n) <= 120 {
		_, _ = a.store.UpdateUser(r.Context(), u.ID, store.UserPatch{Name: &n})
	}
	tok, err := a.store.CreateSession(r.Context(), u.ID, sessionTTL)
	if err != nil {
		a.internal(w, err)
		return
	}
	u, _ = a.store.GetUser(r.Context(), u.ID)
	writeJSON(w, 200, map[string]any{"token": tok, "user": u})
}

func validEmail(e string) bool {
	a, err := mail.ParseAddress(strings.TrimSpace(e))
	return err == nil && a.Address == strings.TrimSpace(e) && len(e) <= 254
}
