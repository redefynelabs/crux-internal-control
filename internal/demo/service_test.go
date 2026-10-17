package demo

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/config"
	"github.com/redefynelabs/erp-control-plane/internal/provision"
	"github.com/redefynelabs/erp-control-plane/internal/store"
	"github.com/redefynelabs/erp-control-plane/internal/testutil"
)

type fakeDBs struct {
	mu      sync.Mutex
	created map[string]bool
	failOn  string
	dropped []string
	modules map[string][2][]string
}

func (f *fakeDBs) Clone(_ context.Context, template, name, owner string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn == "clone" {
		return errors.New("clone failed")
	}
	f.created[name] = true
	return nil
}
func (f *fakeDBs) SetModules(_ context.Context, name string, installed, disabled []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modules = map[string][2][]string{name: {installed, disabled}}
	return nil
}
func (f *fakeDBs) Drop(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.created, name)
	f.dropped = append(f.dropped, name)
	return nil
}

type fakeStacks struct {
	mu      sync.Mutex
	running map[string]provision.StackSpec
	failOn  string
	stopped []string
}

func (f *fakeStacks) Start(_ context.Context, s provision.StackSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running[s.Slug] = s // a failed start leaves containers behind, like the real thing could
	if f.failOn == "start" {
		return errors.New("start failed")
	}
	return nil
}
func (f *fakeStacks) WaitReady(context.Context, string, time.Duration) error {
	if f.failOn == "ready" {
		return errors.New("not ready")
	}
	return nil
}
func (f *fakeStacks) Stop(_ context.Context, slug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.running, slug)
	f.stopped = append(f.stopped, slug)
	return nil
}
func (f *fakeStacks) List(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for s := range f.running {
		out = append(out, s)
	}
	return out, nil
}

type env struct {
	svc    *Service
	st     *store.Store
	dbs    *fakeDBs
	stacks *fakeStacks
	now    time.Time
}

func newEnv(t *testing.T, mutate func(*config.Demo)) *env {
	t.Helper()
	cfg := config.Demo{
		TTL: time.Hour, MaxLive: 3, MaxPerEmail: 2, MaxPerIP: 3, Domain: "demo.example.com", Scheme: "https",
		ServerImage: "erp-server:v1", WebImage: "erp-web:v1", Memory: "384m", CPUs: "0.5", PGHost: "pg:5432", PGUser: "erp_demo", PGPassword: "pw",
		Template: "erp_demo_seed", RedisURL: "redis://redis:6379/1",
	}
	if mutate != nil {
		mutate(&cfg)
	}
	e := &env{st: testutil.NewStore(t), dbs: &fakeDBs{created: map[string]bool{}}, stacks: &fakeStacks{running: map[string]provision.StackSpec{}}, now: time.Now()}
	e.svc = New(cfg, e.st, e.dbs, e.stacks, slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.svc.now = func() time.Time { return e.now }
	return e
}

func req(email string) Request {
	return Request{Name: "Asha Rao", Email: email, Phone: "+91 98765 43210", Company: "Rao Traders", IP: "203.0.113.7"}
}

func TestARequestStartsADemoAndRecordsTheLead(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	ready, err := e.svc.Request(ctx, req("asha@rao.example"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ready.URL, "https://") || !strings.HasSuffix(ready.URL, ".demo.example.com") || ready.Email != "asha@rao.example" || len(ready.Password) != 14 {
		t.Fatalf("unexpected answer: %+v", ready)
	}
	if !ready.ExpiresAt.Equal(e.now.Add(time.Hour)) {
		t.Fatalf("a demo lasts the configured time: %v", ready.ExpiresAt)
	}
	if len(e.dbs.created) != 1 || len(e.stacks.running) != 1 {
		t.Fatalf("one database and one stack: %v %v", e.dbs.created, e.stacks.running)
	}
	for slug, spec := range e.stacks.running {
		if !e.dbs.created["demo_"+slug] {
			t.Errorf("database name must follow the slug: %s", slug)
		}
		if spec.ServerEnv["DB_MAX_OPEN_CONNS"] != "6" {
			t.Errorf("a demo keeps to a few database connections: %v", spec.ServerEnv)
		}
		if spec.ServerEnv["DEMO_ADMIN_EMAIL"] != "asha@rao.example" || spec.ServerEnv["DEMO_ADMIN_PASSWORD"] != ready.Password {
			t.Errorf("the demo must be told its visitor's sign-in: %v", spec.ServerEnv)
		}
		if spec.ServerEnv["DEPLOYMENT_MODE"] != "demo" || spec.ServerEnv["REDIS_PREFIX"] != "demo-"+slug || spec.ServerEnv["MIGRATE_ON_START"] != "true" {
			t.Errorf("the server must run as a demo with its own Redis prefix: %v", spec.ServerEnv)
		}
		if !strings.Contains(spec.ServerEnv["DATABASE_URL"], "/demo_"+slug) || spec.ServerEnv["DEMO_EXPIRES_AT"] == "" {
			t.Errorf("database and expiry must be set: %v", spec.ServerEnv)
		}
		if len(spec.ServerEnv["JWT_SECRET"]) < 32 || spec.ServerEnv["JWT_SECRET"] == spec.ServerEnv["SECRETS_KEY"] {
			t.Errorf("each demo gets its own secrets")
		}
	}
	leads, _ := e.st.ListLeads(ctx, store.LeadFilter{Status: store.LeadDemoActive})
	if len(leads) != 1 || leads[0].Email != "asha@rao.example" || leads[0].Company != "Rao Traders" {
		t.Fatalf("the lead should be recorded and marked demo_active: %+v", leads)
	}
	events, _ := e.st.Events(ctx, leads[0].ID)
	if len(events) != 1 || events[0].Kind != "demo_started" {
		t.Fatalf("history: %+v", events)
	}
}

func TestInvalidRequestsAreRefusedBeforeAnythingIsCreated(t *testing.T) {
	e := newEnv(t, nil)
	for name, r := range map[string]Request{
		"no name":        {Email: "a@b.example"},
		"bad email":      {Name: "A", Email: "not-an-email"},
		"display name":   {Name: "A", Email: "Evil <a@b.example>"},
		"overlong phone": {Name: "A", Email: "a@b.example", Phone: strings.Repeat("1", 50)},
		"no phone":       {Name: "A", Email: "a@b.example"},
		"letters":        {Name: "A", Email: "a@b.example", Phone: "call me"},
		"too short":      {Name: "A", Email: "a@b.example", Phone: "12345"},
	} {
		var ve *ValidationError
		if _, err := e.svc.Request(context.Background(), r); !errors.As(err, &ve) {
			t.Errorf("%s: want a validation error, got %v", name, err)
		}
	}
	if len(e.dbs.created)+len(e.stacks.running) != 0 {
		t.Fatal("nothing may be created for an invalid request")
	}
}

func TestOneEmailGetsTwoDemosADay(t *testing.T) {
	e := newEnv(t, nil)
	for i := 0; i < 2; i++ {
		if _, err := e.svc.Request(context.Background(), req("asha@rao.example")); err != nil {
			t.Fatalf("demo %d: %v", i+1, err)
		}
	}
	if _, err := e.svc.Request(context.Background(), req("Asha@Rao.example")); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("a third demo (same email, other case) must be limited, got %v", err)
	}
	e.now = e.now.Add(25 * time.Hour)
	if _, err := e.svc.Request(context.Background(), req("asha@rao.example")); err != nil {
		t.Fatalf("a day later it works again: %v", err)
	}
}

func TestOneAddressIsLimitedAcrossEmails(t *testing.T) {
	e := newEnv(t, func(c *config.Demo) { c.MaxLive = 10 })
	for _, m := range []string{"a@x.example", "b@x.example", "c@x.example"} {
		if _, err := e.svc.Request(context.Background(), req(m)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.Request(context.Background(), req("d@x.example")); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("a fourth email from the same address must be limited, got %v", err)
	}
}

func TestNoNewDemoWhenTheRoomsAreFull(t *testing.T) {
	e := newEnv(t, func(c *config.Demo) { c.MaxLive = 2; c.MaxPerIP = 10 })
	for _, m := range []string{"a@x.example", "b@x.example"} {
		if _, err := e.svc.Request(context.Background(), req(m)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.Request(context.Background(), req("c@x.example")); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("want ErrAtCapacity, got %v", err)
	}
}

func TestAFailedStartLeavesNothingBehind(t *testing.T) {
	for _, stage := range []string{"clone", "start", "ready"} {
		e := newEnv(t, nil)
		e.dbs.failOn, e.stacks.failOn = "", ""
		if stage == "clone" {
			e.dbs.failOn = "clone"
		} else {
			e.stacks.failOn = stage
		}
		if _, err := e.svc.Request(context.Background(), req("asha@rao.example")); err == nil {
			t.Fatalf("%s: the request should fail", stage)
		}
		if len(e.dbs.created) != 0 || len(e.stacks.running) != 0 {
			t.Errorf("%s: a failed start must remove what it made: dbs=%v stacks=%v", stage, e.dbs.created, e.stacks.running)
		}
		if n, _ := e.st.CountLive(context.Background()); n != 0 {
			t.Errorf("%s: a failed demo must not count as live", stage)
		}
	}
}

func TestTheReaperRemovesExpiredDemosAndMovesTheLeadOn(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	if _, err := e.svc.Request(ctx, req("asha@rao.example")); err != nil {
		t.Fatal(err)
	}
	e.svc.Reap(ctx)
	if len(e.stacks.running) != 1 {
		t.Fatal("a demo inside its time must be left alone")
	}
	e.now = e.now.Add(2*time.Hour + time.Minute)
	e.svc.Reap(ctx)
	if len(e.stacks.running) != 0 || len(e.dbs.created) != 0 {
		t.Fatalf("an expired demo loses its containers and database: %v %v", e.stacks.running, e.dbs.created)
	}
	ended, _ := e.st.ListLeads(ctx, store.LeadFilter{Status: store.LeadDemoEnded})
	if len(ended) != 1 {
		t.Fatalf("the lead should be marked demo_ended: %+v", ended)
	}
	e.svc.Reap(ctx) // idempotent
}

func TestTheReaperDoesNotPullBackALeadThatWasWorked(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	if _, err := e.svc.Request(ctx, req("asha@rao.example")); err != nil {
		t.Fatal(err)
	}
	leads, _ := e.st.ListLeads(ctx, store.LeadFilter{})
	won := store.LeadWon
	if _, err := e.st.UpdateLead(ctx, leads[0].ID, store.LeadPatch{Status: &won}, store.Actor{}); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(3 * time.Hour)
	e.svc.Reap(ctx)
	got, _ := e.st.GetLead(ctx, leads[0].ID)
	if got.Status != store.LeadWon {
		t.Fatalf("a won lead must stay won, got %s", got.Status)
	}
}

func TestTheReaperRemovesStacksNoDemoAccountsFor(t *testing.T) {
	e := newEnv(t, nil)
	e.stacks.running["strayslug"] = provision.StackSpec{Slug: "strayslug"}
	e.svc.Reap(context.Background())
	if len(e.stacks.running) != 0 {
		t.Fatal("an orphan stack must be stopped")
	}
}

func TestEndingADemoNowAndExtendingIt(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	if _, err := e.svc.Request(ctx, req("asha@rao.example")); err != nil {
		t.Fatal(err)
	}
	demos, _ := e.st.ListDemos(ctx, store.DemoActive, 0)
	d, err := e.svc.Extend(ctx, demos[0].ID, 30*time.Minute)
	if err != nil || !d.ExpiresAt.Equal(demos[0].ExpiresAt.Add(30*time.Minute)) {
		t.Fatalf("extend: %+v %v", d, err)
	}
	d, err = e.svc.Extend(ctx, demos[0].ID, 48*time.Hour)
	if err != nil || d.ExpiresAt.After(d.CreatedAt.Add(24*time.Hour)) {
		t.Fatalf("a demo never lasts more than a day in total: %+v %v", d, err)
	}
	if err := e.svc.End(ctx, demos[0].ID); err != nil {
		t.Fatal(err)
	}
	if len(e.stacks.running) != 0 {
		t.Fatal("ending a demo removes it at once")
	}
	if _, err := e.svc.Extend(ctx, demos[0].ID, time.Hour); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("an ended demo cannot be extended, got %v", err)
	}
}

func TestEveryDemoGetsItsOwnPassword(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		p, err := loginPassword()
		if err != nil || len(p) != 14 || strings.ContainsAny(p, "0O1lI") || seen[p] {
			t.Fatalf("password %q: %v", p, err)
		}
		seen[p] = true
	}
}

func TestADemoShowsOnlyTheAppsTheVisitorPicked(t *testing.T) {
	e := newEnv(t, nil)
	r := req("asha@rao.example")
	r.Modules = []string{"sales", "inventory", "people"}
	if _, err := e.svc.Request(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if len(e.dbs.modules) != 1 {
		t.Fatalf("the copy must be told which apps to install: %v", e.dbs.modules)
	}
	for _, m := range e.dbs.modules {
		installed, disabled := m[0], m[1]
		for _, want := range []string{"crm", "sales", "inventory", "hr"} {
			if !slices.Contains(installed, want) {
				t.Errorf("%s should be installed: %v", want, installed)
			}
		}
		for _, no := range []string{"pms", "accounting"} {
			if slices.Contains(installed, no) {
				t.Errorf("%s was not asked for: %v", no, installed)
			}
		}
		// People & HR was picked without Payroll, so payroll is off inside the hr app.
		if !slices.Contains(disabled, "hr.payroll") || slices.Contains(disabled, "hr.attendance") {
			t.Errorf("only the parts of an installed app that were not picked are switched off: %v", disabled)
		}
	}
}

func TestPayrollBringsPeopleAndHR(t *testing.T) {
	_, _, apps, features := selection([]string{"payroll"})
	if !slices.Contains(apps, "hr") || !slices.Contains(features, "hr.payroll") || !slices.Contains(features, "hr.attendance") {
		t.Fatalf("payroll reads attendance, so people comes with it: %v %v", apps, features)
	}
}

func TestAnUnknownAppIsRefused(t *testing.T) {
	e := newEnv(t, nil)
	r := req("asha@rao.example")
	r.Modules = []string{"sales", "rockets"}
	_, err := e.svc.Request(context.Background(), r)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Fields["modules"] == "" {
		t.Fatalf("expected a modules error, got %v", err)
	}
}

func TestNoPicksMeansEverything(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.svc.Request(context.Background(), req("asha@rao.example")); err != nil {
		t.Fatal(err)
	}
	if len(e.dbs.modules) != 0 {
		t.Fatalf("a demo with no picks keeps every app: %v", e.dbs.modules)
	}
}
