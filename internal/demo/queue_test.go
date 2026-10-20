package demo

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/config"
	"github.com/redefynelabs/erp-control-plane/internal/store"
)

type sentMail struct{ to, subject, body string }

type fakeMailer struct {
	mu    sync.Mutex
	ready bool
	sent  []sentMail
}

func (f *fakeMailer) Ready(context.Context) bool { return f.ready }
func (f *fakeMailer) Queue(_ context.Context, _, _, to, subject, body string, _ time.Time) (store.Email, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentMail{to, subject, body})
	return store.Email{}, nil
}
func (f *fakeMailer) to(addr string) []sentMail {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []sentMail
	for _, m := range f.sent {
		if m.to == addr {
			out = append(out, m)
		}
	}
	return out
}

// fullEnv has two slots, both taken by a@ and b@, and a mailer that works.
func fullEnv(t *testing.T) (*env, *fakeMailer) {
	t.Helper()
	e := newEnv(t, func(c *config.Demo) { c.MaxLive = 2; c.MaxPerIP = 20; c.MaxQueue = 3; c.QueueWait = 2 * time.Hour })
	m := &fakeMailer{ready: true}
	e.svc.WithMailer(m)
	for _, a := range []string{"a@x.example", "b@x.example"} {
		if _, err := e.svc.Request(context.Background(), req(a)); err != nil {
			t.Fatal(err)
		}
	}
	return e, m
}

func demoOf(t *testing.T, e *env, email string) store.Demo {
	t.Helper()
	l, err := e.st.LeadByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("lead %s: %v", email, err)
	}
	ds, err := e.st.ListDemos(context.Background(), "", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		if d.LeadID == l.ID {
			return d
		}
	}
	t.Fatalf("no demo for %s", email)
	return store.Demo{}
}

func TestAVisitorIsQueuedWhenTheRoomsAreFull(t *testing.T) {
	e, m := fullEnv(t)
	ctx := context.Background()
	r, err := e.svc.Request(ctx, req("c@x.example"))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Queued || r.Position != 1 || r.URL != "" || r.Password != "" {
		t.Fatalf("want queued at 1 with no login, got %+v", r)
	}
	d := demoOf(t, e, "c@x.example")
	if d.Status != store.DemoQueued || d.QueuedAt == nil {
		t.Fatalf("want a queued row, got %+v", d)
	}
	if len(e.stacks.running) != 2 || len(e.dbs.created) != 2 {
		t.Errorf("a queued request must not build anything: dbs=%v stacks=%v", e.dbs.created, e.stacks.running)
	}
	if n, _ := e.st.CountLive(ctx); n != 2 {
		t.Errorf("a queued request must not count as live, got %d", n)
	}
	if len(m.sent) != 0 {
		t.Errorf("nothing is mailed until the demo starts, got %v", m.sent)
	}

	r2, err := e.svc.Request(ctx, req("d@x.example"))
	if err != nil || !r2.Queued || r2.Position != 2 {
		t.Fatalf("want queued at 2, got %+v, %v", r2, err)
	}
	// Asking again keeps the place and adds no second request.
	again, err := e.svc.Request(ctx, req("c@x.example"))
	if err != nil || !again.Queued || again.Position != 1 {
		t.Fatalf("a repeat request must keep its place, got %+v, %v", again, err)
	}
	if n, _ := e.st.CountQueued(ctx); n != 2 {
		t.Errorf("want 2 waiting, got %d", n)
	}
}

func TestNobodyIsQueuedWithoutWorkingMailOrRoomInTheLine(t *testing.T) {
	e, m := fullEnv(t)
	ctx := context.Background()
	m.ready = false
	if _, err := e.svc.Request(ctx, req("c@x.example")); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("without mail want ErrAtCapacity, got %v", err)
	}
	if n, _ := e.st.CountQueued(ctx); n != 0 {
		t.Errorf("nobody should be queued, got %d", n)
	}
	m.ready = true
	for _, a := range []string{"c@x.example", "d@x.example", "e@x.example"} {
		if r, err := e.svc.Request(ctx, req(a)); err != nil || !r.Queued {
			t.Fatalf("%s: %+v %v", a, r, err)
		}
	}
	if _, err := e.svc.Request(ctx, req("f@x.example")); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("a full line wants ErrAtCapacity, got %v", err)
	}
	// Someone already waiting keeps their place even when the line is full.
	if r, err := e.svc.Request(ctx, req("c@x.example")); err != nil || !r.Queued || r.Position != 1 {
		t.Fatalf("a waiting visitor must keep their place, got %+v, %v", r, err)
	}
}

func TestAFreedSlotStartsTheLongestWaitingDemoAndMailsTheLogin(t *testing.T) {
	e, m := fullEnv(t)
	ctx := context.Background()
	for _, a := range []string{"c@x.example", "d@x.example"} {
		if _, err := e.svc.Request(ctx, req(a)); err != nil {
			t.Fatal(err)
		}
		e.now = e.now.Add(time.Minute) // distinct queue times
	}
	e.svc.Dispatch(ctx)
	if demoOf(t, e, "c@x.example").Status != store.DemoQueued {
		t.Fatal("with every slot busy nothing may start")
	}

	if err := e.svc.End(ctx, demoOf(t, e, "a@x.example").ID); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(30 * time.Minute) // the wait does not eat into the hour
	e.svc.Dispatch(ctx)

	c := demoOf(t, e, "c@x.example")
	if c.Status != store.DemoActive || !strings.HasPrefix(c.URL, "https://"+c.Slug+".demo.example.com") {
		t.Fatalf("c is first in line and should be running, got %+v", c)
	}
	// Postgres keeps microseconds; the clock may have more.
	if d := c.ExpiresAt.Sub(e.now.Add(time.Hour)); d < -time.Millisecond || d > time.Millisecond {
		t.Errorf("the hour starts when the demo starts: want %v, got %v", e.now.Add(time.Hour), c.ExpiresAt)
	}
	if demoOf(t, e, "d@x.example").Status != store.DemoQueued {
		t.Error("d must keep waiting: only one slot opened")
	}
	mails := m.to("c@x.example")
	if len(mails) != 1 || !strings.Contains(mails[0].body, c.URL) || !strings.Contains(mails[0].body, "c@x.example") {
		t.Fatalf("c must be mailed the address and login, got %v", mails)
	}
	spec := e.stacks.running[c.Slug]
	if pw := spec.ServerEnv["DEMO_ADMIN_PASSWORD"]; pw == "" || !strings.Contains(mails[0].body, pw) {
		t.Errorf("the mailed password must be the demo's own")
	}
	if spec.ServerEnv["DEMO_ADMIN_EMAIL"] != "c@x.example" {
		t.Errorf("the demo must sign in the requester, got %q", spec.ServerEnv["DEMO_ADMIN_EMAIL"])
	}
	if l, _ := e.st.LeadByEmail(ctx, "c@x.example"); l.Status != store.LeadDemoActive {
		t.Errorf("lead should be demo_active, got %s", l.Status)
	}
	if n, _ := e.st.CountLive(ctx); n != 2 {
		t.Errorf("want 2 live, got %d", n)
	}
}

func TestANewcomerDoesNotJumpTheLine(t *testing.T) {
	e, _ := fullEnv(t)
	ctx := context.Background()
	if _, err := e.svc.Request(ctx, req("c@x.example")); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.End(ctx, demoOf(t, e, "a@x.example").ID); err != nil {
		t.Fatal(err)
	}
	r, err := e.svc.Request(ctx, req("d@x.example"))
	if err != nil || !r.Queued || r.Position != 2 {
		t.Fatalf("a slot is free but c waits first: d must queue at 2, got %+v, %v", r, err)
	}
}

func TestNothingStartsWhileMailIsDown(t *testing.T) {
	e, m := fullEnv(t)
	ctx := context.Background()
	if _, err := e.svc.Request(ctx, req("c@x.example")); err != nil {
		t.Fatal(err)
	}
	_ = e.svc.End(ctx, demoOf(t, e, "a@x.example").ID)
	m.ready = false
	e.svc.Dispatch(ctx)
	if demoOf(t, e, "c@x.example").Status != store.DemoQueued {
		t.Fatal("a demo whose login cannot be mailed must not start")
	}
	m.ready = true
	e.svc.Dispatch(ctx)
	if demoOf(t, e, "c@x.example").Status != store.DemoActive {
		t.Fatal("it should start once mail works again")
	}
}

func TestAQueuedRequestGivesUpAfterTheWaitAndIsToldSo(t *testing.T) {
	e, m := fullEnv(t)
	ctx := context.Background()
	if _, err := e.svc.Request(ctx, req("c@x.example")); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(2*time.Hour + time.Minute)
	e.svc.Dispatch(ctx)
	c := demoOf(t, e, "c@x.example")
	if c.Status != store.DemoAbandoned {
		t.Fatalf("want abandoned, got %s", c.Status)
	}
	if mails := m.to("c@x.example"); len(mails) != 1 || !strings.Contains(mails[0].body, "Sorry") {
		t.Fatalf("want one apology, got %v", mails)
	}
	if n, _ := e.st.CountQueued(ctx); n != 0 {
		t.Errorf("the line should be empty, got %d", n)
	}
}

func TestEndingAQueuedRequestCancelsIt(t *testing.T) {
	e, m := fullEnv(t)
	ctx := context.Background()
	_, _ = e.svc.Request(ctx, req("c@x.example"))
	if err := e.svc.End(ctx, demoOf(t, e, "c@x.example").ID); err != nil {
		t.Fatal(err)
	}
	if s := demoOf(t, e, "c@x.example").Status; s != store.DemoAbandoned {
		t.Fatalf("want abandoned, got %s", s)
	}
	_ = e.svc.End(ctx, demoOf(t, e, "a@x.example").ID)
	e.svc.Dispatch(ctx)
	if len(m.to("c@x.example")) != 0 || len(e.stacks.running) != 1 {
		t.Errorf("a cancelled request must never start: mails=%v stacks=%v", m.sent, e.stacks.running)
	}
}

func TestAQueuedDemoThatFailsToStartIsRetriedInPlace(t *testing.T) {
	e, m := fullEnv(t)
	ctx := context.Background()
	for _, a := range []string{"c@x.example", "d@x.example"} {
		_, _ = e.svc.Request(ctx, req(a))
		e.now = e.now.Add(time.Minute)
	}
	_ = e.svc.End(ctx, demoOf(t, e, "a@x.example").ID)

	e.dbs.failOn = "clone"
	e.svc.Dispatch(ctx)
	if s := demoOf(t, e, "c@x.example").Status; s != store.DemoQueued {
		t.Fatalf("a first failure puts c back in line, got %s", s)
	}
	if s := demoOf(t, e, "d@x.example").Status; s != store.DemoQueued {
		t.Fatalf("a failure must stop the pass, d got %s", s)
	}
	if len(m.to("c@x.example")) != 0 {
		t.Error("nobody is told about a retry")
	}
	if pos, _ := e.st.QueuePosition(ctx, demoOf(t, e, "c@x.example")); pos != 1 {
		t.Errorf("c keeps first place, got %d", pos)
	}

	e.dbs.failOn = ""
	e.svc.Dispatch(ctx)
	if s := demoOf(t, e, "c@x.example").Status; s != store.DemoActive {
		t.Fatalf("c should start once the fault is gone, got %s", s)
	}
}

func TestAQueuedDemoThatKeepsFailingIsGivenUpAndTheLineMovesOn(t *testing.T) {
	e, m := fullEnv(t)
	ctx := context.Background()
	for _, a := range []string{"c@x.example", "d@x.example"} {
		_, _ = e.svc.Request(ctx, req(a))
		e.now = e.now.Add(time.Minute)
	}
	_ = e.svc.End(ctx, demoOf(t, e, "a@x.example").ID)
	e.dbs.failOn = "clone"
	for i := 0; i < maxStartAttempts; i++ {
		e.svc.Dispatch(ctx)
	}
	if s := demoOf(t, e, "c@x.example").Status; s != store.DemoAbandoned {
		t.Fatalf("c should be given up after %d tries, got %s", maxStartAttempts, s)
	}
	if mails := m.to("c@x.example"); len(mails) != 1 || !strings.Contains(mails[0].body, "Sorry") {
		t.Fatalf("c should be told once, got %v", mails)
	}
	e.dbs.failOn = ""
	e.svc.Dispatch(ctx)
	if s := demoOf(t, e, "d@x.example").Status; s != store.DemoActive {
		t.Fatalf("d should start next, got %s", s)
	}
}
