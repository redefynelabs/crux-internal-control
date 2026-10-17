package store

import (
	"context"
	"time"
)

// Email statuses.
const (
	EmailQueued = "queued"
	EmailSent   = "sent"
	EmailFailed = "failed"
)

// Email is a message sent, or waiting to be sent, to a lead.
type Email struct {
	ID         string     `json:"id"`
	LeadID     *string    `json:"lead_id"`
	Subject    string     `json:"subject"`
	Body       string     `json:"body"`
	ToEmail    string     `json:"to_email"`
	Status     string     `json:"status"`
	Error      string     `json:"error"`
	SentByName string     `json:"sent_by_name"`
	Attempts   int        `json:"-"`
	CreatedAt  time.Time  `json:"created_at"`
	SentAt     *time.Time `json:"sent_at"`
}

const emailSelect = `
	SELECT e.id, e.lead_id, e.subject, e.body, e.to_email, e.status, e.error, COALESCE(u.name, ''), e.attempts, e.created_at, e.sent_at
	FROM emails e LEFT JOIN users u ON u.id = e.sent_by`

func scanEmail(r rowScanner) (Email, error) {
	var e Email
	err := r.Scan(&e.ID, &e.LeadID, &e.Subject, &e.Body, &e.ToEmail, &e.Status, &e.Error, &e.SentByName, &e.Attempts, &e.CreatedAt, &e.SentAt)
	return e, err
}

// QueueEmail adds a message to the outbox, to go out at or after sendAfter. leadID and sentBy may be empty.
func (s *Store) QueueEmail(ctx context.Context, leadID, sentBy, to, subject, body string, sendAfter time.Time) (Email, error) {
	var id string
	if err := s.pool.QueryRow(ctx, `INSERT INTO emails (lead_id, sent_by, to_email, subject, body, send_after) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		nullable(leadID), nullable(sentBy), to, subject, body, sendAfter).Scan(&id); err != nil {
		return Email{}, err
	}
	return scanEmail(s.pool.QueryRow(ctx, emailSelect+` WHERE e.id = $1`, id))
}

// LeadEmails returns the messages for a lead, newest first.
func (s *Store) LeadEmails(ctx context.Context, leadID string) ([]Email, error) {
	rows, err := s.pool.Query(ctx, emailSelect+` WHERE e.lead_id = $1 ORDER BY e.created_at DESC`, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Email{}
	for rows.Next() {
		e, err := scanEmail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ClaimDueEmails returns queued messages that are due and marks them as being tried, so two workers never send the same.
func (s *Store) ClaimDueEmails(ctx context.Context, limit int) ([]Email, error) {
	rows, err := s.pool.Query(ctx, `
		WITH due AS (
			SELECT id FROM emails WHERE status = 'queued' AND send_after <= now() ORDER BY send_after LIMIT $1 FOR UPDATE SKIP LOCKED
		)
		UPDATE emails e SET attempts = e.attempts + 1, send_after = now() + interval '10 minutes'
		FROM due WHERE e.id = due.id
		RETURNING e.id, e.lead_id, e.subject, e.body, e.to_email, e.status, e.error, COALESCE((SELECT name FROM users WHERE id = e.sent_by), ''), e.attempts, e.created_at, e.sent_at`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Email
	for rows.Next() {
		e, err := scanEmail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkEmailSent records a delivered message.
func (s *Store) MarkEmailSent(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE emails SET status = 'sent', error = '', sent_at = now() WHERE id = $1`, id)
	return err
}

// MarkEmailFailed records a failed attempt. After final it will not be tried again.
func (s *Store) MarkEmailFailed(ctx context.Context, id, msg string, final bool) error {
	status := "queued"
	if final {
		status = "failed"
	}
	_, err := s.pool.Exec(ctx, `UPDATE emails SET status = $2, error = $3 WHERE id = $1`, id, status, msg)
	return err
}
