package api

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/redefynelabs/erp-control-plane/internal/config"
)

var withRegistry = func(c *config.Config) {
	c.Install = config.Install{Registry: "ghcr.io/vendor", User: "reader", Token: "ghp_readonly123"}
}

func TestTheInstallerIsServedWithTheAddressFilledIn(t *testing.T) {
	h, _, _ := entitlementServerWith(t, func(c *config.Config) { c.Install.PublicURL = "https://api.vendor.example" })
	w := do(h, "GET", "/install", "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/x-shellscript") {
		t.Fatalf("%d %s", w.Code, w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	if strings.Contains(body, "__CP_URL__") || !strings.Contains(body, `CP_URL="https://api.vendor.example"`) {
		t.Fatalf("the address was not filled in:\n%.400s", body)
	}
	// What is served must be a script bash accepts, and it holds no secret.
	f := t.TempDir() + "/install.sh"
	if err := os.WriteFile(f, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bash", "-n", f).CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v %s", err, out)
	}
}

func TestWithoutAConfiguredAddressTheInstallerUsesTheOneItWasReachedOn(t *testing.T) {
	h, _, _ := entitlementServer(t)
	w := do(h, "GET", "/install", "", "X-Forwarded-Proto", "https")
	if !strings.Contains(w.Body.String(), `CP_URL="https://`) {
		t.Fatalf("%.300s", w.Body)
	}
}

func TestTheInstallerKnowsWhoIsAsking(t *testing.T) {
	h, _, _ := entitlementServer(t)
	w := do(h, "GET", "/v1/install/ip", "")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) == "" {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
}

func TestInstallsMustBeSetUpOnTheControlPlane(t *testing.T) {
	h, _, _ := entitlementServer(t)
	if w := do(h, "POST", "/v1/install/start", `{"code":"ACT-AAAAA-BBBBB-CCCCC-DDDDD"}`); w.Code != 503 {
		t.Fatalf("no registry configured: %d %s", w.Code, w.Body)
	}
}

func TestStartingAnInstall(t *testing.T) {
	h, _, st := entitlementServerWith(t, withRegistry)
	inst := newInstallation(t, h)
	do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{`+crmSales+`,"license":"lifetime","updates_until":"2026-06-30T00:00:00Z"}`, bearer()...)
	var made struct{ Data activationOut }
	_ = json.Unmarshal(do(h, "POST", "/v1/installations/"+inst.ID+"/activation", "", bearer()...).Body.Bytes(), &made)
	code := `{"code":"` + made.Data.Code + `"}`
	start := func(accept string) (int, string) {
		hdr := []string{}
		if accept != "" {
			hdr = append(hdr, "Accept", accept)
		}
		w := do(h, "POST", "/v1/install/start", code, hdr...)
		return w.Code, w.Body.String()
	}

	if status, _ := start(""); status != 409 {
		t.Fatalf("a customer with no release published yet: %d", status)
	}
	images := `{"server":"ghcr.io/vendor/erp-server@sha256:aa","web":"ghcr.io/vendor/erp-web@sha256:bb"}`
	// Published after the maintenance ended: not theirs.
	do(h, "POST", "/v1/releases", `{"version":"2.0.0","channel":"stable","published_at":"2026-10-01T00:00:00Z","images":`+images+`}`, bearer()...)
	if status, _ := start(""); status != 409 {
		t.Fatalf("a release outside the maintenance is not offered: %d", status)
	}
	do(h, "POST", "/v1/releases", `{"version":"1.4.2","channel":"stable","published_at":"2026-03-01T00:00:00Z","images":`+images+`}`, bearer()...)
	do(h, "POST", "/v1/releases", `{"version":"1.5.0-beta1","channel":"beta","published_at":"2026-04-01T00:00:00Z","images":`+images+`}`, bearer()...)

	status, body := start("")
	var out struct{ Data installStartOut }
	_ = json.Unmarshal([]byte(body), &out)
	if status != 200 || out.Data.Version != "v1.4.2" || out.Data.Registry != "ghcr.io/vendor" || out.Data.RegistryUser != "reader" || out.Data.RegistryToken != "ghp_readonly123" || out.Data.Customer != "Acme Clinic" {
		t.Fatalf("the newest release the maintenance covers, never a beta: %d %s", status, body)
	}

	status, body = start("text/x-shellvars")
	for _, want := range []string{"REGISTRY='ghcr.io/vendor'\n", "REGISTRY_USER='reader'\n", "REGISTRY_TOKEN='ghp_readonly123'\n", "VERSION='v1.4.2'\n", "CUSTOMER='Acme Clinic'\n"} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("the installer's format lacks %q:\n%s", want, body)
		}
	}

	// Asking is not using the code up: the customer's setup still works.
	if w := do(h, "POST", "/v1/activate", code); w.Code != 200 {
		t.Fatalf("the code must survive the installer: %d %s", w.Code, w.Body)
	}
	// And now it is used, the installer is refused too.
	if status, _ := start(""); status != 422 {
		t.Fatalf("a used code: %d", status)
	}

	trail, _ := st.ListAudit(t.Context(), inst.ID, 50)
	var seen bool
	for _, e := range trail {
		seen = seen || e.Action == "install_started"
	}
	if !seen {
		t.Fatal("starting an install is recorded")
	}
}

func TestAnInstallIsRefusedForABadCodeARevokedKeyAndGuessing(t *testing.T) {
	h, _, _ := entitlementServerWith(t, withRegistry)
	if w := do(h, "POST", "/v1/install/start", `{"code":"nonsense"}`); w.Code != 422 {
		t.Fatalf("not a code: %d", w.Code)
	}
	inst := newInstallation(t, h)
	do(h, "POST", "/v1/installations/"+inst.ID+"/entitlements", `{`+crmSales+`}`, bearer()...)
	var made struct{ Data activationOut }
	_ = json.Unmarshal(do(h, "POST", "/v1/installations/"+inst.ID+"/activation", "", bearer()...).Body.Bytes(), &made)
	do(h, "POST", "/v1/installations/"+inst.ID+"/revoke", `{"note":"Contract ended"}`, bearer()...)
	if w := do(h, "POST", "/v1/install/start", `{"code":"`+made.Data.Code+`"}`); w.Code != 422 {
		t.Fatalf("a revoked customer gets no registry login: %d", w.Code)
	}
	for i := 0; i < guardMax; i++ {
		do(h, "POST", "/v1/install/start", `{"code":"ACT-AAAAA-BBBBB-CCCCC-DDDDD"}`)
	}
	if w := do(h, "POST", "/v1/install/start", `{"code":"ACT-AAAAA-BBBBB-CCCCC-DDDDD"}`); w.Code != 429 {
		t.Fatalf("after too many guesses: %d", w.Code)
	}
}

func TestOnlyShellSafeValuesGoIntoTheInstaller(t *testing.T) {
	for _, ok := range []string{"ghcr.io/redefynelabs", "v1.4.2", "ghp_abc123", "user-name_1"} {
		if !config.ShellSafe(ok) {
			t.Errorf("%q should be allowed", ok)
		}
	}
	for _, bad := range []string{"", "a b", "a;b", "$(x)", "a'b", "a\nb", "a`b`"} {
		if config.ShellSafe(bad) {
			t.Errorf("%q must not be allowed", bad)
		}
	}
	if got := shellText("Acme's $(rm -rf /) Ltd & Sons"); got != "Acmes rm -rf  Ltd & Sons" {
		t.Errorf("shellText = %q", got)
	}
}
