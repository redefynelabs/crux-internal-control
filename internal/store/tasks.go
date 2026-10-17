package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Task kinds.
var TaskKinds = []string{"call", "email", "other"}

// Task is a follow-up a person must do for a lead.
type Task struct {
	ID           string     `json:"id"`
	LeadID       string     `json:"lead_id"`
	LeadName     string     `json:"lead_name"`
	Title        string     `json:"title"`
	Kind         string     `json:"kind"`
	DueAt        time.Time  `json:"due_at"`
	AssigneeID   *string    `json:"assignee_id"`
	AssigneeName string     `json:"assignee_name"`
	DoneAt       *time.Time `json:"done_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

const taskSelect = `
	SELECT t.id, t.lead_id, l.name, t.title, t.kind, t.due_at, t.assignee_id, COALESCE(u.name, ''), t.done_at, t.created_at
	FROM lead_tasks t JOIN leads l ON l.id = t.lead_id LEFT JOIN users u ON u.id = t.assignee_id`

type rowScanner interface{ Scan(dest ...any) error }

func scanTask(r rowScanner) (Task, error) {
	var t Task
	err := r.Scan(&t.ID, &t.LeadID, &t.LeadName, &t.Title, &t.Kind, &t.DueAt, &t.AssigneeID, &t.AssigneeName, &t.DoneAt, &t.CreatedAt)
	return t, err
}

// CreateTask adds a follow-up. An empty assignee leaves it with nobody.
func (s *Store) CreateTask(ctx context.Context, leadID, title, kind string, due time.Time, assigneeID, createdBy string) (Task, error) {
	var id string
	if err := s.pool.QueryRow(ctx, `INSERT INTO lead_tasks (lead_id, title, kind, due_at, assignee_id, created_by) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		leadID, title, kind, due, nullable(assigneeID), nullable(createdBy)).Scan(&id); err != nil {
		return Task{}, err
	}
	return s.GetTask(ctx, id)
}

// GetTask returns one task.
func (s *Store) GetTask(ctx context.Context, id string) (Task, error) {
	t, err := scanTask(s.pool.QueryRow(ctx, taskSelect+` WHERE t.id = $1`, id))
	return t, notFound(err)
}

// TaskFilter narrows the task list.
type TaskFilter struct {
	Assignee string // a user id, or "" for everyone
	LeadID   string
	State    string // "open" (default), "done", "overdue", "today"
	Limit    int
}

// ListTasks returns tasks, earliest due first (done tasks: most recent first).
func (s *Store) ListTasks(ctx context.Context, f TaskFilter) ([]Task, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 200
	}
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	if f.Assignee != "" {
		add("t.assignee_id = ?", f.Assignee)
	}
	if f.LeadID != "" {
		add("t.lead_id = ?", f.LeadID)
	}
	order := "t.due_at"
	switch f.State {
	case "done":
		where = append(where, "t.done_at IS NOT NULL")
		order = "t.done_at DESC"
	case "overdue":
		where = append(where, "t.done_at IS NULL", "t.due_at < now()")
	case "today":
		where = append(where, "t.done_at IS NULL", "t.due_at < date_trunc('day', now()) + interval '1 day'")
	case "all":
	default:
		where = append(where, "t.done_at IS NULL")
	}
	sql := taskSelect
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, f.Limit)
	sql += fmt.Sprintf(" ORDER BY %s LIMIT $%d", order, len(args))
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TaskPatch changes the fields that are set. AssigneeID "" removes the assignee.
type TaskPatch struct {
	Done       *bool
	DueAt      *time.Time
	Title      *string
	AssigneeID *string
}

// UpdateTask applies a patch.
func (s *Store) UpdateTask(ctx context.Context, id string, p TaskPatch) (Task, error) {
	var assignee any
	setAssignee := p.AssigneeID != nil
	if setAssignee {
		assignee = nullable(*p.AssigneeID)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE lead_tasks SET
			title = COALESCE($2, title), due_at = COALESCE($3, due_at),
			assignee_id = CASE WHEN $4 THEN $5::uuid ELSE assignee_id END,
			done_at = CASE WHEN $6::boolean IS NULL THEN done_at WHEN $6 THEN COALESCE(done_at, now()) ELSE NULL END
		WHERE id = $1`, id, p.Title, p.DueAt, setAssignee, assignee, p.Done)
	if err != nil {
		return Task{}, err
	}
	if tag.RowsAffected() == 0 {
		return Task{}, ErrNotFound
	}
	return s.GetTask(ctx, id)
}

// DeleteTask removes a task.
func (s *Store) DeleteTask(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM lead_tasks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
