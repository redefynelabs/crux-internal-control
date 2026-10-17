package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/entitle"
	"github.com/redefynelabs/erp-control-plane/internal/store"
)

type installationKey struct{}

func (a *API) entitlementRoutes(mux *http.ServeMux, staff, adminOnly func(http.HandlerFunc) http.HandlerFunc) {
	// Called by the ERP installations, authenticated by their credential rather than a person's session.
	mux.HandleFunc("POST /v1/access-requests", a.asInstallation(a.receiveAccessRequest))
	mux.HandleFunc("GET /v1/entitlements/current", a.asInstallation(a.currentEntitlement))

	mux.HandleFunc("GET /v1/entitlements/catalog", staff(a.entitlementCatalog))
	mux.HandleFunc("GET /v1/installations", staff(a.listInstallations))
	mux.HandleFunc("POST /v1/installations", adminOnly(a.createInstallation))
	mux.HandleFunc("GET /v1/installations/{id}", staff(a.getInstallation))
	mux.HandleFunc("PATCH /v1/installations/{id}", adminOnly(a.patchInstallation))
	mux.HandleFunc("GET /v1/installations/{id}/credential", adminOnly(a.installationCredential))
	mux.HandleFunc("POST /v1/installations/{id}/credential/rotate", adminOnly(a.rotateCredential))
	mux.HandleFunc("POST /v1/installations/{id}/entitlements", adminOnly(a.issueEntitlement))
	mux.HandleFunc("GET /v1/installations/{id}/entitlements/{version}/file", staff(a.entitlementFile))
	mux.HandleFunc("GET /v1/access-requests", staff(a.listAccessRequests))
	mux.HandleFunc("POST /v1/installations/{id}/requests/{request}/decline", adminOnly(a.declineAccessRequest))
	a.licensingRoutes(mux, staff, adminOnly)
	a.installRoutes(mux)
}

func (a *API) needIssuer(w http.ResponseWriter) bool {
	if a.issuer == nil {
		writeError(w, 503, "entitlements_unavailable", "Entitlements are not set up on this control plane")
		return false
	}
	return true
}

// asInstallation authenticates a call from an ERP installation: its id, a timestamp close to now and an HMAC of the
// request made with its credential. Every failure looks the same, so the answer does not tell which part was wrong.
func (a *API) asInstallation(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fail := func() { writeError(w, 401, "unauthorized", "The installation could not be verified") }
		id := r.Header.Get("X-Installation-Id")
		body, err := io.ReadAll(r.Body)
		if err != nil || id == "" {
			fail()
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		inst, err := a.store.GetInstallation(r.Context(), id)
		if err != nil {
			fail()
			return
		}
		secret := entitle.Secret(a.cfg.SecretsKey, inst.ID, inst.SecretGeneration)
		if !entitle.VerifyRequest(secret, id, r.Header.Get("X-Timestamp"), r.Header.Get("X-Signature"), r.Method, r.URL.Path, body, time.Now()) {
			fail()
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), installationKey{}, inst)))
	}
}

func installationOf(r *http.Request) store.Installation {
	i, _ := r.Context().Value(installationKey{}).(store.Installation)
	return i
}

var requestNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)?$`)

type accessRequestJSON struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Name        string    `json:"name"`
	Note        string    `json:"note"`
	RequestedAt time.Time `json:"requested_at"`
}

func (a *API) receiveAccessRequest(w http.ResponseWriter, r *http.Request) {
	var in accessRequestJSON
	if !decode(w, r, &in) {
		return
	}
	f := map[string]string{}
	if _, err := parseUUID(in.ID); err != nil {
		f["id"] = "Must be a UUID"
	}
	if in.Kind != "app" && in.Kind != "feature" {
		f["kind"] = "Must be app or feature"
	}
	if len(in.Name) > 80 || !requestNameRE.MatchString(in.Name) || (in.Kind == "app") == strings.Contains(in.Name, ".") {
		f["name"] = "Not the name of an app or feature"
	}
	if len(in.Note) > 2000 {
		f["note"] = "Too long"
	}
	if len(f) > 0 {
		writeError(w, 422, "validation_failed", "The request is not valid", f)
		return
	}
	if in.RequestedAt.IsZero() {
		in.RequestedAt = time.Now()
	}
	created, err := a.store.AddAccessRequest(r.Context(), installationOf(r).ID, in.ID, in.Kind, in.Name, strings.TrimSpace(in.Note), in.RequestedAt)
	if err != nil {
		a.internal(w, err)
		return
	}
	if created {
		a.log.Info("access request received", "installation", installationOf(r).Name, "name", in.Name)
		writeJSON(w, 201, map[string]string{"status": "received"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "received"})
}

func parseUUID(s string) (string, error) {
	if len(s) != 36 || strings.Count(s, "-") != 4 {
		return "", errors.New("not a uuid")
	}
	return s, nil
}

// currentEntitlement answers with the newest file as the raw bundle, which is what the installation applies.
func (a *API) currentEntitlement(w http.ResponseWriter, r *http.Request) {
	if err := a.store.RecordSeen(r.Context(), installationOf(r).ID, nil, "", ""); err != nil {
		a.log.Error("could not record a check", "installation", installationOf(r).ID, "error", err)
	}
	if installationOf(r).Status == "revoked" {
		writeError(w, 403, "revoked", "This key was revoked")
		return
	}
	last, err := a.store.LatestIssued(r.Context(), installationOf(r).ID)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, last.File)
}

func (a *API) entitlementCatalog(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"catalog": entitle.Catalog, "can_issue": a.issuer != nil && a.issuer.Signer() != nil}
	// install_url is where customers fetch the one-line installer; empty until installs are set up (registry and public address).
	if a.cfg.Install.Registry != "" && a.cfg.Install.PublicURL != "" {
		out["install_url"] = a.cfg.Install.PublicURL
	}
	if a.issuer != nil && a.issuer.Signer() != nil {
		out["public_key"] = a.issuer.Signer().PublicKey()
	}
	writeJSON(w, 200, out)
}

func (a *API) listInstallations(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListInstallations(r.Context(), r.URL.Query().Get("kind"))
	if err != nil {
		a.internal(w, err)
		return
	}
	// state=<state> and amc=ending|lapsed narrow the list; the states are worked out here, not stored.
	state, amc, now := r.URL.Query().Get("state"), r.URL.Query().Get("amc"), time.Now()
	out := make([]installationView, 0, len(list))
	for _, i := range list {
		v := installationViewOf(i, now)
		if (state != "" && v.State != state) || (amc != "" && v.AMC != amc) {
			continue
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
}

type installationJSON struct {
	Name   string `json:"name"`
	LeadID string `json:"lead_id"`
	Notes  string `json:"notes"`
}

func (in installationJSON) validate() map[string]string {
	f := map[string]string{}
	if n := strings.TrimSpace(in.Name); n == "" || len(n) > 160 {
		f["name"] = "Enter the customer's name"
	}
	if len(in.Notes) > 4000 {
		f["notes"] = "Too long"
	}
	return f
}

func (a *API) createInstallation(w http.ResponseWriter, r *http.Request) {
	var in installationJSON
	if !decode(w, r, &in) {
		return
	}
	if f := in.validate(); len(f) > 0 {
		writeError(w, 422, "validation_failed", "Please check the highlighted fields", f)
		return
	}
	inst, err := a.store.CreateInstallation(r.Context(), strings.TrimSpace(in.Name), "customer", in.LeadID, "", in.Notes)
	if err != nil {
		a.internal(w, err)
		return
	}
	a.audit(r, inst.ID, "customer_created", map[string]any{"name": inst.Name})
	writeJSON(w, 201, installationViewOf(inst, time.Now()))
}

func (a *API) getInstallation(w http.ResponseWriter, r *http.Request) {
	inst, err := a.store.GetInstallation(r.Context(), r.PathValue("id"))
	if a.notFoundOrFail(w, err) {
		return
	}
	history, err := a.store.ListIssued(r.Context(), inst.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	reqs, err := a.store.ListAccessRequests(r.Context(), "", inst.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	activation, err := a.store.LatestActivation(r.Context(), inst.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		a.internal(w, err)
		return
	}
	trail, err := a.store.ListAudit(r.Context(), inst.ID, 50)
	if err != nil {
		a.internal(w, err)
		return
	}
	out := map[string]any{"installation": installationViewOf(inst, time.Now()), "history": history, "requests": reqs, "audit": trail}
	if err == nil && activation.ID != "" {
		out["activation"] = activation
	}
	writeJSON(w, 200, out)
}

// notFoundOrFail writes the answer for an error and reports whether it did.
func (a *API) notFoundOrFail(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrNotFound) || isBadID(err):
		writeError(w, 404, "not_found", "Not found")
	default:
		a.internal(w, err)
	}
	return true
}

func (a *API) patchInstallation(w http.ResponseWriter, r *http.Request) {
	var in installationJSON
	if !decode(w, r, &in) {
		return
	}
	if f := in.validate(); len(f) > 0 {
		writeError(w, 422, "validation_failed", "Please check the highlighted fields", f)
		return
	}
	inst, err := a.store.UpdateInstallation(r.Context(), r.PathValue("id"), strings.TrimSpace(in.Name), in.Notes)
	if a.notFoundOrFail(w, err) {
		return
	}
	a.audit(r, inst.ID, "customer_updated", map[string]any{"name": inst.Name})
	writeJSON(w, 200, installationViewOf(inst, time.Now()))
}

// installationCredential shows what to put in the installation's environment. The secret is derived, so it can be shown
// again; only admins can see it.
func (a *API) installationCredential(w http.ResponseWriter, r *http.Request) {
	inst, err := a.store.GetInstallation(r.Context(), r.PathValue("id"))
	if a.notFoundOrFail(w, err) {
		return
	}
	writeJSON(w, 200, a.credential(inst))
}

func (a *API) credential(inst store.Installation) map[string]string {
	out := map[string]string{"INSTALLATION_ID": inst.ID, "INSTALLATION_SECRET": entitle.Secret(a.cfg.SecretsKey, inst.ID, inst.SecretGeneration)}
	if a.issuer != nil && a.issuer.Signer() != nil {
		out["ENTITLEMENT_PUBLIC_KEY"] = a.issuer.Signer().PublicKey()
	}
	return out
}

func (a *API) rotateCredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := a.store.RotateSecret(r.Context(), id); a.notFoundOrFail(w, err) {
		return
	}
	inst, err := a.store.GetInstallation(r.Context(), id)
	if a.notFoundOrFail(w, err) {
		return
	}
	a.audit(r, id, "credential_replaced", nil)
	writeJSON(w, 200, a.credential(inst))
}

type issueJSON struct {
	entitle.Draft
	// Approve lists the access requests this file answers.
	Approve []string `json:"approve_requests"`
}

func (a *API) issueEntitlement(w http.ResponseWriter, r *http.Request) {
	if !a.needIssuer(w) {
		return
	}
	var in issueJSON
	if !decode(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	issued, ok := a.issueDraft(w, r, id, in.Draft, in.Approve)
	if !ok {
		return
	}
	a.log.Info("entitlement issued", "installation", id, "version", issued.Version, "by", who(r).User.Email)
	a.audit(r, id, "entitlement_issued", map[string]any{"version": issued.Version, "apps": len(in.Apps), "features": len(in.Features), "license": in.License})
	writeJSON(w, 201, issued)
}

func (a *API) entitlementFile(w http.ResponseWriter, r *http.Request) {
	v := 0
	if s := r.PathValue("version"); s != "latest" {
		if _, err := fmt.Sscanf(s, "%d", &v); err != nil || v < 1 {
			writeError(w, 404, "not_found", "Not found")
			return
		}
	}
	inst, err := a.store.GetInstallation(r.Context(), r.PathValue("id"))
	if a.notFoundOrFail(w, err) {
		return
	}
	issued, err := a.store.IssuedVersion(r.Context(), inst.ID, v)
	if a.notFoundOrFail(w, err) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-v%d.entitlement"`, fileSafe(inst.Name), issued.Version))
	_, _ = io.WriteString(w, issued.File)
}

var unsafeFile = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func fileSafe(s string) string {
	s = strings.Trim(unsafeFile.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if s == "" {
		return "entitlement"
	}
	return s
}

func (a *API) listAccessRequests(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListAccessRequests(r.Context(), r.URL.Query().Get("status"), "")
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, list)
}

type declineJSON struct {
	Note string `json:"note"`
}

// declineAccessRequest turns a request down. The customer learns it from the next file, so one is signed at once when the
// installation already has one.
func (a *API) declineAccessRequest(w http.ResponseWriter, r *http.Request) {
	var in declineJSON
	if !decode(w, r, &in) {
		return
	}
	note := strings.TrimSpace(in.Note)
	if note == "" || len(note) > 500 {
		writeError(w, 422, "validation_failed", "Write a short note the customer will see", map[string]string{"note": "Between 1 and 500 characters"})
		return
	}
	id := r.PathValue("id")
	if err := a.store.DeclineAccessRequest(r.Context(), id, r.PathValue("request"), note, who(r).User.ID); a.notFoundOrFail(w, err) {
		return
	}
	if a.issuer != nil {
		if err := a.issuer.Reissue(r.Context(), id, who(r).User.ID); err != nil {
			a.log.Error("could not sign a file carrying the decline", "installation", id, "error", err)
		}
	}
	req, err := a.store.GetAccessRequest(r.Context(), id, r.PathValue("request"))
	if a.notFoundOrFail(w, err) {
		return
	}
	writeJSON(w, 200, req)
}
