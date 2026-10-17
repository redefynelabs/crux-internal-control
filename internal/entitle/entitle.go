// Package entitle makes the signed entitlement files that tell an ERP installation which apps, features and record words
// it has. The format is the ERP's (erp-software/docs/entitlements.md): a JSON payload and an Ed25519 signature over its
// canonical form (keys sorted, no whitespace). The private key never leaves the key file.
package entitle

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// App is an app and the features it can grant.
type App struct {
	Name     string    `json:"name"`
	Label    string    `json:"label"`
	Features []Feature `json:"features"`
	// Needs are apps this one cannot run without (accounting posts payments against sales invoices).
	Needs []string `json:"needs,omitempty"`
}

// Group is a business outcome the vendor ticks in the editor: it stands for the apps and features that deliver it. A group
// is only an editing convenience; what is signed is always apps and features, so groups can change without touching an
// installation.
type Group struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Text     string   `json:"text"`
	Apps     []string `json:"apps"`
	Features []string `json:"features"`
}

// Feature is one switchable part of an app.
type Feature struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// Template is an industry template an installation starts from. The list is the ERP's own (templates.json, kept equal
// by a test in erp-software), so the editor can start a customer from what the industry gets.
type Template struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Summary  string           `json:"summary"`
	Apps     []string         `json:"apps"`
	Features []string         `json:"features"`
	Labels   map[string]Label `json:"labels"`
}

//go:embed templates.json
var templatesJSON []byte

func loadTemplates() []Template {
	var out []Template
	if err := json.Unmarshal(templatesJSON, &out); err != nil {
		panic("entitle: templates.json: " + err.Error())
	}
	return out
}

// Record is one of the seven records whose words a bundle sets.
type Record struct {
	Key      string `json:"key"`
	Singular string `json:"singular"`
	Plural   string `json:"plural"`
}

// Catalog is what the ERP build knows. An older or newer ERP ignores names it does not know, so this list may run ahead of
// or behind an installation.
var Catalog = struct {
	// Core are features of the core, which is always on but can be optional per customer. They are named without an app.
	Core      []Feature  `json:"core"`
	Groups    []Group    `json:"groups"`
	Apps      []App      `json:"apps"`
	Templates []Template `json:"templates"`
	Records   []Record   `json:"records"`
}{
	Core: []Feature{{Name: "clients.documents", Label: "Client documents"}},
	Apps: []App{
		{Name: "crm", Label: "CRM", Features: []Feature{
			{"crm.contacts", "Contacts"}, {"crm.deals", "Deals and pipelines"}, {"crm.leads", "Leads"},
			{"crm.followups", "Follow-ups"}, {"crm.meetings", "Meetings"}, {"crm.import", "Import"}}},
		{Name: "sales", Label: "Sales", Features: []Feature{
			{"sales.products", "Product catalog"}, {"sales.quotes", "Quotes"}, {"sales.orders", "Orders"},
			{"sales.invoices", "Invoices"}, {"sales.recurring", "Recurring invoices"}, {"sales.reports", "Sales reports"}}},
		{Name: "accounting", Label: "Accounting", Needs: []string{"sales"}, Features: []Feature{
			{"accounting.receipts", "Receipts"}, {"accounting.expenses", "Expenses"}, {"accounting.banking", "Banking"}, {"accounting.reports", "Accounting reports"}}},
		{Name: "inventory", Label: "Inventory", Features: []Feature{
			{"inventory.stock", "Stock"}, {"inventory.purchasing", "Purchasing"}}},
		{Name: "pms", Label: "Projects", Features: []Feature{{"pms.tasks", "Tasks"}}},
		{Name: "hr", Label: "HR", Features: []Feature{
			{"hr.attendance", "Attendance"}, {"hr.leave", "Leave"}, {"hr.shifts", "Shifts"}, {"hr.timesheets", "Timesheets"}, {"hr.payroll", "Payroll"}}},
	},
	Groups: []Group{
		{Key: "crm", Label: "CRM & client follow-ups", Text: "Contacts, leads, deals, follow-ups, meetings and import.",
			Apps: []string{"crm"}, Features: []string{"crm.contacts", "crm.leads", "crm.deals", "crm.followups", "crm.meetings", "crm.import"}},
		{Key: "sales", Label: "Sales & orders", Text: "Product catalog, quotes and orders.",
			Apps: []string{"sales"}, Features: []string{"sales.products", "sales.quotes", "sales.orders", "sales.reports"}},
		{Key: "projects", Label: "Projects & tasks", Text: "Projects, boards and task tracking.",
			Apps: []string{"pms"}, Features: []string{"pms.tasks"}},
		{Key: "people", Label: "People & HR", Text: "Attendance, leave, shifts and timesheets.",
			Apps: []string{"hr"}, Features: []string{"hr.attendance", "hr.leave", "hr.shifts", "hr.timesheets"}},
		{Key: "payroll", Label: "Payroll", Text: "Salary structures, monthly pay runs with approval, and payslips. Reads attendance and leave, so choose People & HR with it.",
			Apps: []string{"hr"}, Features: []string{"hr.payroll"}},
		{Key: "finance", Label: "Invoices & expenses", Text: "Invoices and recurring invoices, receipts, expenses, banking and reports. Brings the Sales app for invoices.",
			Apps: []string{"sales", "accounting"}, Features: []string{"sales.invoices", "sales.recurring", "accounting.receipts", "accounting.expenses", "accounting.banking", "accounting.reports"}},
		{Key: "inventory", Label: "Inventory & purchasing", Text: "Stock in warehouses, movements, counts and purchase orders.",
			Apps: []string{"inventory"}, Features: []string{"inventory.stock", "inventory.purchasing"}},
	},
	Templates: loadTemplates(),
	Records: []Record{
		{"client", "Client", "Clients"}, {"project", "Project", "Projects"}, {"lead", "Lead", "Leads"}, {"deal", "Deal", "Deals"},
		{"task", "Task", "Tasks"}, {"quote", "Quote", "Quotes"}, {"order", "Order", "Orders"}},
}

// Names returns every app and feature name of the catalog.
// Select returns the apps and features of the named groups, with the apps those apps need (accounting needs sales).
// Unknown group keys are skipped. The core features are always included.
func Select(groups []string) (apps, features []string) {
	want := map[string]bool{}
	feat := map[string]bool{}
	for _, f := range Catalog.Core {
		feat[f.Name] = true
	}
	for _, g := range Catalog.Groups {
		if !slices.Contains(groups, g.Key) {
			continue
		}
		for _, a := range g.Apps {
			want[a] = true
		}
		for _, f := range g.Features {
			feat[f] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, a := range Catalog.Apps {
			if !want[a.Name] {
				continue
			}
			for _, n := range a.Needs {
				if !want[n] {
					want[n], changed = true, true
				}
			}
		}
	}
	for _, a := range Catalog.Apps {
		if want[a.Name] {
			apps = append(apps, a.Name)
		}
	}
	for f := range feat {
		features = append(features, f)
	}
	slices.Sort(features)
	return apps, features
}

func Names() (apps, features []string) {
	for _, f := range Catalog.Core {
		features = append(features, f.Name)
	}
	for _, a := range Catalog.Apps {
		apps = append(apps, a.Name)
		for _, f := range a.Features {
			features = append(features, f.Name)
		}
	}
	return
}

// Label is the word for a record.
type Label struct {
	Singular string `json:"singular"`
	Plural   string `json:"plural"`
}

// Declined is a request the vendor turned down, with a note the customer sees.
type Declined struct {
	Name string `json:"name"`
	Note string `json:"note"`
}

// Customer names who the bundle is for.
type Customer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Licence kinds. A lifetime licence never expires; a subscription ends at ExpiresAt.
const (
	LicenseSubscription = "subscription"
	LicenseLifetime     = "lifetime"
)

// License is the kind of licence.
type License struct {
	Type string `json:"type"`
}

// Payload is what is signed. The field set must match the ERP's, which refuses unknown fields.
type Payload struct {
	Version   int        `json:"version"`
	IssuedAt  time.Time  `json:"issued_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// License, UpdatesUntil, Suspended and ControlPlaneURL are read by ERP releases that know them; do not issue them
	// to an installation older than that release.
	License         *License         `json:"license,omitempty"`
	UpdatesUntil    *time.Time       `json:"updates_until,omitempty"`
	Suspended       bool             `json:"suspended,omitempty"`
	ControlPlaneURL string           `json:"control_plane_url,omitempty"`
	Customer        Customer         `json:"customer"`
	Template        string           `json:"template"`
	Apps            []string         `json:"apps"`
	Features        []string         `json:"features"`
	Labels          map[string]Label `json:"labels"`
	Declined        []Declined       `json:"declined,omitempty"`
}

// Problem is a reason a payload cannot be issued; Fields names the form fields at fault.
type Problem struct{ Fields map[string]string }

func (p *Problem) Error() string { return "entitle: the entitlement is not valid" }

// Validate checks a payload against the catalog and tidies it (sorted, no duplicates). Names the catalog does not have are
// refused here, unlike in the ERP, because the vendor should never grant what it cannot name.
func (p *Payload) Validate() error {
	f := map[string]string{}
	apps, features := Names()
	p.Apps, p.Features = unique(p.Apps), unique(p.Features)
	for _, a := range p.Apps {
		if !slices.Contains(apps, a) {
			f["apps"] = "Unknown app " + a
		}
	}
	for _, name := range p.Features {
		app, _, _ := strings.Cut(name, ".")
		switch {
		case !slices.Contains(features, name):
			f["features"] = "Unknown feature " + name
		case app != "clients" && !slices.Contains(p.Apps, app):
			f["features"] = name + " needs the " + app + " app"
		}
	}
	if !slices.ContainsFunc(Catalog.Templates, func(t Template) bool { return t.ID == p.Template }) {
		f["template"] = "Choose a template"
	}
	for k, l := range p.Labels {
		if !slices.ContainsFunc(Catalog.Records, func(r Record) bool { return r.Key == k }) {
			f["labels"] = "Unknown record " + k
		} else if n, m := len([]rune(l.Singular)), len([]rune(l.Plural)); n < 1 || n > 40 || m < 1 || m > 40 {
			f["labels"] = "The words for " + k + " must be 1 to 40 characters"
		}
	}
	if p.ExpiresAt != nil && !p.ExpiresAt.After(p.IssuedAt) {
		f["expires_at"] = "Must be in the future"
	}
	if p.License != nil {
		switch {
		case p.License.Type != LicenseSubscription && p.License.Type != LicenseLifetime:
			f["license"] = "Choose subscription or lifetime"
		case p.License.Type == LicenseLifetime && p.ExpiresAt != nil:
			f["expires_at"] = "A lifetime licence does not end"
		case p.License.Type == LicenseSubscription && p.ExpiresAt == nil:
			f["expires_at"] = "A subscription needs an end date"
		}
	}
	if p.ControlPlaneURL != "" && !strings.HasPrefix(p.ControlPlaneURL, "https://") {
		f["control_plane_url"] = "Must start with https://"
	}
	// An app cannot be granted without the apps it needs.
	for _, a := range Catalog.Apps {
		if !slices.Contains(p.Apps, a.Name) {
			continue
		}
		for _, need := range a.Needs {
			if !slices.Contains(p.Apps, need) {
				f["apps"] = a.Label + " needs the " + need + " app"
			}
		}
	}
	if len(f) > 0 {
		return &Problem{Fields: f}
	}
	return nil
}

func unique(in []string) []string {
	out := slices.Clone(in)
	if out == nil {
		out = []string{}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// Bundle is the file.
type Bundle struct {
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}

// Canonical is the form that is signed: keys sorted, no whitespace.
func Canonical(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

// Signer holds the vendor's private key.
type Signer struct{ priv ed25519.PrivateKey }

// Sign makes the file for a payload: the canonical payload and its signature.
func (s *Signer) Sign(p Payload) (file []byte, payload []byte, err error) {
	if p.Labels == nil {
		p.Labels = map[string]Label{}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, nil, err
	}
	canon, err := Canonical(raw)
	if err != nil {
		return nil, nil, err
	}
	sig := ed25519.Sign(s.priv, canon)
	file, err = json.MarshalIndent(Bundle{Payload: canon, Signature: base64.StdEncoding.EncodeToString(sig)}, "", "  ")
	return file, canon, err
}

// PublicKey is the base64 key to put in an installation's ENTITLEMENT_PUBLIC_KEY.
func (s *Signer) PublicKey() string {
	return base64.StdEncoding.EncodeToString(s.priv.Public().(ed25519.PublicKey))
}

// ErrKeyPermissions: the key file can be read by others.
var ErrKeyPermissions = errors.New("entitle: the key file must be readable only by its owner (chmod 600)")

// GenerateKey writes a new key file (base64 of the private key, mode 0600) and refuses to replace one.
func GenerateKey(path string) (*Signer, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("entitle: %s already exists; a new key would stop every installation from accepting new files", path)
		}
		return nil, err
	}
	defer f.Close()
	if _, err := f.WriteString(base64.StdEncoding.EncodeToString(priv) + "\n"); err != nil {
		return nil, err
	}
	return &Signer{priv: priv}, nil
}

// LoadKey reads a key file. It refuses one that group or others can read.
func LoadKey(path string) (*Signer, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("entitle: cannot read the key file: %w", err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		return nil, ErrKeyPermissions
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("entitle: the key file does not hold an Ed25519 private key")
	}
	return &Signer{priv: ed25519.PrivateKey(b)}, nil
}

// Secret is the credential an installation signs its calls with. It is derived from the control plane's secrets key, the
// installation id and a generation number, so nothing needs storing and rotating means raising the generation.
func Secret(secretsKey, installationID string, generation int) string {
	mac := hmac.New(sha256.New, []byte(secretsKey))
	fmt.Fprintf(mac, "installation-secret\n%s\n%d", installationID, generation)
	return hex.EncodeToString(mac.Sum(nil))
}

// MaxSkew is how far a request's timestamp may be from now.
const MaxSkew = 5 * time.Minute

// RequestSignature is the ERP's: hex HMAC-SHA256 over id, timestamp, method, path and the body's SHA-256, joined by newlines.
func RequestSignature(secret, id, timestamp, method, path string, body []byte) string {
	sum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s\n%s\n%s\n%s\n%s", id, timestamp, method, path, hex.EncodeToString(sum[:]))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyRequest checks a call from an installation.
func VerifyRequest(secret, id, timestamp, signature, method, path string, body []byte, now time.Time) bool {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if d := now.Sub(time.Unix(ts, 0)); d > MaxSkew || d < -MaxSkew {
		return false
	}
	want := RequestSignature(secret, id, timestamp, method, path, body)
	return hmac.Equal([]byte(want), []byte(strings.ToLower(signature)))
}

func hashHex(s string) string {
	sum := sha256.Sum256([]byte("activation\n" + s))
	return hex.EncodeToString(sum[:])
}
