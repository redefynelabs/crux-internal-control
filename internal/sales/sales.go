// Package sales is what happens to a lead when it arrives and when its demo ends: who owns it, the first follow-up, and
// the automatic emails. Each piece can be switched off in Settings → General.
package sales

import (
	"context"
	"log/slog"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/mail"
	"github.com/redefynelabs/erp-control-plane/internal/store"
)

// SettingsKey is where General settings are stored.
const SettingsKey = "general"

// General is the sales behaviour an admin can change.
type General struct {
	// Assignment is "unassigned" (leads wait to be claimed) or "round_robin" (leads rotate between active salespeople).
	Assignment string `json:"assignment"`
	// AutoConfirm emails a visitor who wrote to us, to say we got it.
	AutoConfirm bool `json:"auto_confirm"`
	// AutoDemoFollowup emails a visitor after their demo ends.
	AutoDemoFollowup bool `json:"auto_demo_followup"`
	// FollowupHours is how long until the first follow-up on a new lead is due.
	FollowupHours int `json:"followup_hours"`
	// DemoFollowupDelayMinutes is how long after a demo ends the follow-up email goes out.
	DemoFollowupDelayMinutes int `json:"demo_followup_delay_minutes"`
}

// Defaults are used until an admin saves General settings.
func Defaults() General {
	return General{Assignment: "unassigned", AutoConfirm: true, AutoDemoFollowup: false, FollowupHours: 24, DemoFollowupDelayMinutes: 60}
}

// Validate returns what is wrong with the settings, by field.
func (g General) Validate() map[string]string {
	f := map[string]string{}
	if g.Assignment != "unassigned" && g.Assignment != "round_robin" {
		f["assignment"] = "Choose one of the options"
	}
	if g.FollowupHours < 1 || g.FollowupHours > 24*30 {
		f["followup_hours"] = "Between 1 and 720"
	}
	if g.DemoFollowupDelayMinutes < 0 || g.DemoFollowupDelayMinutes > 7*24*60 {
		f["demo_followup_delay_minutes"] = "Between 0 and 10080"
	}
	return f
}

// Sales runs the automation.
type Sales struct {
	store *store.Store
	mail  *mail.Service
	log   *slog.Logger
	now   func() time.Time
}

// New returns the automation.
func New(st *store.Store, m *mail.Service, log *slog.Logger) *Sales {
	return &Sales{store: st, mail: m, log: log, now: time.Now}
}

// General returns the saved settings or the defaults.
func (s *Sales) General(ctx context.Context) General {
	g := Defaults()
	_, _ = s.store.GetSetting(ctx, SettingsKey, &g)
	return g
}

// NewLead runs when a visitor asks for a demo or writes to us. wrote is true for a message (they expect a reply).
// created says whether this is a first contact; a returning person is not given a second owner or first follow-up.
func (s *Sales) NewLead(ctx context.Context, l store.Lead, created, wrote bool) {
	g := s.General(ctx)
	by := store.Actor{Name: "Automatic"}
	if created {
		_ = s.store.AddEvent(ctx, l.ID, "lead_created", map[string]any{"source": l.Source})
	}
	if l.OwnerID == nil && g.Assignment == "round_robin" {
		if owner, err := s.store.NextRoundRobinOwner(ctx); err != nil {
			s.log.Warn("sales: choosing an owner", "error", err)
		} else if owner != "" {
			if nl, err := s.store.UpdateLead(ctx, l.ID, store.LeadPatch{OwnerID: &owner}, by); err == nil {
				l = nl
			}
		}
	}
	if created || wrote {
		open, err := s.store.ListTasks(ctx, store.TaskFilter{LeadID: l.ID})
		if err == nil && len(open) == 0 {
			title := "Follow up with " + l.Name
			if l.Interest == "cloud" {
				title = "Follow up on the cloud plan request from " + l.Name
			} else if wrote {
				title = "Reply to the message from " + l.Name
			}
			owner := ""
			if l.OwnerID != nil {
				owner = *l.OwnerID
			}
			if _, err := s.store.CreateTask(ctx, l.ID, title, "call", s.now().Add(time.Duration(g.FollowupHours)*time.Hour), owner, ""); err != nil {
				s.log.Warn("sales: creating the first follow-up", "error", err)
			}
		}
	}
	if wrote && g.AutoConfirm {
		s.queue(ctx, l, "contact_received", s.now())
	}
}

// DemoEnded runs when a visitor's demo is removed.
func (s *Sales) DemoEnded(ctx context.Context, l store.Lead) {
	g := s.General(ctx)
	if !g.AutoDemoFollowup || l.Status == store.LeadWon || l.Status == store.LeadLost {
		return
	}
	s.queue(ctx, l, "demo_followup", s.now().Add(time.Duration(g.DemoFollowupDelayMinutes)*time.Minute))
}

func (s *Sales) queue(ctx context.Context, l store.Lead, key string, after time.Time) {
	if !s.mail.Ready(ctx) {
		return
	}
	t, err := s.store.TemplateByKey(ctx, key)
	if err != nil {
		return
	}
	ms, _ := s.mail.Settings(ctx)
	vars := s.mail.Enrich(ctx, mail.Vars(l.Name, l.Company, "", l.OwnerName, ms.Signature))
	r := s.mail.Composer().Compose(t.Format, t.Subject, t.Body, vars)
	if _, err := s.mail.QueueRendered(ctx, l.ID, "", l.Email, r, after); err != nil {
		s.log.Warn("sales: queueing an email", "template", key, "error", err)
	}
}
