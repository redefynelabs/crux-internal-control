package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// GetSetting reads one settings document into v. It returns false when none was saved yet.
func (s *Store) GetSetting(ctx context.Context, key string, v any) (bool, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&raw)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(raw, v)
}

// PutSetting saves one settings document.
func (s *Store) PutSetting(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, b)
	return err
}

// Template is an email people can start from.
type Template struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListTemplates returns every template.
func (s *Store) ListTemplates(ctx context.Context) ([]Template, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, key, name, subject, body, updated_at FROM email_templates ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Template{}
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Key, &t.Name, &t.Subject, &t.Body, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TemplateByKey returns the template with a key.
func (s *Store) TemplateByKey(ctx context.Context, key string) (Template, error) {
	var t Template
	err := s.pool.QueryRow(ctx, `SELECT id, key, name, subject, body, updated_at FROM email_templates WHERE key = $1`, key).Scan(&t.ID, &t.Key, &t.Name, &t.Subject, &t.Body, &t.UpdatedAt)
	return t, notFound(err)
}

// UpdateTemplate changes a template's text.
func (s *Store) UpdateTemplate(ctx context.Context, id, name, subject, body string) (Template, error) {
	var t Template
	err := s.pool.QueryRow(ctx, `UPDATE email_templates SET name = $2, subject = $3, body = $4, updated_at = now() WHERE id = $1 RETURNING id, key, name, subject, body, updated_at`, id, name, subject, body).
		Scan(&t.ID, &t.Key, &t.Name, &t.Subject, &t.Body, &t.UpdatedAt)
	return t, notFound(err)
}
