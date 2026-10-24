package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// Installation is an ERP installation the vendor issues entitlements to.
type Installation struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Kind             string    `json:"kind"`
	DemoID           *string   `json:"demo_id"`
	LeadID           *string   `json:"lead_id"`
	LeadName         string    `json:"lead_name"`
	Notes            string    `json:"notes"`
	SecretGeneration int       `json:"-"`
	CreatedAt        time.Time `json:"created_at"`
	// Version, Template and ExpiresAt describe the newest file issued; Version is 0 when there is none.
	Version  int    `json:"version"`
	Template string `json:"template"`
	// Edition is the newest file's, services or goods; empty for a file issued before editions.
	Edition      string     `json:"edition"`
	ExpiresAt    *time.Time `json:"expires_at"`
	IssuedAt     *time.Time `json:"issued_at"`
	PendingAsks  int        `json:"pending_requests"`
	AppCount     int        `json:"app_count"`
	FeatureCount int        `json:"feature_count"`

	// Licence, from the newest file.
	LicenseType  string     `json:"license_type"`
	UpdatesUntil *time.Time `json:"updates_until"`
	Suspended    bool       `json:"file_suspended"`
	// Lifecycle and what the installation last reported.
	Status          string     `json:"status"`
	StatusNote      string     `json:"status_note"`
	StatusChangedAt *time.Time `json:"status_changed_at"`
	ActivatedAt     *time.Time `json:"activated_at"`
	LastSeenAt      *time.Time `json:"last_seen_at"`
	AppliedVersion  *int       `json:"applied_version"`
	ServerVersion   string     `json:"server_version"`
	ServerBuild     string     `json:"server_build"`
}

const installationSelect = `
	SELECT i.id, i.name, i.kind, i.demo_id, i.lead_id, COALESCE(l.name, ''), i.notes, i.secret_generation, i.created_at,
	       COALESCE(e.version, 0), COALESCE(e.payload->>'template', ''), COALESCE(e.payload->>'edition', ''),
	       (e.payload->>'expires_at')::timestamptz, e.issued_at,
	       (SELECT count(*) FROM access_requests r WHERE r.installation_id = i.id AND r.status = 'pending'),
	       COALESCE(jsonb_array_length(e.payload->'apps'), 0), COALESCE(jsonb_array_length(e.payload->'features'), 0),
	       COALESCE(e.payload->'license'->>'type', CASE WHEN e.payload IS NULL THEN '' ELSE 'subscription' END),
	       (e.payload->>'updates_until')::timestamptz, COALESCE((e.payload->>'suspended')::boolean, false),
	       i.status, i.status_note, i.status_changed_at, i.activated_at, i.last_seen_at, i.applied_version, i.server_version, i.server_build
	FROM installations i
	LEFT JOIN leads l ON l.id = i.lead_id
	LEFT JOIN LATERAL (SELECT * FROM entitlements x WHERE x.installation_id = i.id ORDER BY x.version DESC LIMIT 1) e ON true`

func scanInstallation(r pgx.Row) (Installation, error) {
	var i Installation
	err := r.Scan(&i.ID, &i.Name, &i.Kind, &i.DemoID, &i.LeadID, &i.LeadName, &i.Notes, &i.SecretGeneration, &i.CreatedAt,
		&i.Version, &i.Template, &i.Edition, &i.ExpiresAt, &i.IssuedAt, &i.PendingAsks, &i.AppCount, &i.FeatureCount,
		&i.LicenseType, &i.UpdatesUntil, &i.Suspended,
		&i.Status, &i.StatusNote, &i.StatusChangedAt, &i.ActivatedAt, &i.LastSeenAt, &i.AppliedVersion, &i.ServerVersion, &i.ServerBuild)
	// Dates are the vendor's, not the database session's: an end date is the end of a day in UTC, and shown in another zone it
	// would read as the next day.
	i.ExpiresAt, i.UpdatesUntil, i.IssuedAt = utc(i.ExpiresAt), utc(i.UpdatesUntil), utc(i.IssuedAt)
	i.StatusChangedAt, i.ActivatedAt, i.LastSeenAt = utc(i.StatusChangedAt), utc(i.ActivatedAt), utc(i.LastSeenAt)
	return i, err
}

// CreateInstallation adds an installation. leadID and demoID may be empty.
func (s *Store) CreateInstallation(ctx context.Context, name, kind, leadID, demoID, notes string) (Installation, error) {
	var id string
	if err := s.pool.QueryRow(ctx, `INSERT INTO installations (name, kind, lead_id, demo_id, notes, activated_at)
		VALUES ($1, $2, $3, $4, $5, CASE WHEN $2 = 'demo' THEN now() END) RETURNING id`,
		name, kind, nullable(leadID), nullable(demoID), notes).Scan(&id); err != nil {
		return Installation{}, err
	}
	return s.GetInstallation(ctx, id)
}

// GetInstallation returns one installation.
func (s *Store) GetInstallation(ctx context.Context, id string) (Installation, error) {
	i, err := scanInstallation(s.pool.QueryRow(ctx, installationSelect+` WHERE i.id = $1`, id))
	return i, notFound(err)
}

// ListInstallations returns installations, newest first. kind "" lists all.
func (s *Store) ListInstallations(ctx context.Context, kind string) ([]Installation, error) {
	rows, err := s.pool.Query(ctx, installationSelect+` WHERE ($1 = '' OR i.kind = $1) ORDER BY i.created_at DESC LIMIT 500`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Installation{}
	for rows.Next() {
		i, err := scanInstallation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// UpdateInstallation changes the name and notes.
func (s *Store) UpdateInstallation(ctx context.Context, id, name, notes string) (Installation, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE installations SET name = $2, notes = $3 WHERE id = $1`, id, name, notes)
	if err != nil {
		return Installation{}, err
	}
	if tag.RowsAffected() == 0 {
		return Installation{}, ErrNotFound
	}
	return s.GetInstallation(ctx, id)
}

// RotateSecret replaces the installation's credential and returns the new generation.
func (s *Store) RotateSecret(ctx context.Context, id string) (int, error) {
	var g int
	err := s.pool.QueryRow(ctx, `UPDATE installations SET secret_generation = secret_generation + 1 WHERE id = $1 RETURNING secret_generation`, id).Scan(&g)
	return g, notFound(err)
}

// Issued is one file issued to an installation.
type Issued struct {
	Version  int             `json:"version"`
	Payload  json.RawMessage `json:"payload"`
	File     string          `json:"-"`
	IssuedBy *string         `json:"issued_by"`
	IssuedAt time.Time       `json:"issued_at"`
}

// NextVersion is the version the next file for an installation takes.
func (s *Store) NextVersion(ctx context.Context, installationID string) (int, error) {
	var v int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(max(version), 0) + 1 FROM entitlements WHERE installation_id = $1`, installationID).Scan(&v)
	return v, err
}

// SaveIssued stores a signed file and, in the same transaction, settles the requests it answers: approved ones become
// approved (only those still pending). A concurrent issue that took the same version fails on the unique key.
func (s *Store) SaveIssued(ctx context.Context, installationID string, version int, payload []byte, file, issuedBy string, approved []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO entitlements (installation_id, version, payload, file, issued_by) VALUES ($1, $2, $3, $4, $5)`,
		installationID, version, payload, file, nullable(issuedBy)); err != nil {
		return err
	}
	if len(approved) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE access_requests SET status = 'approved', decided_at = now(), decided_by = $3
			WHERE installation_id = $1 AND id = ANY($2::uuid[]) AND status = 'pending'`, installationID, approved, nullable(issuedBy)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// LatestIssued returns the newest file for an installation.
func (s *Store) LatestIssued(ctx context.Context, installationID string) (Issued, error) {
	return s.issued(ctx, installationID, 0)
}

// IssuedVersion returns one file; version 0 is the newest.
func (s *Store) IssuedVersion(ctx context.Context, installationID string, version int) (Issued, error) {
	return s.issued(ctx, installationID, version)
}

func (s *Store) issued(ctx context.Context, id string, version int) (Issued, error) {
	var i Issued
	err := s.pool.QueryRow(ctx, `SELECT version, payload, file, issued_by, issued_at FROM entitlements
		WHERE installation_id = $1 AND ($2 = 0 OR version = $2) ORDER BY version DESC LIMIT 1`, id, version).
		Scan(&i.Version, &i.Payload, &i.File, &i.IssuedBy, &i.IssuedAt)
	return i, notFound(err)
}

// ListIssued returns the history of an installation, newest first, without the files.
func (s *Store) ListIssued(ctx context.Context, installationID string) ([]Issued, error) {
	rows, err := s.pool.Query(ctx, `SELECT version, payload, issued_by, issued_at FROM entitlements WHERE installation_id = $1 ORDER BY version DESC`, installationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Issued{}
	for rows.Next() {
		var i Issued
		if err := rows.Scan(&i.Version, &i.Payload, &i.IssuedBy, &i.IssuedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// AccessRequest is what an installation asked for.
type AccessRequest struct {
	ID               string     `json:"id"`
	InstallationID   string     `json:"installation_id"`
	InstallationName string     `json:"installation_name"`
	Kind             string     `json:"kind"`
	Name             string     `json:"name"`
	Note             string     `json:"note"`
	RequestedAt      time.Time  `json:"requested_at"`
	Status           string     `json:"status"`
	DecidedNote      string     `json:"decided_note"`
	DecidedAt        *time.Time `json:"decided_at"`
	CreatedAt        time.Time  `json:"created_at"`
}

const requestSelect = `SELECT r.id, r.installation_id, i.name, r.kind, r.name, r.note, r.requested_at, r.status, r.decided_note, r.decided_at, r.created_at
	FROM access_requests r JOIN installations i ON i.id = r.installation_id`

func scanRequest(r pgx.Row) (AccessRequest, error) {
	var a AccessRequest
	err := r.Scan(&a.ID, &a.InstallationID, &a.InstallationName, &a.Kind, &a.Name, &a.Note, &a.RequestedAt, &a.Status, &a.DecidedNote, &a.DecidedAt, &a.CreatedAt)
	return a, err
}

// AddAccessRequest records a request from an installation. It reports false when that id was already recorded.
func (s *Store) AddAccessRequest(ctx context.Context, installationID, id, kind, name, note string, requestedAt time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `INSERT INTO access_requests (id, installation_id, kind, name, note, requested_at) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (installation_id, id) DO NOTHING`, id, installationID, kind, name, note, requestedAt)
	return tag.RowsAffected() == 1, err
}

// ListAccessRequests returns requests, newest first. status "" lists all; installationID "" every installation.
func (s *Store) ListAccessRequests(ctx context.Context, status, installationID string) ([]AccessRequest, error) {
	rows, err := s.pool.Query(ctx, requestSelect+` WHERE ($1 = '' OR r.status = $1) AND ($2 = '' OR r.installation_id::text = $2)
		ORDER BY (r.status = 'pending') DESC, r.created_at DESC LIMIT 500`, status, installationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AccessRequest{}
	for rows.Next() {
		a, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAccessRequest returns one request of an installation.
func (s *Store) GetAccessRequest(ctx context.Context, installationID, id string) (AccessRequest, error) {
	a, err := scanRequest(s.pool.QueryRow(ctx, requestSelect+` WHERE r.installation_id = $1 AND r.id = $2`, installationID, id))
	return a, notFound(err)
}

// DeclineAccessRequest settles a pending request as declined with a note the customer will see.
func (s *Store) DeclineAccessRequest(ctx context.Context, installationID, id, note, by string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE access_requests SET status = 'declined', decided_note = $3, decided_at = now(), decided_by = $4
		WHERE installation_id = $1 AND id = $2 AND status = 'pending'`, installationID, id, note, nullable(by))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeclinedFor returns the declined requests of an installation, as the notes the next file carries.
func (s *Store) DeclinedFor(ctx context.Context, installationID string) ([]AccessRequest, error) {
	all, err := s.ListAccessRequests(ctx, "declined", installationID)
	return all, err
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
