package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/store"
)

// queuedRequest is what a waiting visitor chose, kept on the demo row until a slot is free.
type queuedRequest struct {
	Modules     []string          `json:"modules,omitempty"`
	TeamSize    string            `json:"team_size,omitempty"`
	Focus       string            `json:"focus,omitempty"`
	Attribution map[string]string `json:"attribution,omitempty"`
	// Attempts counts failed starts; after maxStartAttempts the request is given up.
	Attempts int `json:"attempts,omitempty"`
}

// maxStartAttempts is how often a queued demo is tried before the visitor is told it cannot be started. A failed try puts
// it back in line, so a problem that affects everyone (a missing template, Docker down) is retried and does not apologise
// to the whole line at once.
const maxStartAttempts = 3

// enqueue puts the visitor in line. It turns them away (ErrAtCapacity) when waiting is off, when mail cannot go out (the
// login would never reach them) or when the line is full. A visitor already in line keeps their place.
func (s *Service) enqueue(ctx context.Context, r Request, email string, waiting int) (Ready, error) {
	var zero Ready
	if s.cfg.MaxQueue < 1 || s.mailer == nil || !s.mailer.Ready(ctx) || waiting >= s.cfg.MaxQueue {
		// A visitor who is already waiting keeps their place even if the line is now full.
		if d, ok := s.waitingByEmail(ctx, email); ok {
			return s.queuedReady(ctx, d)
		}
		return zero, ErrAtCapacity
	}
	lead, created, err := s.store.UpsertLead(ctx, store.LeadInput{Name: strings.TrimSpace(r.Name), Email: email, Phone: strings.TrimSpace(r.Phone), Company: strings.TrimSpace(r.Company), Source: "website", TeamSize: r.TeamSize})
	if err != nil {
		return zero, err
	}
	if d, ok, err := s.store.QueuedDemoForLead(ctx, lead.ID); err != nil {
		return zero, err
	} else if ok {
		return s.queuedReady(ctx, d)
	}
	slug, err := newSlug()
	if err != nil {
		return zero, err
	}
	body, err := json.Marshal(queuedRequest{Modules: r.Modules, TeamSize: r.TeamSize, Focus: r.Focus, Attribution: CleanAttribution(r.Attribution)})
	if err != nil {
		return zero, err
	}
	d, err := s.store.CreateQueuedDemo(ctx, lead.ID, slug, r.IP, s.now().Add(s.cfg.QueueWait), body)
	if err != nil {
		return zero, err
	}
	ready, err := s.queuedReady(ctx, d)
	if err != nil {
		return zero, err
	}
	if s.intake != nil {
		s.intake.NewLead(ctx, lead, created, false)
	}
	_ = s.store.AddEvent(ctx, lead.ID, "demo_queued", map[string]any{"slug": slug, "position": ready.Position, "new_lead": created, "team_size": r.TeamSize, "focus": r.Focus, "modules": r.Modules, "attribution": CleanAttribution(r.Attribution)})
	s.log.Info("demo queued", "slug", slug, "position", ready.Position)
	s.notify.Send(ctx, fmt.Sprintf("Demo queued (all rooms busy, #%d): %s <%s>%s", ready.Position, lead.Name, lead.Email, companySuffix(lead.Company)))
	return ready, nil
}

func (s *Service) queuedReady(ctx context.Context, d store.Demo) (Ready, error) {
	pos, err := s.store.QueuePosition(ctx, d)
	if err != nil {
		return Ready{}, err
	}
	return Ready{Queued: true, Position: pos}, nil
}

func (s *Service) waitingByEmail(ctx context.Context, email string) (store.Demo, bool) {
	l, err := s.store.LeadByEmail(ctx, email)
	if err != nil {
		return store.Demo{}, false
	}
	d, ok, err := s.store.QueuedDemoForLead(ctx, l.ID)
	return d, ok && err == nil
}

// RunDispatcher starts waiting demos as slots free up, every interval until ctx ends.
func (s *Service) RunDispatcher(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	s.Dispatch(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Dispatch(ctx)
		}
	}
}

// Dispatch gives up on requests that waited too long, then starts the longest waiting ones while there is room. It is
// safe to call at any time.
func (s *Service) Dispatch(ctx context.Context) {
	s.dispatchMu.Lock()
	defer s.dispatchMu.Unlock()
	s.expireQueued(ctx)
	for ctx.Err() == nil {
		live, err := s.store.CountLive(ctx)
		if err != nil {
			s.log.Error("dispatcher: counting live demos", "error", err)
			return
		}
		// Without mail the login cannot be delivered; the visitors keep waiting until it works or their time runs out.
		if live >= s.cfg.MaxLive || s.mailer == nil || !s.mailer.Ready(ctx) {
			return
		}
		d, ok, err := s.store.ClaimNextQueued(ctx, s.now().Add(s.cfg.TTL))
		if err != nil {
			s.log.Error("dispatcher: claiming a queued demo", "error", err)
			return
		}
		if !ok {
			return
		}
		if !s.startQueued(ctx, d) {
			return // try again on the next pass
		}
	}
}

// startQueued builds the demo for a claimed request and mails the visitor their login. It reports whether the demo
// started.
func (s *Service) startQueued(ctx context.Context, d store.Demo) bool {
	var qr queuedRequest
	_ = json.Unmarshal(d.Request, &qr)
	lead, err := s.store.GetLead(ctx, d.LeadID)
	if err != nil {
		s.failQueued(ctx, d, store.Lead{ID: d.LeadID}, qr, err)
		return false
	}
	password, err := loginPassword()
	if err != nil {
		s.failQueued(ctx, d, lead, qr, err)
		return false
	}
	url, err := s.provision(ctx, d, d.ExpiresAt, lead.Email, password, qr.Modules)
	if err == nil {
		// The login goes by mail only; if that cannot be queued the demo is useless to the visitor.
		subject, body := readyMail(lead.Name, url, lead.Email, password, d.ExpiresAt, s.cfg.TTL)
		_, err = s.mailer.Queue(ctx, lead.ID, "", lead.Email, subject, body, s.now())
	}
	if err == nil {
		err = s.store.MarkDemoActive(ctx, d.ID, url)
	}
	if err != nil {
		s.failQueued(ctx, d, lead, qr, err)
		return false
	}
	s.issueEntitlement(ctx, d, lead, d.ExpiresAt, qr.Modules)
	_ = s.store.SetLeadStatus(ctx, lead.ID, store.LeadDemoActive, store.LeadNew, store.LeadDemoEnded)
	_ = s.store.AddEvent(ctx, lead.ID, "demo_started", map[string]any{"slug": d.Slug, "expires_at": d.ExpiresAt, "from_queue": true, "waited": s.now().Sub(*d.QueuedAt).Round(time.Second).String(), "team_size": qr.TeamSize, "focus": qr.Focus, "modules": qr.Modules})
	s.log.Info("queued demo started", "slug", d.Slug, "expires_at", d.ExpiresAt)
	s.notify.Send(ctx, fmt.Sprintf("Queued demo started: %s <%s>%s", lead.Name, lead.Email, companySuffix(lead.Company)))
	return true
}

// failQueued removes what a queued demo's failed start made. The request goes back in line, keeping its place, until it
// has failed maxStartAttempts times; then it is failed and the visitor is told to ask again.
func (s *Service) failQueued(ctx context.Context, d store.Demo, lead store.Lead, qr queuedRequest, cause error) {
	ctx = context.WithoutCancel(ctx)
	qr.Attempts++
	s.log.Error("queued demo failed to start", "slug", d.Slug, "attempt", qr.Attempts, "error", cause)
	s.teardown(ctx, d) //nolint:errcheck // the reaper retries
	if qr.Attempts < maxStartAttempts && d.QueuedAt != nil {
		body, err := json.Marshal(qr)
		if err == nil && s.store.RequeueDemo(ctx, d.ID, d.QueuedAt.Add(s.cfg.QueueWait), body) == nil {
			return
		}
	}
	_, _ = s.store.AbandonQueued(ctx, d.ID, cause.Error())
	_ = s.store.AddEvent(ctx, d.LeadID, "demo_failed", map[string]any{"slug": d.Slug, "from_queue": true})
	s.apologise(ctx, lead, "We could not start your demo")
}

// expireQueued drops requests that waited longer than QueueWait and says sorry.
func (s *Service) expireQueued(ctx context.Context) {
	stale, err := s.store.StaleQueued(ctx, s.now())
	if err != nil {
		s.log.Error("dispatcher: listing stale requests", "error", err)
		return
	}
	for _, d := range stale {
		if ok, err := s.store.AbandonQueued(ctx, d.ID, "waited too long"); err != nil || !ok {
			continue
		}
		_ = s.store.AddEvent(ctx, d.LeadID, "demo_queue_expired", map[string]any{"slug": d.Slug})
		if l, err := s.store.GetLead(ctx, d.LeadID); err == nil {
			s.apologise(ctx, l, "We could not start your demo")
		}
		s.log.Info("queued demo gave up", "slug", d.Slug)
	}
}

func (s *Service) apologise(ctx context.Context, l store.Lead, subject string) {
	if l.Email == "" || s.mailer == nil || !s.mailer.Ready(ctx) {
		return
	}
	body := fmt.Sprintf("Hi %s,\n\nSorry, we could not fit your demo in this time. Please ask for a new demo on our website any time; it starts straight away when a room is free.\n\nIf you would rather talk to a person, just reply to this email.\n\nThanks for your patience.\n", firstName(l.Name))
	if _, err := s.mailer.Queue(ctx, l.ID, "", l.Email, subject, body, s.now()); err != nil {
		s.log.Warn("could not mail an apology", "error", err)
	}
}

func readyMail(name, url, email, password string, until time.Time, ttl time.Duration) (subject, body string) {
	minutes := int(ttl.Round(time.Minute) / time.Minute)
	subject = "Your demo is ready"
	body = fmt.Sprintf(`Hi %s,

A room is free and your demo is ready. It runs for %d minutes from now, until %s UTC.

Open it:  %s
Email:    %s
Password: %s

Do not enter real customer or employee data: everything is deleted when the time ends.

Enjoy the demo, and reply to this email if you would like to talk to a person.
`, firstName(name), minutes, until.UTC().Format("15:04"), url, email, password)
	return subject, body
}

func firstName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return "there"
}
