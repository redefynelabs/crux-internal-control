package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/redefynelabs/erp-control-plane/internal/entitle"
	"github.com/redefynelabs/erp-control-plane/internal/store"
)

// Installation states a person reads in the back office. They are worked out, never typed in.
const (
	StateDemo        = "demo"
	StateRevoked     = "revoked"
	StateSuspended   = "suspended"
	StateAwaiting    = "awaiting_activation"
	StateExpired     = "expired"
	StateUnreachable = "unreachable"
	StateOutOfDate   = "out_of_date"
	StateActive      = "active"
)

// UnreachableAfter is how long without a check before an installation is called unreachable.
const UnreachableAfter = 24 * time.Hour

// AMCWarning is how far ahead a maintenance contract that is ending is flagged.
const AMCWarning = 60 * 24 * time.Hour

const activationLife = 7 * 24 * time.Hour

// DeriveState says where an installation stands, and why in a few words.
func DeriveState(i store.Installation, now time.Time) (state, text string) {
	switch {
	case i.Kind == "demo":
		return StateDemo, "Demo"
	case i.Status == "revoked":
		return StateRevoked, "The key was revoked"
	case i.Status == "suspended" || i.Suspended:
		return StateSuspended, "Suspended: read-only until resumed"
	case i.ActivatedAt == nil:
		return StateAwaiting, "Waiting for the customer to enter the activation code"
	case i.ExpiresAt != nil && now.After(*i.ExpiresAt):
		return StateExpired, "The subscription ended"
	case i.LastSeenAt == nil || now.Sub(*i.LastSeenAt) > UnreachableAfter:
		return StateUnreachable, "Has not checked in for over a day"
	case i.Version > 0 && (i.AppliedVersion == nil || *i.AppliedVersion < i.Version):
		return StateOutOfDate, "Has not applied the newest file yet"
	}
	return StateActive, "Working and up to date"
}

// AMC says where the maintenance contract stands: none, ok, ending (within 60 days) or lapsed.
func AMC(i store.Installation, now time.Time) string {
	switch {
	case i.UpdatesUntil == nil:
		return "none"
	case now.After(*i.UpdatesUntil):
		return "lapsed"
	case i.UpdatesUntil.Sub(now) <= AMCWarning:
		return "ending"
	}
	return "ok"
}

type installationView struct {
	store.Installation
	State     string `json:"state"`
	StateText string `json:"state_text"`
	AMC       string `json:"amc"`
}

func installationViewOf(i store.Installation, now time.Time) installationView {
	s, t := DeriveState(i, now)
	return installationView{Installation: i, State: s, StateText: t, AMC: AMC(i, now)}
}

// audit records what a person did. A failure is logged and does not undo the action.
func (a *API) audit(r *http.Request, installationID, action string, detail any) {
	p := who(r)
	if err := a.store.Audit(r.Context(), p.User.ID, p.User.Email, installationID, action, detail); err != nil {
		a.log.Error("audit entry not written", "action", action, "error", err)
	}
}

func (a *API) licensingRoutes(mux *http.ServeMux, staff, adminOnly func(http.HandlerFunc) http.HandlerFunc) {
	// Called by installations.
	mux.HandleFunc("POST /v1/activate", a.activate)
	mux.HandleFunc("POST /v1/sync", a.asInstallation(a.sync))

	mux.HandleFunc("POST /v1/installations/{id}/activation", adminOnly(a.makeActivation))
	mux.HandleFunc("POST /v1/installations/{id}/licence", adminOnly(a.setLicence))
	mux.HandleFunc("POST /v1/installations/{id}/suspend", adminOnly(a.suspend))
	mux.HandleFunc("POST /v1/installations/{id}/resume", adminOnly(a.resume))
	mux.HandleFunc("POST /v1/installations/{id}/revoke", adminOnly(a.revoke))
	mux.HandleFunc("GET /v1/releases", staff(a.listReleases))
	mux.HandleFunc("POST /v1/releases", adminOnly(a.publishRelease))
	mux.HandleFunc("POST /v1/releases/{id}/withdraw", adminOnly(a.withdrawRelease))
	mux.HandleFunc("GET /v1/audit", staff(a.listAudit))
}

// --- activation ---

type activationOut struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// makeActivation makes a one-time code for an installation. The code is in this answer only.
func (a *API) makeActivation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, err := a.store.GetInstallation(r.Context(), id)
	if a.notFoundOrFail(w, err) {
		return
	}
	if inst.Kind != "customer" {
		writeError(w, 409, "not_a_customer", "Only a customer installation is activated with a code")
		return
	}
	if inst.Version == 0 {
		writeError(w, 409, "no_entitlement", "Issue an entitlement first: the installation starts from it")
		return
	}
	code, hash, hint, err := entitle.NewActivationCode()
	if err != nil {
		a.internal(w, err)
		return
	}
	expires := time.Now().Add(activationLife).UTC()
	if _, err := a.store.CreateActivation(r.Context(), id, hash, hint, expires, who(r).User.ID); a.notFoundOrFail(w, err) {
		return
	}
	a.audit(r, id, "activation_code_created", map[string]any{"ends": hint, "expires_at": expires})
	writeJSON(w, 201, activationOut{Code: code, ExpiresAt: expires})
}

type activateIn struct {
	Code string `json:"code"`
}

type activateOut struct {
	InstallationID     string `json:"installation_id"`
	InstallationSecret string `json:"installation_secret"`
	PublicKey          string `json:"public_key"`
	// Entitlement is the signed file, as the raw bundle.
	Entitlement string `json:"entitlement"`
}

// activate exchanges a code for the installation's identity and its first file. It is open to the internet, so wrong
// guesses are counted per address and every failure looks the same.
func (a *API) activate(w http.ResponseWriter, r *http.Request) {
	if a.issuer == nil || a.issuer.Signer() == nil {
		writeError(w, 503, "entitlements_unavailable", "Activation is not set up on this control plane")
		return
	}
	ip := "activate:" + a.clientIP(r)
	if a.logins.blocked(ip) {
		writeError(w, 429, "too_many_attempts", "Too many attempts. Try again in a few minutes")
		return
	}
	var in activateIn
	if !decode(w, r, &in) {
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
	id, err := a.store.ConsumeActivation(r.Context(), hash)
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
	last, err := a.store.LatestIssued(r.Context(), id)
	if err != nil {
		a.internal(w, err)
		return
	}
	if err := a.store.RecordSeen(r.Context(), id, nil, "", ""); err != nil {
		a.log.Error("could not record activation", "installation", id, "error", err)
	}
	if err := a.store.Audit(r.Context(), "", "", id, "activated", map[string]any{"ip": a.clientIP(r)}); err != nil {
		a.log.Error("audit entry not written", "action", "activated", "error", err)
	}
	a.log.Info("installation activated", "installation", inst.Name)
	writeJSON(w, 200, activateOut{
		InstallationID:     inst.ID,
		InstallationSecret: entitle.Secret(a.cfg.SecretsKey, inst.ID, inst.SecretGeneration),
		PublicKey:          a.issuer.Signer().PublicKey(),
		Entitlement:        last.File,
	})
}

// --- sync: the installation's regular check ---

type syncIn struct {
	// AppliedVersion is the entitlement version in force on the installation, nil when it has none.
	AppliedVersion *int   `json:"applied_version"`
	ServerVersion  string `json:"server_version"`
	ServerBuild    string `json:"server_build"`
}

// offered is a release as an installation sees it.
type offered struct {
	Version        string            `json:"version"`
	Channel        string            `json:"channel"`
	PublishedAt    time.Time         `json:"published_at"`
	Notes          string            `json:"notes"`
	HasMigrations  bool              `json:"has_migrations"`
	MinFromVersion string            `json:"min_from_version"`
	Images         map[string]string `json:"images"`
	// Direct is false when the running version is older than MinFromVersion: an earlier release is needed first.
	Direct   bool   `json:"direct"`
	Manifest string `json:"manifest"`
}

type syncOut struct {
	// KeyStatus is active, suspended or revoked.
	KeyStatus  string `json:"key_status"`
	StatusNote string `json:"status_note"`
	Latest     int    `json:"latest_version"`
	// Entitlement is the newest file as the raw bundle, present only when the installation does not have it yet.
	Entitlement string    `json:"entitlement,omitempty"`
	Releases    []offered `json:"releases"`
	ServerTime  time.Time `json:"server_time"`
}

// EligibleReleases are the releases an installation may be offered: published, not withdrawn, on the stable or security
// channel, newer than what it runs, and published within its maintenance contract. With no contract date in the file the
// subscription end counts; with neither, every release.
func EligibleReleases(all []store.Release, p *entitle.Payload, running string) []offered {
	// A build that does not report a release version (a development build says "dev") is offered every eligible release.
	if !entitle.ValidVersion(running) {
		running = ""
	}
	var cutoff *time.Time
	if p != nil {
		cutoff = p.UpdatesUntil
		if cutoff == nil {
			cutoff = p.ExpiresAt
		}
	}
	var out []offered
	for _, rel := range all {
		if rel.WithdrawnAt != nil || (rel.Channel != "stable" && rel.Channel != "security") {
			continue
		}
		if cutoff != nil && rel.PublishedAt.After(*cutoff) {
			continue
		}
		if running != "" && entitle.CompareVersions(rel.Version, running) <= 0 {
			continue
		}
		out = append(out, offered{
			Version: rel.Version, Channel: rel.Channel, PublishedAt: rel.PublishedAt, Notes: rel.Notes, HasMigrations: rel.HasMigrations,
			MinFromVersion: rel.MinFromVersion, Images: rel.Images, Manifest: rel.Manifest,
			Direct: rel.MinFromVersion == "" || running == "" || entitle.CompareVersions(running, rel.MinFromVersion) >= 0,
		})
	}
	slices.SortFunc(out, func(x, y offered) int { return entitle.CompareVersions(y.Version, x.Version) })
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

func (a *API) sync(w http.ResponseWriter, r *http.Request) {
	var in syncIn
	if !decode(w, r, &in) {
		return
	}
	if len(in.ServerVersion) > 40 || len(in.ServerBuild) > 40 {
		writeError(w, 422, "validation_failed", "The version is too long")
		return
	}
	inst := installationOf(r)
	if err := a.store.RecordSeen(r.Context(), inst.ID, in.AppliedVersion, in.ServerVersion, in.ServerBuild); err != nil {
		a.internal(w, err)
		return
	}
	out := syncOut{KeyStatus: inst.Status, StatusNote: inst.StatusNote, Latest: inst.Version, Releases: []offered{}, ServerTime: time.Now().UTC()}
	if inst.Status == "revoked" {
		writeJSON(w, 200, out)
		return
	}
	last, err := a.store.LatestIssued(r.Context(), inst.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		a.internal(w, err)
		return
	}
	var payload *entitle.Payload
	if err == nil {
		payload = &entitle.Payload{}
		if err := jsonUnmarshal(last.Payload, payload); err != nil {
			a.internal(w, err)
			return
		}
		if in.AppliedVersion == nil || last.Version > *in.AppliedVersion {
			out.Entitlement = last.File
		}
	}
	rels, err := a.store.ListReleases(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	// A suspended installation is told so but is offered nothing new.
	if inst.Status == "active" {
		out.Releases = EligibleReleases(rels, payload, in.ServerVersion)
	}
	writeJSON(w, 200, out)
}

// --- licence terms and lifecycle ---

type licenceIn struct {
	License      string     `json:"license"`
	ExpiresAt    *time.Time `json:"expires_at"`
	UpdatesUntil *time.Time `json:"updates_until"`
	Note         string     `json:"note"`
}

// setLicence changes the licence terms (kind, subscription end, maintenance end) and signs a new file with everything else
// as it was. Renewing a subscription or the AMC, and selling a lifetime licence, all go through here.
func (a *API) setLicence(w http.ResponseWriter, r *http.Request) {
	if !a.needIssuer(w) {
		return
	}
	var in licenceIn
	if !decode(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	cur, ok, err := a.issuer.Latest(r.Context(), id)
	if err != nil {
		a.internal(w, err)
		return
	}
	if !ok {
		writeError(w, 409, "no_entitlement", "Issue an entitlement first")
		return
	}
	d := entitle.DraftOf(cur)
	d.License, d.ExpiresAt, d.UpdatesUntil = in.License, in.ExpiresAt, in.UpdatesUntil
	if in.License == entitle.LicenseLifetime {
		d.ExpiresAt = nil
	}
	issued, ok := a.issueDraft(w, r, id, d, nil)
	if !ok {
		return
	}
	a.audit(r, id, "licence_changed", map[string]any{"version": issued.Version, "license": in.License, "expires_at": in.ExpiresAt, "updates_until": in.UpdatesUntil, "note": strings.TrimSpace(in.Note)})
	writeJSON(w, 200, issued)
}

type noteIn struct {
	Note string `json:"note"`
}

func (a *API) lifecycle(w http.ResponseWriter, r *http.Request, status, action string, suspended bool) {
	if !a.needIssuer(w) {
		return
	}
	var in noteIn
	if !decode(w, r, &in) {
		return
	}
	note := strings.TrimSpace(in.Note)
	if len(note) > 500 || (status != "active" && note == "") {
		writeError(w, 422, "validation_failed", "Write a short reason", map[string]string{"note": "Between 1 and 500 characters"})
		return
	}
	id := r.PathValue("id")
	inst, err := a.store.GetInstallation(r.Context(), id)
	if a.notFoundOrFail(w, err) {
		return
	}
	if inst.Kind != "customer" {
		writeError(w, 409, "not_a_customer", "Only a customer installation can be suspended or revoked")
		return
	}
	if err := a.store.SetStatus(r.Context(), id, status, note); a.notFoundOrFail(w, err) {
		return
	}
	// The file carries the suspension, so an installation that cannot reach us still learns it from its next file.
	if cur, ok, err := a.issuer.Latest(r.Context(), id); err != nil {
		a.internal(w, err)
		return
	} else if ok && status != "revoked" {
		d := entitle.DraftOf(cur)
		d.Suspended = suspended
		if _, ok := a.issueDraft(w, r, id, d, nil); !ok {
			return
		}
	}
	a.audit(r, id, action, map[string]any{"note": note})
	got, err := a.store.GetInstallation(r.Context(), id)
	if a.notFoundOrFail(w, err) {
		return
	}
	writeJSON(w, 200, installationViewOf(got, time.Now()))
}

func (a *API) suspend(w http.ResponseWriter, r *http.Request) {
	a.lifecycle(w, r, "suspended", "suspended", true)
}
func (a *API) resume(w http.ResponseWriter, r *http.Request) {
	a.lifecycle(w, r, "active", "resumed", false)
}
func (a *API) revoke(w http.ResponseWriter, r *http.Request) {
	a.lifecycle(w, r, "revoked", "revoked", false)
}

// issueDraft signs a draft and writes the failure, if any. It reports whether the file was issued.
func (a *API) issueDraft(w http.ResponseWriter, r *http.Request, id string, d entitle.Draft, approve []string) (store.Issued, bool) {
	issued, err := a.issuer.Issue(r.Context(), id, d, who(r).User.ID, approve)
	var problem *entitle.Problem
	var pg *pgconn.PgError
	switch {
	case err == nil:
		return issued, true
	case errors.As(err, &problem):
		writeError(w, 422, "validation_failed", "Please check the highlighted fields", problem.Fields)
	case errors.Is(err, entitle.ErrNoSigner):
		writeError(w, 503, "no_signing_key", err.Error())
	case errors.As(err, &pg) && pg.Code == "23505":
		writeError(w, 409, "conflict", "Someone issued a file for this installation at the same moment. Reload and try again")
	case errors.As(err, &pg) && pg.Code == "22P02" && !isBadID(err):
		writeError(w, 422, "validation_failed", "A request id is not valid")
	default:
		if !a.notFoundOrFail(w, err) {
			a.internal(w, err)
		}
	}
	return store.Issued{}, false
}

// --- releases ---

func (a *API) listReleases(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListReleases(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, list)
}

type releaseIn struct {
	Version        string            `json:"version"`
	Channel        string            `json:"channel"`
	PublishedAt    *time.Time        `json:"published_at"`
	Notes          string            `json:"notes"`
	HasMigrations  bool              `json:"has_migrations"`
	MinFromVersion string            `json:"min_from_version"`
	Images         map[string]string `json:"images"`
}

func (in releaseIn) validate() map[string]string {
	f := map[string]string{}
	if !entitle.ValidVersion(in.Version) {
		f["version"] = "Use numbers separated by dots, like 1.4.2"
	}
	if in.Channel != "stable" && in.Channel != "beta" && in.Channel != "security" {
		f["channel"] = "Choose stable, beta or security"
	}
	if in.MinFromVersion != "" && !entitle.ValidVersion(in.MinFromVersion) {
		f["min_from_version"] = "Use numbers separated by dots, like 1.2.0"
	}
	if len(in.Notes) > 8000 {
		f["notes"] = "Too long"
	}
	for _, k := range []string{"server", "web"} {
		if v := in.Images[k]; !strings.Contains(v, "@sha256:") || len(v) > 300 {
			f["images"] = "Give the server and web images by digest, like registry/erp-server@sha256:…"
		}
	}
	for k, v := range in.Images {
		if k == "" || len(k) > 40 || v == "" || len(v) > 300 {
			f["images"] = "An image name or reference is not valid"
		}
	}
	return f
}

func (a *API) publishRelease(w http.ResponseWriter, r *http.Request) {
	if !a.needIssuer(w) {
		return
	}
	var in releaseIn
	if !decode(w, r, &in) {
		return
	}
	in.Version = strings.TrimPrefix(strings.TrimSpace(in.Version), "v")
	in.Notes = strings.TrimSpace(in.Notes)
	if f := in.validate(); len(f) > 0 {
		writeError(w, 422, "validation_failed", "Please check the highlighted fields", f)
		return
	}
	at := time.Now().UTC().Truncate(time.Second)
	if in.PublishedAt != nil {
		at = in.PublishedAt.UTC().Truncate(time.Second)
	}
	file, err := a.issuer.Signer().SignManifest(entitle.Manifest{
		Version: in.Version, Channel: in.Channel, PublishedAt: at, HasMigrations: in.HasMigrations,
		MinFromVersion: in.MinFromVersion, Images: in.Images, Notes: in.Notes,
	})
	if err != nil {
		a.internal(w, err)
		return
	}
	rel, err := a.store.CreateRelease(r.Context(), store.Release{
		Version: in.Version, Channel: in.Channel, PublishedAt: at, Notes: in.Notes, HasMigrations: in.HasMigrations,
		MinFromVersion: in.MinFromVersion, Images: in.Images, Manifest: string(file),
	}, who(r).User.ID)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		writeError(w, 409, "conflict", "That version was already published", map[string]string{"version": "Already published"})
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	a.audit(r, "", "release_published", map[string]any{"version": rel.Version, "channel": rel.Channel})
	writeJSON(w, 201, rel)
}

func (a *API) withdrawRelease(w http.ResponseWriter, r *http.Request) {
	if err := a.store.WithdrawRelease(r.Context(), r.PathValue("id")); a.notFoundOrFail(w, err) {
		return
	}
	rel, err := a.store.GetRelease(r.Context(), r.PathValue("id"))
	if a.notFoundOrFail(w, err) {
		return
	}
	a.audit(r, "", "release_withdrawn", map[string]any{"version": rel.Version})
	writeJSON(w, 200, rel)
}

func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListAudit(r.Context(), r.URL.Query().Get("installation"), 200)
	if a.notFoundOrFail(w, err) {
		return
	}
	writeJSON(w, 200, list)
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
