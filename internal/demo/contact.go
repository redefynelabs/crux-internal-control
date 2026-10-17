package demo

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/store"
)

// Interests a visitor can name when they write to sales.
var Interests = []string{"cloud", "on_prem", "other"}

// Contact is a message to sales from the contact form.
type Contact struct {
	Name, Email, Phone, Company string
	Interest, Plan              string
	// TeamSize and Timeline are optional (see TeamSizes and Timelines).
	TeamSize, Timeline string
	Message            string
	IP                 string
	Attribution        map[string]string
}

// Validate checks the message and returns what is wrong with it, by field.
func (c Contact) Validate() error {
	f := map[string]string{}
	if err := (Request{Name: c.Name, Email: c.Email, Phone: c.Phone, Company: c.Company}).Validate(); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			for k, v := range ve.Fields {
				f[k] = v
			}
		}
	}
	if m := strings.TrimSpace(c.Message); m == "" || len(m) > 2000 {
		f["message"] = "Write a short message (up to 2000 characters)"
	}
	if c.Interest != "" {
		ok := false
		for _, i := range Interests {
			ok = ok || i == c.Interest
		}
		if !ok {
			f["interest"] = "Choose one of the options"
		}
	}
	if c.TeamSize != "" && !slices.Contains(TeamSizes, c.TeamSize) {
		f["team_size"] = "Choose one of the options"
	}
	if c.Plan != "" && !slices.Contains(store.Plans, c.Plan) {
		f["plan"] = "Choose one of the options"
	}
	if c.Timeline != "" && !slices.Contains(Timelines, c.Timeline) {
		f["timeline"] = "Choose one of the options"
	}
	if len(f) > 0 {
		return &ValidationError{Fields: f}
	}
	return nil
}

// Contact records a message from the contact form on the lead, creating the lead if needed. It starts no demo.
func (s *Service) Contact(ctx context.Context, c Contact) error {
	if err := c.Validate(); err != nil {
		return err
	}
	since := s.now().Add(-time.Hour)
	if c.IP != "" {
		if n, err := s.store.CountContactsByIP(ctx, c.IP, since); err != nil {
			return err
		} else if n >= 5 {
			return ErrRateLimited
		}
	}
	lead, created, err := s.store.UpsertLead(ctx, store.LeadInput{Name: strings.TrimSpace(c.Name), Email: c.Email, Phone: strings.TrimSpace(c.Phone), Company: strings.TrimSpace(c.Company), Source: "website-contact", Interest: c.Interest, PlanInterest: c.Plan, TeamSize: c.TeamSize})
	if err != nil {
		return err
	}
	if s.intake != nil {
		s.intake.NewLead(ctx, lead, created, true)
	}
	if err := s.store.AddEvent(ctx, lead.ID, "contact_message", map[string]any{"message": strings.TrimSpace(c.Message), "interest": c.Interest, "plan": c.Plan, "team_size": c.TeamSize, "timeline": c.Timeline, "ip": c.IP, "attribution": CleanAttribution(c.Attribution)}); err != nil {
		return err
	}
	s.notify.Send(ctx, fmt.Sprintf("Message from %s <%s>%s: %s", lead.Name, lead.Email, companySuffix(lead.Company), preview(c.Message)))
	return nil
}

func preview(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
