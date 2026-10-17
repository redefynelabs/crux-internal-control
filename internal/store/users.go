package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Roles.
const (
	RoleAdmin = "admin"
	RoleSales = "sales"
)

// Roles lists the roles a person can hold.
var Roles = []string{RoleAdmin, RoleSales}

// ErrEmailTaken is returned when a person with that email already exists.
var ErrEmailTaken = errors.New("email already in use")

// User is a member of the team.
type User struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Email         string     `json:"email"`
	Role          string     `json:"role"`
	Active        bool       `json:"active"`
	PendingInvite bool       `json:"pending_invite"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

const userCols = `id, name, email, role, active, (password_hash = ''), last_login_at, created_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Active, &u.PendingInvite, &u.LastLoginAt, &u.CreatedAt)
	return u, err
}

// HashToken is how session and invite tokens are stored: only the hash is kept, so a database leak holds no usable token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewToken returns a random URL-safe token.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func uniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// CountUsers is how many people exist; zero means the first admin still has to be created.
func (s *Store) CountUsers(ctx context.Context) (n int, err error) {
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return
}

// CreateUser adds a person. A password hash of "" makes them an invitee who sets a password from the invite.
func (s *Store) CreateUser(ctx context.Context, name, email, role, passwordHash string) (User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `INSERT INTO users (name, email, role, password_hash) VALUES ($1, $2, $3, $4) RETURNING `+userCols, name, normEmail(email), role, passwordHash))
	if uniqueViolation(err) {
		return u, ErrEmailTaken
	}
	return u, err
}

// GetUser returns one person.
func (s *Store) GetUser(ctx context.Context, id string) (User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
	return u, notFound(err)
}

// ListUsers returns everyone, active first.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY active DESC, lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UserPatch changes the fields that are set.
type UserPatch struct {
	Name   *string
	Role   *string
	Active *bool
}

// UpdateUser applies a patch. Deactivating a person also ends their sessions.
func (s *Store) UpdateUser(ctx context.Context, id string, p UserPatch) (User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `UPDATE users SET name = COALESCE($2, name), role = COALESCE($3, role), active = COALESCE($4, active) WHERE id = $1 RETURNING `+userCols, id, p.Name, p.Role, p.Active))
	if err != nil {
		return u, notFound(err)
	}
	if p.Active != nil && !*p.Active {
		_, _ = s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id)
	}
	return u, nil
}

// CountActiveAdmins is how many active admins exist; the last one may not be removed.
func (s *Store) CountActiveAdmins(ctx context.Context) (n int, err error) {
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'admin' AND active`).Scan(&n)
	return
}

// Credentials returns a person and their password hash for sign-in.
func (s *Store) Credentials(ctx context.Context, email string) (User, string, error) {
	var hash string
	var u User
	err := s.pool.QueryRow(ctx, `SELECT `+userCols+`, password_hash FROM users WHERE lower(email) = $1`, normEmail(email)).
		Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Active, &u.PendingInvite, &u.LastLoginAt, &u.CreatedAt, &hash)
	return u, hash, notFound(err)
}

// PasswordHash returns a person's current hash.
func (s *Store) PasswordHash(ctx context.Context, id string) (hash string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, id).Scan(&hash)
	return hash, notFound(err)
}

// SetPassword stores a new hash, clears any invite and ends the person's other sessions.
func (s *Store) SetPassword(ctx context.Context, id, hash string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $2, invite_hash = '', invite_expires_at = NULL WHERE id = $1`, id, hash)
	return err
}

// StartInvite makes a new invite token for a person and returns it. They set their password with it.
func (s *Store) StartInvite(ctx context.Context, id string, ttl time.Duration) (string, error) {
	tok, err := NewToken()
	if err != nil {
		return "", err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE users SET invite_hash = $2, invite_expires_at = $3 WHERE id = $1`, id, HashToken(tok), time.Now().Add(ttl))
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "", ErrNotFound
	}
	_, _ = s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id)
	return tok, nil
}

// UserByInvite returns the person an unexpired invite token belongs to.
func (s *Store) UserByInvite(ctx context.Context, token string) (User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE invite_hash = $1 AND invite_hash <> '' AND invite_expires_at > now() AND active`, HashToken(token)))
	return u, notFound(err)
}

// CreateSession stores a session for a person and returns its token.
func (s *Store) CreateSession(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	tok, err := NewToken()
	if err != nil {
		return "", err
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, HashToken(tok), userID, time.Now().Add(ttl)); err != nil {
		return "", err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, userID)
	_, _ = s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	return tok, nil
}

// UserBySession returns the active person a session token belongs to.
func (s *Store) UserBySession(ctx context.Context, token string) (User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `
		SELECT u.id, u.name, u.email, u.role, u.active, (u.password_hash = ''), u.last_login_at, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now() AND u.active`, HashToken(token)))
	return u, notFound(err)
}

// DeleteSession ends one session.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, HashToken(token))
	return err
}

// DeleteOtherSessions ends every session of a person except the one given.
func (s *Store) DeleteOtherSessions(ctx context.Context, userID, keepToken string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, userID, HashToken(keepToken))
	return err
}

// EndSessions signs a person out everywhere.
func (s *Store) EndSessions(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}
