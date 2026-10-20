package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Lead statuses.
const (
	LeadNew        = "new"
	LeadDemoActive = "demo_active"
	LeadDemoEnded  = "demo_ended"
	LeadContacted  = "contacted"
	LeadQualified  = "qualified"
	LeadProposal   = "proposal"
	LeadWon        = "won"
	LeadLost       = "lost"
)

// LeadStatuses lists the statuses a person may set by hand.
var LeadStatuses = []string{LeadNew, LeadDemoActive, LeadDemoEnded, LeadContacted, LeadQualified, LeadProposal, LeadWon, LeadLost}

// Interests and Plans a lead can show interest in.
var (
	Interests = []string{"", "cloud", "on_prem", "other"}
	Plans     = []string{"", "start", "grow", "complete", "small", "business", "larger"}
)

// Lead is a person who asked for a demo or got in touch.
type Lead struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Email          string     `json:"email"`
	Phone          string     `json:"phone"`
	Company        string     `json:"company"`
	Source         string     `json:"source"`
	Status         string     `json:"status"`
	Notes          string     `json:"notes"`
	Interest       string     `json:"interest"`
	PlanInterest   string     `json:"plan_interest"`
	TeamSize       string     `json:"team_size"`
	ExpectedValue  int64      `json:"expected_value"`
	LostReason     string     `json:"lost_reason"`
	OwnerID        *string    `json:"owner_id"`
	OwnerName      string     `json:"owner_name"`
	LastContactAt  *time.Time `json:"last_contact_at"`
	NextFollowUpAt *time.Time `json:"next_follow_up_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Actor is who did something. The zero value is the system itself.
type Actor struct{ ID, Name string }

// Event is one entry in a lead's history.
type Event struct {
	Kind      string          `json:"kind"`
	Detail    json.RawMessage `json:"detail"`
	At        time.Time       `json:"at"`
	ActorName string          `json:"actor_name"`
}

// LeadInput is what a request carries.
type LeadInput struct {
	Name, Email, Phone, Company, Source string
	Interest, PlanInterest, TeamSize    string
	ExpectedValue                       int64
	OwnerID                             string
}

const leadSelect = `
	SELECT l.id, l.name, l.email, l.phone, l.company, l.source, l.status, l.notes, l.interest, l.plan_interest, l.team_size,
	       l.expected_value, l.lost_reason, l.owner_id, COALESCE(u.name, ''), l.last_contact_at,
	       (SELECT min(t.due_at) FROM lead_tasks t WHERE t.lead_id = l.id AND t.done_at IS NULL), l.created_at, l.updated_at
	FROM leads l LEFT JOIN users u ON u.id = l.owner_id`

func scanLead(row pgx.Row) (Lead, error) {
	var l Lead
	err := row.Scan(&l.ID, &l.Name, &l.Email, &l.Phone, &l.Company, &l.Source, &l.Status, &l.Notes, &l.Interest, &l.PlanInterest, &l.TeamSize,
		&l.ExpectedValue, &l.LostReason, &l.OwnerID, &l.OwnerName, &l.LastContactAt, &l.NextFollowUpAt, &l.CreatedAt, &l.UpdatedAt)
	return l, err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// UpsertLead creates the lead or, for an email already known, refreshes the contact details and fills in what they want
// when it was not known. A returning person keeps their history, status and owner. created says whether the row is new.
func (s *Store) UpsertLead(ctx context.Context, in LeadInput) (l Lead, created bool, err error) {
	var id string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO leads (name, email, phone, company, source, interest, plan_interest, team_size, expected_value, owner_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (lower(email)) DO UPDATE SET
			name = EXCLUDED.name,
			phone = CASE WHEN EXCLUDED.phone <> '' THEN EXCLUDED.phone ELSE leads.phone END,
			company = CASE WHEN EXCLUDED.company <> '' THEN EXCLUDED.company ELSE leads.company END,
			interest = CASE WHEN EXCLUDED.interest <> '' THEN EXCLUDED.interest ELSE leads.interest END,
			plan_interest = CASE WHEN EXCLUDED.plan_interest <> '' THEN EXCLUDED.plan_interest ELSE leads.plan_interest END,
			team_size = CASE WHEN EXCLUDED.team_size <> '' THEN EXCLUDED.team_size ELSE leads.team_size END,
			updated_at = now()
		RETURNING id, (xmax = 0)`,
		in.Name, normEmail(in.Email), in.Phone, in.Company, in.Source, in.Interest, in.PlanInterest, in.TeamSize, in.ExpectedValue, nullable(in.OwnerID)).Scan(&id, &created)
	if err != nil {
		return l, false, err
	}
	l, err = s.GetLead(ctx, id)
	return l, created, err
}

// GetLead returns one lead.
func (s *Store) GetLead(ctx context.Context, id string) (Lead, error) {
	l, err := scanLead(s.pool.QueryRow(ctx, leadSelect+` WHERE l.id = $1`, id))
	return l, notFound(err)
}

// LeadByEmail returns the lead with this email address.
func (s *Store) LeadByEmail(ctx context.Context, email string) (Lead, error) {
	l, err := scanLead(s.pool.QueryRow(ctx, leadSelect+` WHERE lower(l.email) = $1`, normEmail(email)))
	return l, notFound(err)
}

// LeadDeleteBlockers counts what makes deleting a lead unsafe: sales (the revenue record would lose its customer) and
// installations that are not demos (a customer's licence would lose its owner).
func (s *Store) LeadDeleteBlockers(ctx context.Context, id string) (sales, installs int, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM sales WHERE lead_id = $1),
		       (SELECT count(*) FROM installations WHERE lead_id = $1 AND kind <> 'demo')`, id).Scan(&sales, &installs)
	return
}

// LiveDemoIDs returns the lead's demos that still hold a room or a place in line.
func (s *Store) LiveDemoIDs(ctx context.Context, leadID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM demos WHERE lead_id = $1 AND status IN ('queued', 'provisioning', 'active')`, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DeleteLead removes the lead with its history, tasks, emails and demos, and the installation records the demos made.
// The caller has ended any live demo and checked LeadDeleteBlockers first.
func (s *Store) DeleteLead(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a no-op after commit
	if _, err := tx.Exec(ctx, `DELETE FROM installations WHERE lead_id = $1 AND kind = 'demo'`, id); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM leads WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// LeadFilter narrows the lead list. Empty fields do not filter.
type LeadFilter struct {
	Status   string
	Owner    string // a user id, or "none"
	Interest string
	Due      string // "overdue" or "today": has an open follow-up due by then
	Query    string
	Limit    int
}

// ListLeads returns leads, newest first.
func (s *Store) ListLeads(ctx context.Context, f LeadFilter) ([]Lead, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	if f.Status != "" {
		add("l.status = ?", f.Status)
	}
	switch f.Owner {
	case "":
	case "none":
		where = append(where, "l.owner_id IS NULL")
	default:
		add("l.owner_id = ?", f.Owner)
	}
	if f.Interest != "" {
		add("l.interest = ?", f.Interest)
	}
	switch f.Due {
	case "overdue":
		where = append(where, "EXISTS (SELECT 1 FROM lead_tasks t WHERE t.lead_id = l.id AND t.done_at IS NULL AND t.due_at < now())")
	case "today":
		where = append(where, "EXISTS (SELECT 1 FROM lead_tasks t WHERE t.lead_id = l.id AND t.done_at IS NULL AND t.due_at < date_trunc('day', now()) + interval '1 day')")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		add("(l.name ILIKE ? OR l.email ILIKE ? OR l.company ILIKE ? OR l.phone ILIKE ?)", "%"+escapeLike(q)+"%")
	}
	sql := leadSelect
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, f.Limit)
	sql += fmt.Sprintf(" ORDER BY l.created_at DESC LIMIT $%d", len(args))
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lead{}
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// LeadPatch changes the fields that are set. OwnerID "" removes the owner.
type LeadPatch struct {
	Status        *string
	Notes         *string
	OwnerID       *string
	Interest      *string
	PlanInterest  *string
	TeamSize      *string
	ExpectedValue *int64
	LostReason    *string
	Name          *string
	Phone         *string
	Company       *string
}

// UpdateLead applies a patch and records what changed in the lead's history.
func (s *Store) UpdateLead(ctx context.Context, id string, p LeadPatch, by Actor) (Lead, error) {
	old, err := s.GetLead(ctx, id)
	if err != nil {
		return old, err
	}
	var owner any
	setOwner := p.OwnerID != nil
	if setOwner {
		owner = nullable(*p.OwnerID)
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE leads SET
			status = COALESCE($2, status), notes = COALESCE($3, notes),
			owner_id = CASE WHEN $4 THEN $5::uuid ELSE owner_id END,
			interest = COALESCE($6, interest), plan_interest = COALESCE($7, plan_interest), team_size = COALESCE($8, team_size),
			expected_value = COALESCE($9, expected_value), lost_reason = COALESCE($10, lost_reason),
			name = COALESCE($11, name), phone = COALESCE($12, phone), company = COALESCE($13, company),
			updated_at = now()
		WHERE id = $1`,
		id, p.Status, p.Notes, setOwner, owner, p.Interest, p.PlanInterest, p.TeamSize, p.ExpectedValue, p.LostReason, p.Name, p.Phone, p.Company)
	if err != nil {
		return old, err
	}
	l, err := s.GetLead(ctx, id)
	if err != nil {
		return l, err
	}
	if p.Status != nil && *p.Status != old.Status {
		_ = s.AddEventBy(ctx, id, by, "status_changed", map[string]any{"status": *p.Status, "from": old.Status})
	}
	if setOwner && deref(l.OwnerID) != deref(old.OwnerID) {
		_ = s.AddEventBy(ctx, id, by, "owner_changed", map[string]any{"owner_name": l.OwnerName})
	}
	return l, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// SetLeadStatus sets a status from the system itself (a demo started or ended), only when the lead has not moved
// further along by hand: a won lead is not pulled back to "demo ended".
func (s *Store) SetLeadStatus(ctx context.Context, id, status string, onlyFrom ...string) error {
	_, err := s.pool.Exec(ctx, `UPDATE leads SET status = $2, updated_at = now() WHERE id = $1 AND status = ANY($3)`, id, status, onlyFrom)
	return err
}

// TouchContact records that someone reached out to the lead now.
func (s *Store) TouchContact(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE leads SET last_contact_at = now(), updated_at = now() WHERE id = $1`, id)
	return err
}

// AddEvent appends to a lead's history as the system.
func (s *Store) AddEvent(ctx context.Context, leadID, kind string, detail any) error {
	return s.AddEventBy(ctx, leadID, Actor{}, kind, detail)
}

// AddEventBy appends to a lead's history on behalf of a person.
func (s *Store) AddEventBy(ctx context.Context, leadID string, by Actor, kind string, detail any) error {
	b, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO lead_events (lead_id, kind, detail, actor_id, actor_name) VALUES ($1, $2, $3, $4, $5)`, leadID, kind, b, nullable(by.ID), by.Name)
	return err
}

// Events returns a lead's history, oldest first.
func (s *Store) Events(ctx context.Context, leadID string) ([]Event, error) {
	rows, err := s.pool.Query(ctx, `SELECT kind, detail, at, actor_name FROM lead_events WHERE lead_id = $1 ORDER BY at, id`, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Kind, &e.Detail, &e.At, &e.ActorName); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountContactsByIP is how many contact messages came from an address since a time.
func (s *Store) CountContactsByIP(ctx context.Context, ip string, since time.Time) (n int, err error) {
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM lead_events WHERE kind = 'contact_message' AND detail->>'ip' = $1 AND at >= $2`, ip, since).Scan(&n)
	return
}

// NextRoundRobinOwner picks the active salesperson who was given a lead longest ago (or never), so new leads rotate.
// Admins only take leads when no salesperson is active. It returns "" when nobody can take leads.
func (s *Store) NextRoundRobinOwner(ctx context.Context) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT u.id FROM users u
		WHERE u.active AND u.password_hash <> ''
		ORDER BY (u.role = 'sales') DESC, (SELECT max(l.created_at) FROM leads l WHERE l.owner_id = u.id) NULLS FIRST, u.created_at
		LIMIT 1`).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}
