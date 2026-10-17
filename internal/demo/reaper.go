package demo

import (
	"context"
	"time"
)

// Reap removes every demo that is due, and any container or database that no demo row accounts for. It is safe to run
// at any time and as often as you like; a failure on one demo does not stop the others and is retried next time.
func (s *Service) Reap(ctx context.Context) {
	due, err := s.store.DueDemos(ctx, s.now())
	if err != nil {
		s.log.Error("reaper: listing due demos", "error", err)
		return
	}
	for _, d := range due {
		if err := s.finish(ctx, d); err != nil {
			s.log.Error("reaper: removing demo", "slug", d.Slug, "error", err)
		}
	}
	s.removeOrphans(ctx)
}

// removeOrphans stops stacks whose demo is not live, for example after a crash between starting containers and saving
// the row.
func (s *Service) removeOrphans(ctx context.Context) {
	running, err := s.stacks.List(ctx)
	if err != nil {
		s.log.Error("reaper: listing stacks", "error", err)
		return
	}
	live, err := s.store.LiveSlugs(ctx)
	if err != nil {
		s.log.Error("reaper: listing live demos", "error", err)
		return
	}
	for _, slug := range running {
		if live[slug] {
			continue
		}
		s.log.Warn("reaper: removing a stack no demo accounts for", "slug", slug)
		if err := s.stacks.Stop(ctx, slug); err != nil {
			s.log.Error("reaper: stopping orphan", "slug", slug, "error", err)
		}
	}
}

// RunReaper reaps every interval until ctx ends.
func (s *Service) RunReaper(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	s.Reap(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Reap(ctx)
		}
	}
}
