package entitle

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func testSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := GenerateKey(filepath.Join(t.TempDir(), "key"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSignedFileVerifiesAgainstThePublicKey(t *testing.T) {
	s := testSigner(t)
	p := Payload{Version: 1, IssuedAt: time.Now().UTC(), Customer: Customer{ID: "c", Name: "Acme"}, Template: "clinic",
		Apps: []string{"sales", "crm"}, Features: []string{"sales.quotes", "clients.documents"}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	file, _, err := s.Sign(p)
	if err != nil {
		t.Fatal(err)
	}
	var b Bundle
	if err := json.Unmarshal(file, &b); err != nil {
		t.Fatal(err)
	}
	sig, _ := base64.StdEncoding.DecodeString(b.Signature)
	pub, _ := base64.StdEncoding.DecodeString(s.PublicKey())
	canon, _ := Canonical(b.Payload)
	if !ed25519.Verify(pub, canon, sig) {
		t.Fatal("the signature does not verify")
	}
	if ed25519.Verify(pub, append(canon, ' '), sig) {
		t.Fatal("a changed payload still verifies")
	}
	var back Payload
	if err := json.Unmarshal(b.Payload, &back); err != nil || back.Apps[0] != "crm" || back.Labels == nil {
		t.Fatalf("apps sorted and labels an object: %+v %v", back, err)
	}
}

func TestValidateRefusals(t *testing.T) {
	base := Payload{Version: 1, IssuedAt: time.Now(), Template: "agency", Apps: []string{"crm"}, Features: []string{"crm.leads"}}
	past := time.Now().Add(-time.Hour)
	cases := map[string]func(p *Payload){
		"apps":       func(p *Payload) { p.Apps = []string{"nope"} },
		"features":   func(p *Payload) { p.Features = []string{"sales.quotes"} }, // app not granted
		"template":   func(p *Payload) { p.Template = "" },
		"labels":     func(p *Payload) { p.Labels = map[string]Label{"client": {Singular: "", Plural: "X"}} },
		"expires_at": func(p *Payload) { p.ExpiresAt = &past },
	}
	if err := (&base).Validate(); err != nil {
		t.Fatalf("a valid payload: %v", err)
	}
	for field, mutate := range cases {
		p := base
		mutate(&p)
		err := p.Validate()
		pr, ok := err.(*Problem)
		if !ok || pr.Fields[field] == "" {
			t.Errorf("%s: want a problem on that field, got %v", field, err)
		}
	}
}

func TestKeyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	s, err := GenerateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateKey(path); err == nil {
		t.Fatal("a second keygen replaced the key")
	}
	again, err := LoadKey(path)
	if err != nil || again.PublicKey() != s.PublicKey() {
		t.Fatalf("load: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(path); err != ErrKeyPermissions {
		t.Fatalf("a world-readable key must be refused, got %v", err)
	}
}

func TestRequestSignature(t *testing.T) {
	now := time.Now()
	stamp := func(at time.Time) string { return itoa(at.Unix()) }
	sig := RequestSignature("s3cret", "id-1", stamp(now), "POST", "/v1/access-requests", []byte(`{"a":1}`))
	if !VerifyRequest("s3cret", "id-1", stamp(now), sig, "POST", "/v1/access-requests", []byte(`{"a":1}`), now) {
		t.Fatal("a good request is refused")
	}
	for name, ok := range map[string]bool{
		"body":   VerifyRequest("s3cret", "id-1", stamp(now), sig, "POST", "/v1/access-requests", []byte(`{"a":2}`), now),
		"path":   VerifyRequest("s3cret", "id-1", stamp(now), sig, "POST", "/v1/other", []byte(`{"a":1}`), now),
		"secret": VerifyRequest("other", "id-1", stamp(now), sig, "POST", "/v1/access-requests", []byte(`{"a":1}`), now),
		"stale":  VerifyRequest("s3cret", "id-1", stamp(now), sig, "POST", "/v1/access-requests", []byte(`{"a":1}`), now.Add(time.Hour)),
	} {
		if ok {
			t.Errorf("%s: a changed request was accepted", name)
		}
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestSecretDependsOnGeneration(t *testing.T) {
	if Secret("k", "a", 1) == Secret("k", "a", 2) || Secret("k", "a", 1) == Secret("k", "b", 1) || Secret("k", "a", 1) != Secret("k", "a", 1) {
		t.Fatal("the secret must be stable per installation and generation, and differ otherwise")
	}
}

// A template names apps and features that the editor can show; the ones it names ahead of the ERP (inventory) are the only
// exceptions. Everything else a template switches on must be grantable.
func TestTemplatesNameThingsTheCatalogHas(t *testing.T) {
	apps, features := Names()
	if len(Catalog.Templates) == 0 {
		t.Fatal("no templates")
	}
	for _, tpl := range Catalog.Templates {
		for _, a := range tpl.Apps {
			if a != "inventory" && a != "payroll" && !slices.Contains(apps, a) {
				t.Errorf("%s: app %s is not in the catalog", tpl.ID, a)
			}
		}
		for _, f := range tpl.Features {
			if !strings.HasPrefix(f, "inventory.") && !strings.HasPrefix(f, "payroll.") && !slices.Contains(features, f) {
				t.Errorf("%s: feature %s is not in the catalog", tpl.ID, f)
			}
		}
		if len(tpl.Labels) != len(Catalog.Records) {
			t.Errorf("%s: words for %d records, want %d", tpl.ID, len(tpl.Labels), len(Catalog.Records))
		}
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.2.0", "1.10.0", -1}, {"v1.0.0", "1.0.0", 0}, {"2.0", "2.0.0", 0}, {"1.0.0-rc1", "1.0.0", -1},
		{"1.0.1", "1.0.0-rc1", 1}, {"1.0.0-rc1", "1.0.0-rc2", -1},
	} {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("%s vs %s = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if !ValidVersion("1.4.2") || ValidVersion("latest") || ValidVersion("") {
		t.Error("ValidVersion")
	}
}

func TestActivationCodes(t *testing.T) {
	code, hash, hint, err := NewActivationCode()
	if err != nil || len(code) != 27 || len(hint) != 5 || !strings.HasSuffix(code, hint) {
		t.Fatalf("%q %q %v", code, hint, err)
	}
	for _, typed := range []string{code, strings.ToLower(code), strings.ReplaceAll(code, "-", ""), " " + code + " ", strings.TrimPrefix(code, "ACT-")} {
		if got, err := HashActivationCode(typed); err != nil || got != hash {
			t.Errorf("%q: %v", typed, err)
		}
	}
	for _, bad := range []string{"", "ACT-1234", "ACT-AAAAA-BBBBB-CCCCC-DDDD0", "hello world"} {
		if _, err := HashActivationCode(bad); err == nil {
			t.Errorf("%q should not be a code", bad)
		}
	}
	other, otherHash, _, _ := NewActivationCode()
	if other == code || otherHash == hash {
		t.Error("codes repeat")
	}
}
