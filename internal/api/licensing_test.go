package api

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/config"
	"github.com/redefynelabs/erp-control-plane/internal/entitle"
	"github.com/redefynelabs/erp-control-plane/internal/store"
)

const crmSales = `"edition":"services","template":"agency","apps":["crm","accounting"],"features":["crm.leads","accounting.estimates"]`

// signerKey reads the public key the control plane publishes.
func signerKey(t *testing.T, h http.Handler) string {
	t.Helper()
	var out struct {
		Data struct {
			PublicKey string `json:"public_key"`
		}
	}
	_ = json.Unmarshal(do(h, "GET", "/v1/entitlements/catalog", "", bearer()...).Body.Bytes(), &out)
	return out.Data.PublicKey
}

func payloadOf(t *testing.T, file string) entitle.Payload {
	t.Helper()
	var b entitle.Bundle
	if err := json.Unmarshal([]byte(file), &b); err != nil {
		t.Fatal(err)
	}
	var p entitle.Payload
	if err := json.Unmarshal(b.Payload, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestActivationExchangesACodeOnce(t *testing.T) {
	h, signer, _ := entitlementServer(t)
	inst := newInstallation(t, h)

	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/activation", "", bearer()...); w.Code != 409 {
		t.Fatalf("no code before a file is issued: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{`+crmSales+`,"license":"lifetime","updates_until":"2099-01-01T00:00:00Z"}`, bearer()...); w.Code != 201 {
		t.Fatalf("issue: %d %s", w.Code, w.Body)
	}
	w := do(h, "POST", "/v1/installations/"+inst.ID+"/activation", "", bearer()...)
	if w.Code != 201 {
		t.Fatalf("make code: %d %s", w.Code, w.Body)
	}
	var made struct{ Data activationOut }
	_ = json.Unmarshal(w.Body.Bytes(), &made)
	if !strings.HasPrefix(made.Data.Code, "ACT-") || len(made.Data.Code) != 27 {
		t.Fatalf("code = %q", made.Data.Code)
	}

	if w := do(h, "POST", "/v1/activate", `{"code":"ACT-AAAAA-BBBBB-CCCCC-DDDDD"}`); w.Code != 422 {
		t.Fatalf("a wrong code: %d", w.Code)
	}
	// Typed in lower case without dashes still works.
	typed := strings.ToLower(strings.ReplaceAll(made.Data.Code, "-", ""))
	w = do(h, "POST", "/v1/activate", `{"code":"`+typed+`"}`)
	if w.Code != 200 {
		t.Fatalf("activate: %d %s", w.Code, w.Body)
	}
	var out struct{ Data activateOut }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Data.InstallationID != inst.ID || out.Data.InstallationSecret != entitle.Secret(secretsKey, inst.ID, 1) || out.Data.PublicKey != signer.PublicKey() {
		t.Fatalf("answer = %+v", out.Data)
	}
	p := payloadOf(t, out.Data.Entitlement)
	if p.License == nil || p.License.Type != "lifetime" || p.ExpiresAt != nil || p.UpdatesUntil == nil {
		t.Fatalf("the file carries the licence: %+v", p)
	}
	pub, _ := base64.StdEncoding.DecodeString(out.Data.PublicKey)
	var b entitle.Bundle
	_ = json.Unmarshal([]byte(out.Data.Entitlement), &b)
	sig, _ := base64.StdEncoding.DecodeString(b.Signature)
	canon, _ := entitle.Canonical(b.Payload)
	if !ed25519.Verify(pub, canon, sig) {
		t.Fatal("the file does not verify with the public key it came with")
	}

	if w := do(h, "POST", "/v1/activate", `{"code":"`+made.Data.Code+`"}`); w.Code != 422 {
		t.Fatalf("a code works once: %d", w.Code)
	}
	got := do(h, "GET", "/v1/installations/"+inst.ID, "", bearer()...)
	var detail struct {
		Data struct{ Installation installationView }
	}
	_ = json.Unmarshal(got.Body.Bytes(), &detail)
	if detail.Data.Installation.ActivatedAt == nil || detail.Data.Installation.State == StateAwaiting {
		t.Fatalf("after activation: %+v", detail.Data.Installation)
	}
}

func TestAnewCodeReplacesTheOldOne(t *testing.T) {
	h, _, _ := entitlementServer(t)
	inst := newInstallation(t, h)
	do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{`+crmSales+`}`, bearer()...)
	var first, second struct{ Data activationOut }
	_ = json.Unmarshal(do(h, "POST", "/v1/installations/"+inst.ID+"/activation", "", bearer()...).Body.Bytes(), &first)
	_ = json.Unmarshal(do(h, "POST", "/v1/installations/"+inst.ID+"/activation", "", bearer()...).Body.Bytes(), &second)
	if w := do(h, "POST", "/v1/activate", `{"code":"`+first.Data.Code+`"}`); w.Code != 422 {
		t.Fatalf("the replaced code still works: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/activate", `{"code":"`+second.Data.Code+`"}`); w.Code != 200 {
		t.Fatalf("the new code: %d", w.Code)
	}
}

func TestGuessingCodesIsSlowedDown(t *testing.T) {
	h, _, _ := entitlementServer(t)
	for i := 0; i < guardMax; i++ {
		if w := do(h, "POST", "/v1/activate", `{"code":"ACT-AAAAA-BBBBB-CCCCC-DDDDD"}`); w.Code != 422 {
			t.Fatalf("guess %d: %d", i, w.Code)
		}
	}
	if w := do(h, "POST", "/v1/activate", `{"code":"ACT-AAAAA-BBBBB-CCCCC-DDDDD"}`); w.Code != 429 {
		t.Fatalf("after too many guesses: %d", w.Code)
	}
}

func TestSyncReportsAndOffersOnlyWhatTheContractCovers(t *testing.T) {
	h, _, st := entitlementServer(t)
	inst := newInstallation(t, h)
	secret := entitle.Secret(secretsKey, inst.ID, 1)
	// Maintenance ends mid-2026: the March release is covered, the October one is not.
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{`+crmSales+`,"license":"lifetime","updates_until":"2026-06-30T00:00:00Z"}`, bearer()...); w.Code != 201 {
		t.Fatalf("issue: %d %s", w.Code, w.Body)
	}
	images := `{"server":"reg/erp-server@sha256:aa","web":"reg/erp-web@sha256:bb"}`
	for _, rel := range []string{
		`{"version":"1.1.0","channel":"stable","published_at":"2026-03-01T00:00:00Z","notes":"March","images":` + images + `}`,
		`{"version":"1.2.0","channel":"stable","published_at":"2026-10-01T00:00:00Z","notes":"October","images":` + images + `}`,
		`{"version":"1.3.0-beta1","channel":"beta","published_at":"2026-04-01T00:00:00Z","images":` + images + `}`,
	} {
		if w := do(h, "POST", "/v1/releases", rel, bearer()...); w.Code != 201 {
			t.Fatalf("publish: %d %s", w.Code, w.Body)
		}
	}
	w := asInst(h, inst, "POST", "/v1/sync", `{"server_version":"1.0.0","server_build":"2026-01-05"}`, time.Now(), secret)
	if w.Code != 200 {
		t.Fatalf("sync: %d %s", w.Code, w.Body)
	}
	var out struct{ Data syncOut }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Data.KeyStatus != "active" || out.Data.Entitlement == "" || out.Data.Latest != 1 {
		t.Fatalf("first sync: %+v", out.Data)
	}
	if len(out.Data.Releases) != 1 || out.Data.Releases[0].Version != "1.1.0" || !out.Data.Releases[0].Direct {
		t.Fatalf("offered: %+v", out.Data.Releases)
	}
	// The manifest verifies with the key.
	var m entitle.Bundle
	_ = json.Unmarshal([]byte(out.Data.Releases[0].Manifest), &m)
	canon, _ := entitle.Canonical(m.Payload)
	if !strings.Contains(string(canon), `"version":"1.1.0"`) {
		t.Fatalf("manifest = %s", canon)
	}
	pub, _ := base64.StdEncoding.DecodeString(signerKey(t, h))
	sig, _ := base64.StdEncoding.DecodeString(m.Signature)
	if !ed25519.Verify(pub, canon, sig) {
		t.Fatal("the manifest is not signed with the vendor key")
	}

	// Once it reports version 1 as applied it is not sent the file again, and the control plane knows how it is doing.
	w = asInst(h, inst, "POST", "/v1/sync", `{"applied_version":1,"server_version":"1.1.0"}`, time.Now(), secret)
	out = struct{ Data syncOut }{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Data.Entitlement != "" || len(out.Data.Releases) != 0 {
		t.Fatalf("second sync: %+v", out.Data)
	}
	got, _ := st.GetInstallation(t.Context(), inst.ID)
	if got.AppliedVersion == nil || *got.AppliedVersion != 1 || got.ServerVersion != "1.1.0" || got.LastSeenAt == nil {
		t.Fatalf("what was recorded: %+v", got)
	}
	if s, _ := DeriveState(got, time.Now()); s != StateActive {
		t.Fatalf("state = %s", s)
	}

	if w := asInst(h, inst, "POST", "/v1/sync", `{}`, time.Now().Add(-time.Hour), secret); w.Code != 401 {
		t.Fatalf("a stale timestamp: %d", w.Code)
	}
	if w := asInst(h, inst, "POST", "/v1/sync", `{}`, time.Now(), "wrong"); w.Code != 401 {
		t.Fatalf("a wrong secret: %d", w.Code)
	}
}

func TestSuspendResumeAndRevoke(t *testing.T) {
	h, _, st := entitlementServer(t)
	inst := newInstallation(t, h)
	secret := entitle.Secret(secretsKey, inst.ID, 1)
	do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{`+crmSales+`}`, bearer()...)

	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/suspend", `{}`, bearer()...); w.Code != 422 {
		t.Fatalf("a reason is needed: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/suspend", `{"note":"Unpaid invoice"}`, bearer()...); w.Code != 200 {
		t.Fatalf("suspend: %d %s", w.Code, w.Body)
	}
	last, _ := st.LatestIssued(t.Context(), inst.ID)
	if last.Version != 2 || !payloadOf(t, last.File).Suspended {
		t.Fatalf("the suspension travels in a new signed file: v%d", last.Version)
	}
	var out struct{ Data syncOut }
	_ = json.Unmarshal(asInst(h, inst, "POST", "/v1/sync", `{"applied_version":1,"server_version":"1.0.0"}`, time.Now(), secret).Body.Bytes(), &out)
	if out.Data.KeyStatus != "suspended" || out.Data.StatusNote != "Unpaid invoice" || out.Data.Entitlement == "" || len(out.Data.Releases) != 0 {
		t.Fatalf("suspended sync: %+v", out.Data)
	}

	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/resume", `{}`, bearer()...); w.Code != 200 {
		t.Fatalf("resume: %d %s", w.Code, w.Body)
	}
	last, _ = st.LatestIssued(t.Context(), inst.ID)
	if last.Version != 3 || payloadOf(t, last.File).Suspended || len(payloadOf(t, last.File).Apps) != 2 {
		t.Fatalf("resume restores what the customer had: v%d", last.Version)
	}

	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/revoke", `{"note":"Contract ended"}`, bearer()...); w.Code != 200 {
		t.Fatalf("revoke: %d", w.Code)
	}
	out = struct{ Data syncOut }{}
	_ = json.Unmarshal(asInst(h, inst, "POST", "/v1/sync", `{"server_version":"1.0.0"}`, time.Now(), secret).Body.Bytes(), &out)
	if out.Data.KeyStatus != "revoked" || out.Data.Entitlement != "" || len(out.Data.Releases) != 0 {
		t.Fatalf("revoked sync: %+v", out.Data)
	}
	if w := asInst(h, inst, "GET", "/v1/entitlements/current", "", time.Now(), secret); w.Code != 403 {
		t.Fatalf("an older server fetching after revocation: %d", w.Code)
	}

	trail, _ := st.ListAudit(t.Context(), inst.ID, 50)
	var actions []string
	for _, e := range trail {
		actions = append(actions, e.Action)
	}
	for _, want := range []string{"customer_created", "entitlement_issued", "suspended", "resumed", "revoked"} {
		if !strings.Contains(strings.Join(actions, ","), want) {
			t.Errorf("the audit trail lacks %q: %v", want, actions)
		}
	}
}

func TestLicenceTerms(t *testing.T) {
	h, _, st := entitlementServer(t)
	inst := newInstallation(t, h)
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/licence", `{"license":"lifetime"}`, bearer()...); w.Code != 409 {
		t.Fatalf("before any file: %d", w.Code)
	}
	do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{`+crmSales+`,"license":"subscription","expires_at":"2099-01-01T00:00:00Z"}`, bearer()...)

	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/licence", `{"license":"subscription"}`, bearer()...); w.Code != 422 {
		t.Fatalf("a subscription needs an end: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/licence", `{"license":"lifetime","expires_at":"2099-01-01T00:00:00Z","updates_until":"2027-12-31T00:00:00Z"}`, bearer()...); w.Code != 200 {
		t.Fatalf("lifetime: %d %s", w.Code, w.Body)
	}
	last, _ := st.LatestIssued(t.Context(), inst.ID)
	p := payloadOf(t, last.File)
	if p.License.Type != "lifetime" || p.ExpiresAt != nil || p.UpdatesUntil == nil || len(p.Apps) != 2 {
		t.Fatalf("a lifetime licence has no end date, keeps its apps and has a maintenance date: %+v", p)
	}
	// Renewing the maintenance changes only that date.
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/licence", `{"license":"lifetime","updates_until":"2028-12-31T00:00:00Z"}`, bearer()...); w.Code != 200 {
		t.Fatalf("renew: %d", w.Code)
	}
	last, _ = st.LatestIssued(t.Context(), inst.ID)
	if got := payloadOf(t, last.File).UpdatesUntil; got == nil || got.Year() != 2028 {
		t.Fatalf("updates_until = %v", got)
	}
}

func TestAnAppCannotBeGrantedWithoutWhatItNeeds(t *testing.T) {
	h, _, _ := entitlementServer(t)
	inst := newInstallation(t, h)
	w := do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{"edition":"services","template":"agency","apps":["payroll"],"features":[]}`, bearer()...)
	if w.Code != 422 || !strings.Contains(w.Body.String(), "needs the hr app") {
		t.Fatalf("payroll alone: %d %s", w.Code, w.Body)
	}
}

func TestReleasesAreValidatedAndUnique(t *testing.T) {
	h, _, _ := entitlementServer(t)
	images := `{"server":"reg/erp-server@sha256:aa","web":"reg/erp-web@sha256:bb"}`
	if w := do(h, "POST", "/v1/releases", `{"version":"one","channel":"stable","images":`+images+`}`, bearer()...); w.Code != 422 {
		t.Fatalf("a bad version: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/releases", `{"version":"1.0.0","channel":"stable","images":{"server":"reg/erp-server:latest","web":"x@sha256:1"}}`, bearer()...); w.Code != 422 {
		t.Fatalf("an image by tag, not digest: %d", w.Code)
	}
	ok := `{"version":"v1.0.0","channel":"stable","has_migrations":true,"images":` + images + `}`
	w := do(h, "POST", "/v1/releases", ok, bearer()...)
	if w.Code != 201 {
		t.Fatalf("publish: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/v1/releases", ok, bearer()...); w.Code != 409 {
		t.Fatalf("publishing the same version twice: %d", w.Code)
	}
	list := do(h, "GET", "/v1/releases", "", bearer()...)
	if !strings.Contains(list.Body.String(), `"version":"1.0.0"`) {
		t.Fatalf("list: %s", list.Body)
	}
}

func TestAWithdrawnReleaseIsNotOffered(t *testing.T) {
	h, _, _ := entitlementServer(t)
	inst := newInstallation(t, h)
	secret := entitle.Secret(secretsKey, inst.ID, 1)
	do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{`+crmSales+`}`, bearer()...)
	images := `{"server":"reg/erp-server@sha256:aa","web":"reg/erp-web@sha256:bb"}`
	w := do(h, "POST", "/v1/releases", `{"version":"2.0.0","channel":"stable","images":`+images+`}`, bearer()...)
	var rel struct{ Data store.Release }
	_ = json.Unmarshal(w.Body.Bytes(), &rel)
	count := func() int {
		var out struct{ Data syncOut }
		_ = json.Unmarshal(asInst(h, inst, "POST", "/v1/sync", `{"applied_version":1,"server_version":"1.0.0"}`, time.Now(), secret).Body.Bytes(), &out)
		return len(out.Data.Releases)
	}
	if count() != 1 {
		t.Fatal("a published release is offered")
	}
	if w := do(h, "POST", "/v1/releases/"+rel.Data.ID+"/withdraw", "", bearer()...); w.Code != 200 {
		t.Fatalf("withdraw: %d", w.Code)
	}
	if count() != 0 {
		t.Fatal("a withdrawn release is not offered")
	}
}

func TestOnlyAdminsChangeLicences(t *testing.T) {
	h, _, _ := entitlementServer(t)
	inst := newInstallation(t, h)
	for _, path := range []string{"/activation", "/licence", "/suspend", "/resume", "/revoke"} {
		if w := do(h, "POST", "/v1/installations/"+inst.ID+path, `{}`); w.Code != 401 {
			t.Errorf("%s without signing in: %d", path, w.Code)
		}
	}
	if w := do(h, "POST", "/v1/releases", `{}`); w.Code != 401 {
		t.Errorf("publishing without signing in: %d", w.Code)
	}
}

func TestDeriveState(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	ahead := func(d time.Duration) *time.Time { t := now.Add(d); return &t }
	one, two := 1, 2
	cases := []struct {
		name string
		i    store.Installation
		want string
	}{
		{"demo", store.Installation{Kind: "demo"}, StateDemo},
		{"revoked", store.Installation{Kind: "customer", Status: "revoked", ActivatedAt: ago(time.Hour)}, StateRevoked},
		{"suspended by status", store.Installation{Kind: "customer", Status: "suspended", ActivatedAt: ago(time.Hour)}, StateSuspended},
		{"suspended by file", store.Installation{Kind: "customer", Status: "active", Suspended: true, ActivatedAt: ago(time.Hour)}, StateSuspended},
		{"waiting", store.Installation{Kind: "customer", Status: "active"}, StateAwaiting},
		{"expired", store.Installation{Kind: "customer", Status: "active", ActivatedAt: ago(time.Hour), ExpiresAt: ago(time.Minute), LastSeenAt: ago(time.Minute)}, StateExpired},
		{"silent", store.Installation{Kind: "customer", Status: "active", ActivatedAt: ago(72 * time.Hour), LastSeenAt: ago(30 * time.Hour)}, StateUnreachable},
		{"never seen", store.Installation{Kind: "customer", Status: "active", ActivatedAt: ago(72 * time.Hour)}, StateUnreachable},
		{"behind", store.Installation{Kind: "customer", Status: "active", ActivatedAt: ago(time.Hour), LastSeenAt: ago(time.Minute), Version: 2, AppliedVersion: &one}, StateOutOfDate},
		{"nothing applied", store.Installation{Kind: "customer", Status: "active", ActivatedAt: ago(time.Hour), LastSeenAt: ago(time.Minute), Version: 2}, StateOutOfDate},
		{"fine", store.Installation{Kind: "customer", Status: "active", ActivatedAt: ago(time.Hour), LastSeenAt: ago(time.Minute), Version: 2, AppliedVersion: &two, ExpiresAt: ahead(time.Hour)}, StateActive},
	}
	for _, c := range cases {
		if got, text := DeriveState(c.i, now); got != c.want || text == "" {
			t.Errorf("%s: %s (%q), want %s", c.name, got, text, c.want)
		}
	}
	for name, c := range map[string]struct {
		until *time.Time
		want  string
	}{"none": {nil, "none"}, "ok": {ahead(200 * 24 * time.Hour), "ok"}, "ending": {ahead(30 * 24 * time.Hour), "ending"}, "lapsed": {ago(time.Hour), "lapsed"}} {
		if got := AMC(store.Installation{UpdatesUntil: c.until}, now); got != c.want {
			t.Errorf("amc %s: %s", name, got)
		}
	}
}

func TestEligibleReleases(t *testing.T) {
	at := func(m time.Month) time.Time { return time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC) }
	withdrawn := at(2)
	all := []store.Release{
		{Version: "1.0.0", Channel: "stable", PublishedAt: at(1)},
		{Version: "1.1.0", Channel: "stable", PublishedAt: at(3)},
		{Version: "1.2.0", Channel: "stable", PublishedAt: at(6), MinFromVersion: "1.1.0"},
		{Version: "1.2.1", Channel: "security", PublishedAt: at(7), MinFromVersion: "1.1.0"},
		{Version: "1.3.0", Channel: "stable", PublishedAt: at(9), WithdrawnAt: &withdrawn},
		{Version: "2.0.0-beta1", Channel: "beta", PublishedAt: at(5)},
	}
	end := at(6).Add(24 * time.Hour)
	versions := func(o []offered) string {
		var v []string
		for _, x := range o {
			v = append(v, x.Version)
		}
		return strings.Join(v, ",")
	}
	if got := versions(EligibleReleases(all, &entitle.Payload{UpdatesUntil: &end}, "1.0.0")); got != "1.2.0,1.1.0" {
		t.Errorf("contract to June: %s", got)
	}
	// Running 1.0.0, 1.2.0 needs 1.1.0 first: it is listed but not direct.
	for _, o := range EligibleReleases(all, &entitle.Payload{UpdatesUntil: &end}, "1.0.0") {
		if o.Version == "1.2.0" && o.Direct {
			t.Error("1.2.0 cannot be applied straight from 1.0.0")
		}
	}
	if got := versions(EligibleReleases(all, &entitle.Payload{UpdatesUntil: &end}, "dev")); got != "1.2.0,1.1.0,1.0.0" {
		t.Errorf("a build with no release version is offered every eligible release: %s", got)
	}
	if got := versions(EligibleReleases(all, &entitle.Payload{UpdatesUntil: &end}, "1.2.0")); got != "" {
		t.Errorf("already current: %q", got)
	}
	if got := versions(EligibleReleases(all, nil, "1.1.0")); got != "1.2.1,1.2.0" {
		t.Errorf("no contract date means every stable and security release: %s", got)
	}
	// With no maintenance date the subscription end counts.
	if got := versions(EligibleReleases(all, &entitle.Payload{ExpiresAt: &end}, "1.1.0")); got != "1.2.0" {
		t.Errorf("subscription end: %s", got)
	}
}

func TestDatesAreTheVendorsNotTheDatabaseSessions(t *testing.T) {
	h, _, _ := entitlementServer(t)
	inst := newInstallation(t, h)
	do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{`+crmSales+`,"license":"lifetime","updates_until":"2027-12-31T23:59:59Z"}`, bearer()...)
	var out struct {
		Data []struct {
			UpdatesUntil string `json:"updates_until"`
		}
	}
	_ = json.Unmarshal(do(h, "GET", "/v1/installations", "", bearer()...).Body.Bytes(), &out)
	if len(out.Data) != 1 || out.Data[0].UpdatesUntil != "2027-12-31T23:59:59Z" {
		t.Fatalf("an end date must come back as the same UTC moment, not shifted into the database's zone: %+v", out.Data)
	}
}

func TestPipelineRegistersReleasesWithItsOwnToken(t *testing.T) {
	const pipelineToken = "pipeline-token-pipeline-token-0123456789"
	h, _, st := entitlementServerWith(t, func(c *config.Config) { c.ReleaseToken = pipelineToken })
	rel := `{"version":"v2.1.0","channel":"stable","has_migrations":true,"notes":"n","images":{"server":"reg/erp-server@sha256:aa","web":"reg/erp-web@sha256:bb"}}`
	pipeline := []string{"Authorization", "Bearer " + pipelineToken}

	if w := do(h, "POST", "/v1/ci/releases", rel); w.Code != 401 {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/ci/releases", rel, bearer()...); w.Code != 401 {
		t.Fatalf("a staff session is not the pipeline token: %d", w.Code)
	}
	if w := do(h, "GET", "/v1/releases", "", pipeline...); w.Code != 401 {
		t.Fatalf("the pipeline token reaches nothing else: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/ci/releases", rel, pipeline...); w.Code != 201 {
		t.Fatalf("register: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/v1/ci/releases", rel, pipeline...); w.Code != 200 {
		t.Fatalf("the same version again is a no-op: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/v1/releases", rel, bearer()...); w.Code != 409 {
		t.Fatalf("staff still get a conflict: %d", w.Code)
	}
	all, err := st.ListReleases(t.Context())
	if err != nil || len(all) != 1 || all[0].Version != "2.1.0" {
		t.Fatalf("one release: %+v %v", all, err)
	}
}

func TestPipelineRouteIsOffWithoutAToken(t *testing.T) {
	h, _, _ := entitlementServer(t)
	if w := do(h, "POST", "/v1/ci/releases", `{}`, "Authorization", "Bearer "); w.Code != 401 {
		t.Fatalf("no token configured: %d", w.Code)
	}
}
