package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// SetStatus records the vendor's decision about an installation: active, suspended or revoked.
func (s *Store) SetStatus(ctx context.Context, id, status, note string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE installations SET status = $2, status_note = $3, status_changed_at = now() WHERE id = $1`, id, status, note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordSeen stores what an installation reported when it checked in. A call it could sign is proof it is set up, so the
// first one also counts as activation for installations wired by hand before activation codes existed.
func (s *Store) RecordSeen(ctx context.Context, id string, applied *int, serverVersion, build string) error {
	_, err := s.pool.Exec(ctx, `UPDATE installations SET last_seen_at = now(), activated_at = COALESCE(activated_at, now()), applied_version = COALESCE($2, applied_version),
		server_version = $3, server_build = $4 WHERE id = $1`, id, applied, serverVersion, build)
	return err
}

// Activation is a code made for an installation. The code itself is never stored, only its hash and a hint.
type Activation struct {
	ID        string     `json:"id"`
	Hint      string     `json:"hint"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateActivation stores a new code and revokes the installation's earlier codes that were not used.
func (s *Store) CreateActivation(ctx context.Context, installationID, hash, hint string, expires time.Time, by string) (Activation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Activation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE activation_codes SET revoked_at = now() WHERE installation_id = $1 AND used_at IS NULL AND revoked_at IS NULL`, installationID); err != nil {
		return Activation{}, err
	}
	var a Activation
	err = tx.QueryRow(ctx, `INSERT INTO activation_codes (installation_id, code_hash, hint, expires_at, created_by) VALUES ($1, $2, $3, $4, $5)
		RETURNING id, hint, expires_at, used_at, revoked_at, created_at`, installationID, hash, hint, expires, nullable(by)).
		Scan(&a.ID, &a.Hint, &a.ExpiresAt, &a.UsedAt, &a.RevokedAt, &a.CreatedAt)
	if err != nil {
		return Activation{}, notFound(err)
	}
	return a, tx.Commit(ctx)
}

// LatestActivation returns the newest code of an installation, or ErrNotFound.
func (s *Store) LatestActivation(ctx context.Context, installationID string) (Activation, error) {
	var a Activation
	err := s.pool.QueryRow(ctx, `SELECT id, hint, expires_at, used_at, revoked_at, created_at FROM activation_codes
		WHERE installation_id = $1 ORDER BY created_at DESC LIMIT 1`, installationID).Scan(&a.ID, &a.Hint, &a.ExpiresAt, &a.UsedAt, &a.RevokedAt, &a.CreatedAt)
	return a, notFound(err)
}

// ConsumeActivation burns a valid code and returns the installation it was made for. A code that is unknown, used,
// revoked or expired is ErrNotFound; the caller cannot tell which, and neither can a guesser.
func (s *Store) ConsumeActivation(ctx context.Context, hash string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `UPDATE activation_codes SET used_at = now()
		WHERE code_hash = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now() RETURNING installation_id`, hash).Scan(&id)
	if err != nil {
		return "", notFound(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE installations SET activated_at = COALESCE(activated_at, now()) WHERE id = $1`, id); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

// PeekActivation returns the installation a code is valid for without using it up: the installer asks before the customer's
// setup does. A code that is unknown, used, revoked or expired is ErrNotFound.
func (s *Store) PeekActivation(ctx context.Context, hash string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT installation_id FROM activation_codes
		WHERE code_hash = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now()`, hash).Scan(&id)
	return id, notFound(err)
}

// Release is one published version of the ERP.
type Release struct {
	ID             string            `json:"id"`
	Version        string            `json:"version"`
	Channel        string            `json:"channel"`
	PublishedAt    time.Time         `json:"published_at"`
	Notes          string            `json:"notes"`
	HasMigrations  bool              `json:"has_migrations"`
	MinFromVersion string            `json:"min_from_version"`
	Images         map[string]string `json:"images"`
	Manifest       string            `json:"-"`
	WithdrawnAt    *time.Time        `json:"withdrawn_at"`
	CreatedAt      time.Time         `json:"created_at"`
	// Running counts the installations whose last report was this version.
	Running int `json:"running"`
}

const releaseCols = `r.id, r.version, r.channel, r.published_at, r.notes, r.has_migrations, r.min_from_version, r.images, r.manifest, r.withdrawn_at, r.created_at,
	(SELECT count(*) FROM installations i WHERE i.server_version = r.version AND i.kind = 'customer')`

func scanRelease(r pgx.Row) (Release, error) {
	var x Release
	var images []byte
	if err := r.Scan(&x.ID, &x.Version, &x.Channel, &x.PublishedAt, &x.Notes, &x.HasMigrations, &x.MinFromVersion, &images, &x.Manifest, &x.WithdrawnAt, &x.CreatedAt, &x.Running); err != nil {
		return x, err
	}
	x.Images = map[string]string{}
	return x, json.Unmarshal(images, &x.Images)
}

// CreateRelease stores a signed release. A version can be published once.
func (s *Store) CreateRelease(ctx context.Context, r Release, by string) (Release, error) {
	images, err := json.Marshal(r.Images)
	if err != nil {
		return Release{}, err
	}
	var id string
	if err := s.pool.QueryRow(ctx, `INSERT INTO releases (version, channel, published_at, notes, has_migrations, min_from_version, images, manifest, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		r.Version, r.Channel, r.PublishedAt, r.Notes, r.HasMigrations, r.MinFromVersion, images, r.Manifest, nullable(by)).Scan(&id); err != nil {
		return Release{}, err
	}
	return s.GetRelease(ctx, id)
}

// GetRelease returns one release.
func (s *Store) GetRelease(ctx context.Context, id string) (Release, error) {
	r, err := scanRelease(s.pool.QueryRow(ctx, `SELECT `+releaseCols+` FROM releases r WHERE r.id = $1`, id))
	return r, notFound(err)
}

// ListReleases returns every release, newest first.
func (s *Store) ListReleases(ctx context.Context) ([]Release, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+releaseCols+` FROM releases r ORDER BY r.published_at DESC, r.created_at DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Release{}
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// WithdrawRelease stops offering a release; installations already running it are unaffected.
func (s *Store) WithdrawRelease(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE releases SET withdrawn_at = COALESCE(withdrawn_at, now()) WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AuditEntry is one line of the audit trail.
type AuditEntry struct {
	ID             int64           `json:"id"`
	At             time.Time       `json:"at"`
	ActorEmail     string          `json:"actor_email"`
	InstallationID *string         `json:"installation_id"`
	Installation   string          `json:"installation_name"`
	Action         string          `json:"action"`
	Detail         json.RawMessage `json:"detail"`
}

// Audit records an action. A failure to record is returned, but callers log it and carry on: the action already happened.
func (s *Store) Audit(ctx context.Context, actorID, actorEmail, installationID, action string, detail any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO audit_log (actor_id, actor_email, installation_id, action, detail) VALUES ($1, $2, $3, $4, $5)`,
		nullable(actorID), actorEmail, nullable(installationID), action, raw)
	return err
}

// ListAudit returns the newest entries, for one installation or (with "") all.
func (s *Store) ListAudit(ctx context.Context, installationID string, limit int) ([]AuditEntry, error) {
	rows, err := s.pool.Query(ctx, `SELECT a.id, a.at, a.actor_email, a.installation_id, COALESCE(i.name, ''), a.action, a.detail
		FROM audit_log a LEFT JOIN installations i ON i.id = a.installation_id
		WHERE ($1 = '' OR a.installation_id::text = $1) ORDER BY a.at DESC, a.id DESC LIMIT $2`, installationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.At, &e.ActorEmail, &e.InstallationID, &e.Installation, &e.Action, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
