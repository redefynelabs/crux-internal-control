package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Demo statuses.
const (
	DemoQueued       = "queued"
	DemoProvisioning = "provisioning"
	DemoActive       = "active"
	DemoFailed       = "failed"
	DemoExpired      = "expired"
	// DemoAbandoned is a queued request that was cancelled or gave up waiting; it never had an instance.
	DemoAbandoned = "abandoned"
)

// Demo is one short-lived demo instance.
type Demo struct {
	ID        string     `json:"id"`
	LeadID    string     `json:"lead_id"`
	Slug      string     `json:"slug"`
	Status    string     `json:"status"`
	URL       string     `json:"url"`
	IP        string     `json:"ip,omitempty"`
	Error     string     `json:"error,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	// QueuedAt is when the visitor joined the line (nil for a demo that started at once). For a queued demo ExpiresAt is
	// when the request gives up; Request holds the choices to build it with.
	QueuedAt *time.Time `json:"queued_at,omitempty"`
	Request  []byte     `json:"-"`
}

const demoCols = `id, lead_id, slug, status, url, ip, error, created_at, expires_at, ended_at, queued_at, request`

func scanDemo(row pgx.Row) (Demo, error) {
	var d Demo
	err := row.Scan(&d.ID, &d.LeadID, &d.Slug, &d.Status, &d.URL, &d.IP, &d.Error, &d.CreatedAt, &d.ExpiresAt, &d.EndedAt, &d.QueuedAt, &d.Request)
	return d, err
}

// CreateDemo records a demo that is about to be provisioned.
func (s *Store) CreateDemo(ctx context.Context, leadID, slug, ip string, expires time.Time) (Demo, error) {
	return scanDemo(s.pool.QueryRow(ctx, `INSERT INTO demos (lead_id, slug, ip, expires_at) VALUES ($1, $2, $3, $4) RETURNING `+demoCols, leadID, slug, ip, expires))
}

// CreateQueuedDemo records a request that waits for a free slot until giveUp. request is the JSON of the visitor's choices.
func (s *Store) CreateQueuedDemo(ctx context.Context, leadID, slug, ip string, giveUp time.Time, request []byte) (Demo, error) {
	return scanDemo(s.pool.QueryRow(ctx, `INSERT INTO demos (lead_id, slug, ip, status, expires_at, queued_at, request) VALUES ($1, $2, $3, 'queued', $4, now(), $5) RETURNING `+demoCols, leadID, slug, ip, giveUp, request))
}

// CountQueued is how many requests wait for a slot.
func (s *Store) CountQueued(ctx context.Context) (n int, err error) {
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM demos WHERE status = 'queued'`).Scan(&n)
	return
}

// QueuedDemoForLead returns the lead's waiting request, if it has one.
func (s *Store) QueuedDemoForLead(ctx context.Context, leadID string) (Demo, bool, error) {
	d, err := scanDemo(s.pool.QueryRow(ctx, `SELECT `+demoCols+` FROM demos WHERE lead_id = $1 AND status = 'queued' ORDER BY queued_at LIMIT 1`, leadID))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, false, nil
	}
	return d, err == nil, err
}

// QueuePosition is the place in line of a queued demo, 1 being next.
func (s *Store) QueuePosition(ctx context.Context, d Demo) (n int, err error) {
	if d.QueuedAt == nil {
		return 0, nil
	}
	err = s.pool.QueryRow(ctx, `SELECT 1 + count(*) FROM demos WHERE status = 'queued' AND (queued_at, id) < ($1, $2::uuid)`, *d.QueuedAt, d.ID).Scan(&n)
	return
}

// ClaimNextQueued takes the longest waiting request and turns it into a demo that is being provisioned, ending at
// expires. Its start time is now: the wait does not count against the demo's hour. ok is false when nobody waits.
func (s *Store) ClaimNextQueued(ctx context.Context, expires time.Time) (d Demo, ok bool, err error) {
	d, err = scanDemo(s.pool.QueryRow(ctx, `
		UPDATE demos SET status = 'provisioning', expires_at = $1, created_at = now()
		WHERE id = (SELECT id FROM demos WHERE status = 'queued' ORDER BY queued_at, id LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING `+demoCols, expires))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, false, nil
	}
	return d, err == nil, err
}

// RequeueDemo puts a claimed demo whose start failed back in line, keeping its place, with request updated (the retry
// count) and giving up at giveUp.
func (s *Store) RequeueDemo(ctx context.Context, id string, giveUp time.Time, request []byte) error {
	_, err := s.pool.Exec(ctx, `UPDATE demos SET status = 'queued', expires_at = $2, request = $3, error = '' WHERE id = $1 AND status = 'provisioning'`, id, giveUp, request)
	return err
}

// AbandonQueued marks a waiting (or just claimed) request as given up: cancelled, waited too long or could not be started.
// It reports whether it was still waiting. Unlike a failed demo the reaper leaves it alone and nobody is sent a follow-up.
func (s *Store) AbandonQueued(ctx context.Context, id, reason string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE demos SET status = 'abandoned', error = $2, ended_at = now() WHERE id = $1 AND status IN ('queued', 'provisioning')`, id, reason)
	return tag.RowsAffected() > 0, err
}

// StaleQueued returns waiting requests whose time to give up has passed.
func (s *Store) StaleQueued(ctx context.Context, now time.Time) ([]Demo, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+demoCols+` FROM demos WHERE status = 'queued' AND expires_at <= $1 ORDER BY queued_at LIMIT 100`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Demo
	for rows.Next() {
		d, err := scanDemo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDemo returns one demo.
func (s *Store) GetDemo(ctx context.Context, id string) (Demo, error) {
	d, err := scanDemo(s.pool.QueryRow(ctx, `SELECT `+demoCols+` FROM demos WHERE id = $1`, id))
	return d, notFound(err)
}

// MarkDemoActive records that the demo is reachable at url.
func (s *Store) MarkDemoActive(ctx context.Context, id, url string) error {
	_, err := s.pool.Exec(ctx, `UPDATE demos SET status = 'active', url = $2, error = '' WHERE id = $1`, id, url)
	return err
}

// MarkDemoFailed records why provisioning failed. The reaper removes whatever was half-built.
func (s *Store) MarkDemoFailed(ctx context.Context, id, msg string) error {
	_, err := s.pool.Exec(ctx, `UPDATE demos SET status = 'failed', error = $2 WHERE id = $1`, id, msg)
	return err
}

// MarkDemoExpired records that the demo was torn down.
func (s *Store) MarkDemoExpired(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE demos SET status = 'expired', ended_at = now() WHERE id = $1`, id)
	return err
}

// DueDemos returns the demos to tear down: expired ones, failed ones, and ones stuck in provisioning for over ten
// minutes.
func (s *Store) DueDemos(ctx context.Context, now time.Time) ([]Demo, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+demoCols+` FROM demos
		WHERE (status = 'active' AND expires_at <= $1)
		   OR status = 'failed'
		   OR (status = 'provisioning' AND created_at <= $1 - interval '10 minutes')
		ORDER BY expires_at LIMIT 100`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Demo
	for rows.Next() {
		d, err := scanDemo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDemos returns demos newest first, optionally of one status.
func (s *Store) ListDemos(ctx context.Context, status string, limit int) ([]Demo, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT `+demoCols+` FROM demos WHERE $1 = '' OR status = $1 ORDER BY created_at DESC LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Demo
	for rows.Next() {
		d, err := scanDemo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// LiveSlugs returns the slugs that should have a running stack: demos still provisioning or active.
func (s *Store) LiveSlugs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT slug FROM demos WHERE status IN ('provisioning', 'active')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out[slug] = true
	}
	return out, rows.Err()
}

// CountLive is how many demos use resources now.
func (s *Store) CountLive(ctx context.Context) (n int, err error) {
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM demos WHERE status IN ('provisioning', 'active')`).Scan(&n)
	return
}

// CountDemosByEmail is how many demos this email asked for since a time.
func (s *Store) CountDemosByEmail(ctx context.Context, email string, since time.Time) (n int, err error) {
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM demos d JOIN leads l ON l.id = d.lead_id WHERE lower(l.email) = $1 AND d.created_at >= $2`, normEmail(email), since).Scan(&n)
	return
}

// CountDemosByIP is how many demos this address asked for since a time.
func (s *Store) CountDemosByIP(ctx context.Context, ip string, since time.Time) (n int, err error) {
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM demos WHERE ip = $1 AND created_at >= $2`, ip, since).Scan(&n)
	return
}

// ExtendDemo moves the end of an active demo later, by at most the given duration from now.
func (s *Store) ExtendDemo(ctx context.Context, id string, until time.Time) (Demo, error) {
	d, err := scanDemo(s.pool.QueryRow(ctx, `UPDATE demos SET expires_at = $2 WHERE id = $1 AND status = 'active' RETURNING `+demoCols, id, until))
	return d, notFound(err)
}

// Stats is what the overview shows.
type Stats struct {
	Leads             map[string]int `json:"leads"`
	Demos             map[string]int `json:"demos"`
	Unassigned        int            `json:"unassigned"`
	OverdueTasks      int            `json:"overdue_tasks"`
	MyOpenTasks       int            `json:"my_open_tasks"`
	MyOverdueTasks    int            `json:"my_overdue_tasks"`
	EmailsFailed      int            `json:"emails_failed"`
	NewThisWeek       int            `json:"new_this_week"`
	WonThisMonth      int            `json:"won_this_month"`
	WonValueThisMonth int64          `json:"won_value_this_month"`
	PipelineValue     int64          `json:"pipeline_value"`
}

// Stats returns the counts the overview shows. userID is the person looking (for "my tasks"); it may be empty.
func (s *Store) Stats(ctx context.Context, userID string) (Stats, error) {
	st := Stats{Leads: map[string]int{}, Demos: map[string]int{}}
	for table, dst := range map[string]map[string]int{"leads": st.Leads, "demos": st.Demos} {
		rows, err := s.pool.Query(ctx, `SELECT status, count(*) FROM `+table+` GROUP BY status`)
		if err != nil {
			return st, err
		}
		for rows.Next() {
			var k string
			var n int
			if err := rows.Scan(&k, &n); err != nil {
				rows.Close()
				return st, err
			}
			dst[k] = n
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return st, err
		}
	}
	err := s.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM leads WHERE owner_id IS NULL AND status NOT IN ('won', 'lost')),
		  (SELECT count(*) FROM lead_tasks WHERE done_at IS NULL AND due_at < now()),
		  (SELECT count(*) FROM lead_tasks WHERE done_at IS NULL AND assignee_id::text = $1),
		  (SELECT count(*) FROM lead_tasks WHERE done_at IS NULL AND due_at < now() AND assignee_id::text = $1),
		  (SELECT count(*) FROM emails WHERE status = 'failed'),
		  (SELECT count(*) FROM leads WHERE created_at >= now() - interval '7 days'),
		  (SELECT count(*) FROM lead_events WHERE kind = 'status_changed' AND detail->>'status' = 'won' AND at >= date_trunc('month', now())),
		  COALESCE((SELECT sum(expected_value) FROM leads WHERE status = 'won' AND updated_at >= date_trunc('month', now())), 0),
		  COALESCE((SELECT sum(expected_value) FROM leads WHERE status IN ('qualified', 'proposal')), 0)`, userID).
		Scan(&st.Unassigned, &st.OverdueTasks, &st.MyOpenTasks, &st.MyOverdueTasks, &st.EmailsFailed, &st.NewThisWeek, &st.WonThisMonth, &st.WonValueThisMonth, &st.PipelineValue)
	return st, err
}
