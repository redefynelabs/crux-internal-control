package api

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/redefynelabs/erp-control-plane/internal/config"
	"github.com/redefynelabs/erp-control-plane/internal/entitle"
	"github.com/redefynelabs/erp-control-plane/internal/store"
)

//go:embed install.sh
var installScript string

func (a *API) installRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /install", a.serveInstaller)
	mux.HandleFunc("GET /v1/install/ip", a.installIP)
	mux.HandleFunc("POST /v1/install/start", a.installStart)
}

// publicURL is this service's address as customers reach it: the configured one, or else the one this request came to.
func (a *API) publicURL(r *http.Request) string {
	if a.cfg.Install.PublicURL != "" {
		return a.cfg.Install.PublicURL
	}
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// serveInstaller is the script a customer pipes into bash, with this service's address filled in. It holds no secret.
func (a *API) serveInstaller(w http.ResponseWriter, r *http.Request) {
	url := a.publicURL(r)
	if !config.ShellSafe(strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")) {
		writeError(w, 400, "bad_request", "The address this was reached on is not usable in an installer")
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(strings.ReplaceAll(installScript, "__CP_URL__", url)))
}

// installIP tells the installer which address this request came from, so it can check the domain points at the server
// without asking a third party.
func (a *API) installIP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprint(w, a.clientIP(r))
}

type installStartIn struct {
	Code string `json:"code"`
}

type installStartOut struct {
	Registry      string `json:"registry"`
	RegistryUser  string `json:"registry_user"`
	RegistryToken string `json:"registry_token"`
	// Version is the release tag to install, such as v1.4.2.
	Version  string `json:"version"`
	Customer string `json:"customer"`
}

// installStart tells an installer what to download: the registry (and its read-only login) and the newest release the
// customer's maintenance covers. The code must be valid but is not used up; the customer's setup does that.
func (a *API) installStart(w http.ResponseWriter, r *http.Request) {
	in := installStartIn{}
	if !decode(w, r, &in) {
		return
	}
	if a.cfg.Install.Registry == "" {
		writeError(w, 503, "installs_unavailable", "Installing from here is not set up on this control plane")
		return
	}
	ip := "install:" + a.clientIP(r)
	if a.logins.blocked(ip) {
		writeError(w, 429, "too_many_attempts", "Too many attempts. Try again in a few minutes")
		return
	}
	bad := func() {
		a.logins.fail(ip)
		writeError(w, 422, "invalid_code", "That code is not valid, was already used, or has expired. Ask your provider for a new one")
	}
	hash, err := entitle.HashActivationCode(in.Code)
	if err != nil {
		bad()
		return
	}
	id, err := a.store.PeekActivation(r.Context(), hash)
	if errors.Is(err, store.ErrNotFound) {
		bad()
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	inst, err := a.store.GetInstallation(r.Context(), id)
	if err != nil {
		a.internal(w, err)
		return
	}
	if inst.Status == "revoked" {
		bad()
		return
	}
	releases, err := a.store.ListReleases(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	var payload *entitle.Payload
	if a.issuer != nil {
		if p, ok, err := a.issuer.Latest(r.Context(), id); err != nil {
			a.internal(w, err)
			return
		} else if ok {
			payload = &p
		}
	}
	// Nothing is running yet, so every release the maintenance covers is a candidate and the newest is taken.
	offered := EligibleReleases(releases, payload, "")
	if len(offered) == 0 || !config.ShellSafe(offered[0].Version) {
		writeError(w, 409, "no_release", "No version has been published for this customer yet")
		return
	}
	out := installStartOut{
		Registry: a.cfg.Install.Registry, RegistryUser: a.cfg.Install.User, RegistryToken: a.cfg.Install.Token,
		Version: "v" + strings.TrimPrefix(offered[0].Version, "v"), Customer: inst.Name,
	}
	if err := a.store.Audit(r.Context(), "", "", id, "install_started", map[string]any{"version": out.Version, "ip": a.clientIP(r)}); err != nil {
		a.log.Error("audit entry not written", "action", "install_started", "error", err)
	}
	if strings.Contains(r.Header.Get("Accept"), "text/x-shellvars") {
		w.Header().Set("Content-Type", "text/x-shellvars; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, "REGISTRY='%s'\nREGISTRY_USER='%s'\nREGISTRY_TOKEN='%s'\nVERSION='%s'\nCUSTOMER='%s'\n",
			out.Registry, out.RegistryUser, out.RegistryToken, out.Version, shellText(out.Customer))
		return
	}
	writeJSON(w, 200, out)
}

// shellText makes a customer's name safe inside single quotes of the installer's answer: anything else is dropped.
func shellText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ', r == '.', r == '-', r == '_', r == '&':
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
