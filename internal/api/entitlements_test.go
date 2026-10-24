package api

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/config"
	"github.com/redefynelabs/erp-control-plane/internal/demo"
	"github.com/redefynelabs/erp-control-plane/internal/entitle"
	"github.com/redefynelabs/erp-control-plane/internal/mail"
	"github.com/redefynelabs/erp-control-plane/internal/sales"
	"github.com/redefynelabs/erp-control-plane/internal/store"
	"github.com/redefynelabs/erp-control-plane/internal/testutil"
)

const secretsKey = "test-secrets-key-test-secrets-key-0123"

func entitlementServer(t *testing.T) (http.Handler, *entitle.Signer, *store.Store) {
	return entitlementServerWith(t, nil)
}

// entitlementServerWith is entitlementServer with the settings changed first.
func entitlementServerWith(t *testing.T, tweak func(*config.Config)) (http.Handler, *entitle.Signer, *store.Store) {
	t.Helper()
	cfg := &config.Config{SecretsKey: secretsKey, Demo: config.Demo{TTL: time.Hour, MaxLive: 10, MaxPerEmail: 5, MaxPerIP: 10, Domain: "demo.example.com", Scheme: "https", EntitlementTemplate: "agency"}}
	if tweak != nil {
		tweak(cfg)
	}
	st := testutil.NewStore(t)
	token = adminSession(t, st)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	signer, err := entitle.GenerateKey(filepath.Join(t.TempDir(), "key"))
	if err != nil {
		t.Fatal(err)
	}
	issuer := entitle.NewIssuer(st, signer)
	mailer := mail.New(st, "test-secret", log)
	demos := demo.New(cfg.Demo, st, okDBs{}, okStacks{}, log).WithEntitlements(issuer)
	return New(cfg, st, demos, mailer, sales.New(st, mailer, log), nil, log, WithEntitlements(issuer)), signer, st
}

// asInst sends a call the way the ERP does.
func asInst(h http.Handler, inst store.Installation, method, path, body string, at time.Time, secret string) *httptest.ResponseRecorder {
	ts := strconv.FormatInt(at.Unix(), 10)
	return do(h, method, path, body, "X-Installation-Id", inst.ID, "X-Timestamp", ts,
		"X-Signature", entitle.RequestSignature(secret, inst.ID, ts, method, path, []byte(body)))
}

func newInstallation(t *testing.T, h http.Handler) store.Installation {
	t.Helper()
	w := do(h, "POST", "/v1/installations", `{"name":"Acme Clinic"}`, bearer()...)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var out struct{ Data store.Installation }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out.Data
}

func TestIssueAndFetchEntitlement(t *testing.T) {
	h, signer, _ := entitlementServer(t)
	inst := newInstallation(t, h)
	secret := entitle.Secret(secretsKey, inst.ID, 1)

	if w := asInst(h, inst, "GET", "/v1/entitlements/current", "", time.Now(), secret); w.Code != 204 {
		t.Fatalf("nothing issued yet: %d", w.Code)
	}
	body := `{"edition":"services","template":"clinic","apps":["crm","accounting"],"features":["crm.leads","accounting.estimates"],"labels":{"client":{"singular":"Patient","plural":"Patients"}}}`
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", body, bearer()...); w.Code != 201 {
		t.Fatalf("issue: %d %s", w.Code, w.Body)
	}
	w := asInst(h, inst, "GET", "/v1/entitlements/current", "", time.Now(), secret)
	if w.Code != 200 {
		t.Fatalf("current: %d", w.Code)
	}
	var b entitle.Bundle
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	sig, _ := base64.StdEncoding.DecodeString(b.Signature)
	pub, _ := base64.StdEncoding.DecodeString(signer.PublicKey())
	canon, _ := entitle.Canonical(b.Payload)
	if !ed25519.Verify(pub, canon, sig) {
		t.Fatal("the file served does not verify")
	}
	var p entitle.Payload
	_ = json.Unmarshal(b.Payload, &p)
	if p.Version != 1 || p.Customer.ID != inst.ID || p.Labels["client"].Plural != "Patients" {
		t.Fatalf("payload: %+v", p)
	}

	// The next issue is version 2 and replaces the first as the current one.
	do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{"edition":"services","template":"clinic","apps":["crm"],"features":["crm.leads"]}`, bearer()...)
	var b2 entitle.Bundle
	_ = json.Unmarshal(asInst(h, inst, "GET", "/v1/entitlements/current", "", time.Now(), secret).Body.Bytes(), &b2)
	var p2 entitle.Payload
	_ = json.Unmarshal(b2.Payload, &p2)
	if p2.Version != 2 || len(p2.Apps) != 1 {
		t.Fatalf("second: %+v", p2)
	}

	if w := do(h, "GET", "/v1/installations/"+inst.ID+"/entitlements/1/file", "", bearer()...); w.Code != 200 || w.Header().Get("Content-Disposition") == "" {
		t.Fatalf("download: %d", w.Code)
	}
	if w := do(h, "GET", "/v1/installations/"+inst.ID+"/entitlements/latest/file", "", bearer()...); w.Code != 200 {
		t.Fatalf("download latest: %d", w.Code)
	}
}

func TestIssueRefusals(t *testing.T) {
	h, _, _ := entitlementServer(t)
	inst := newInstallation(t, h)
	w := do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{"edition":"services","template":"clinic","apps":["crm"],"features":["accounting.estimates"]}`, bearer()...)
	if w.Code != 422 {
		t.Fatalf("a feature without its app: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{"edition":"services","template":"clinic"}`); w.Code != 401 {
		t.Fatalf("no session: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/installations/not-a-uuid/entitlements", `{"edition":"services","template":"clinic"}`, bearer()...); w.Code != 404 {
		t.Fatalf("bad id: %d", w.Code)
	}
}

func TestInstallationCallsAreAuthenticated(t *testing.T) {
	h, _, _ := entitlementServer(t)
	inst := newInstallation(t, h)
	secret := entitle.Secret(secretsKey, inst.ID, 1)
	req := `{"id":"6f1f6a4e-3a52-4f43-9b1c-5f4c2f6f9a11","kind":"feature","name":"accounting.estimates","note":"please"}`

	if w := asInst(h, inst, "POST", "/v1/access-requests", req, time.Now(), "wrong-secret"); w.Code != 401 {
		t.Fatalf("wrong secret: %d", w.Code)
	}
	if w := asInst(h, inst, "POST", "/v1/access-requests", req, time.Now().Add(-time.Hour), secret); w.Code != 401 {
		t.Fatalf("a replayed old request: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/access-requests", req); w.Code != 401 {
		t.Fatalf("no credential: %d", w.Code)
	}
	if w := asInst(h, inst, "POST", "/v1/access-requests", req, time.Now(), secret); w.Code != 201 {
		t.Fatalf("good request: %d %s", w.Code, w.Body)
	}
	if w := asInst(h, inst, "POST", "/v1/access-requests", req, time.Now(), secret); w.Code != 200 {
		t.Fatalf("a retried post must change nothing: %d", w.Code)
	}
	bad := `{"id":"6f1f6a4e-3a52-4f43-9b1c-5f4c2f6f9a12","kind":"app","name":"accounting.estimates"}`
	if w := asInst(h, inst, "POST", "/v1/access-requests", bad, time.Now(), secret); w.Code != 422 {
		t.Fatalf("an app named like a feature: %d", w.Code)
	}

	// After rotating, the old credential stops working.
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/credential/rotate", "", bearer()...); w.Code != 200 {
		t.Fatalf("rotate: %d", w.Code)
	}
	if w := asInst(h, inst, "GET", "/v1/entitlements/current", "", time.Now(), secret); w.Code != 401 {
		t.Fatalf("old credential after rotation: %d", w.Code)
	}
	if w := asInst(h, inst, "GET", "/v1/entitlements/current", "", time.Now(), entitle.Secret(secretsKey, inst.ID, 2)); w.Code != 204 {
		t.Fatalf("new credential: %d", w.Code)
	}
}

func TestApproveAndDeclineRequests(t *testing.T) {
	h, _, st := entitlementServer(t)
	inst := newInstallation(t, h)
	secret := entitle.Secret(secretsKey, inst.ID, 1)
	const quotes, orders = "6f1f6a4e-3a52-4f43-9b1c-5f4c2f6f9a21", "6f1f6a4e-3a52-4f43-9b1c-5f4c2f6f9a22"
	for id, name := range map[string]string{quotes: "accounting.estimates", orders: "accounting.invoices"} {
		b, _ := json.Marshal(map[string]string{"id": id, "kind": "feature", "name": name})
		if w := asInst(h, inst, "POST", "/v1/access-requests", string(b), time.Now(), secret); w.Code != 201 {
			t.Fatalf("post: %d", w.Code)
		}
	}
	w := do(h, "GET", "/v1/access-requests?status=pending", "", bearer()...)
	var list struct{ Data []store.AccessRequest }
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Data) != 2 || list.Data[0].InstallationName != "Acme Clinic" {
		t.Fatalf("inbox: %+v", list.Data)
	}

	// Issue a first file granting quotes and answering that request.
	body := `{"edition":"services","template":"clinic","apps":["accounting"],"features":["accounting.estimates"],"approve_requests":["` + quotes + `"]}`
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", body, bearer()...); w.Code != 201 {
		t.Fatalf("issue: %d %s", w.Code, w.Body)
	}
	got, err := st.GetAccessRequest(t.Context(), inst.ID, quotes)
	if err != nil || got.Status != "approved" {
		t.Fatalf("approved: %+v %v", got, err)
	}

	// Declining needs a note, and a file carrying it is signed at once.
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/requests/"+orders+"/decline", `{"note":""}`, bearer()...); w.Code != 422 {
		t.Fatalf("decline without a note: %d", w.Code)
	}
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/requests/"+orders+"/decline", `{"note":"Not on your plan"}`, bearer()...); w.Code != 200 {
		t.Fatalf("decline: %d %s", w.Code, w.Body)
	}
	var b entitle.Bundle
	_ = json.Unmarshal(asInst(h, inst, "GET", "/v1/entitlements/current", "", time.Now(), secret).Body.Bytes(), &b)
	var p entitle.Payload
	_ = json.Unmarshal(b.Payload, &p)
	if p.Version != 2 || len(p.Declined) != 1 || p.Declined[0].Name != "accounting.invoices" || p.Declined[0].Note != "Not on your plan" || len(p.Features) != 1 {
		t.Fatalf("the decline travels in the next file: %+v", p)
	}
	if w := do(h, "POST", "/v1/installations/"+inst.ID+"/requests/"+orders+"/decline", `{"note":"again"}`, bearer()...); w.Code != 404 {
		t.Fatalf("declining a settled request: %d", w.Code)
	}
}

func TestADemoGetsAnEntitlement(t *testing.T) {
	h, _, st := entitlementServer(t)
	w := do(h, "POST", "/v1/demo-requests", good)
	if w.Code != 201 {
		t.Fatalf("demo: %d %s", w.Code, w.Body)
	}
	list, err := st.ListInstallations(t.Context(), "demo")
	// A demo is one edition: the services one has every app but Inventory.
	if err != nil || len(list) != 1 || list[0].Version != 1 || list[0].Template != "agency" || list[0].Edition != "services" || list[0].AppCount != 5 {
		t.Fatalf("the demo's installation: %+v %v", list, err)
	}
}
