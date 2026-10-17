package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Billing cycles and sale statuses.
const (
	BillingMonthly = "monthly"
	BillingYearly  = "yearly"
	BillingOneTime = "one_time"

	SaleActive    = "active"
	SaleCancelled = "cancelled"
)

// Billings lists how a sale can be charged; SalePlans the sizes it can be sold at.
var (
	Billings  = []string{BillingMonthly, BillingYearly, BillingOneTime}
	SalePlans = []string{"", "start", "grow", "complete", "small", "business", "larger", "custom"}
)

// mrrExpr is a sale's monthly recurring revenue: a yearly plan counts a twelfth each month, a one-time sale none.
const mrrExpr = `(CASE s.billing WHEN 'monthly' THEN s.amount WHEN 'yearly' THEN round(s.amount / 12.0) ELSE 0 END)::bigint`

// Sale is a customer who paid, or agreed to pay.
type Sale struct {
	ID           string    `json:"id"`
	LeadID       *string   `json:"lead_id"`
	Customer     string    `json:"customer"`
	Company      string    `json:"company"`
	Email        string    `json:"email"`
	Plan         string    `json:"plan"`
	Billing      string    `json:"billing"`
	Amount       int64     `json:"amount"`
	MRR          int64     `json:"mrr"`
	Status       string    `json:"status"`
	StartedOn    string    `json:"started_on"`
	EndedOn      *string   `json:"ended_on"`
	CancelReason string    `json:"cancel_reason"`
	OwnerID      *string   `json:"owner_id"`
	OwnerName    string    `json:"owner_name"`
	Notes        string    `json:"notes"`
	CreatedAt    time.Time `json:"created_at"`
}

const saleSelect = `
	SELECT s.id, s.lead_id, s.customer, s.company, s.email, s.plan, s.billing, s.amount, ` + mrrExpr + `, s.status,
	       to_char(s.started_on, 'YYYY-MM-DD'), to_char(s.ended_on, 'YYYY-MM-DD'), s.cancel_reason,
	       s.owner_id, COALESCE(u.name, ''), s.notes, s.created_at
	FROM sales s LEFT JOIN users u ON u.id = s.owner_id`

func scanSale(row pgx.Row) (Sale, error) {
	var s Sale
	err := row.Scan(&s.ID, &s.LeadID, &s.Customer, &s.Company, &s.Email, &s.Plan, &s.Billing, &s.Amount, &s.MRR, &s.Status,
		&s.StartedOn, &s.EndedOn, &s.CancelReason, &s.OwnerID, &s.OwnerName, &s.Notes, &s.CreatedAt)
	return s, err
}

// SaleInput is what recording a sale carries. StartedOn is YYYY-MM-DD, or "" for today.
type SaleInput struct {
	LeadID, Customer, Company, Email, Plan, Billing string
	Amount                                          int64
	StartedOn, OwnerID, Notes                       string
}

// CreateSale records a sale.
func (s *Store) CreateSale(ctx context.Context, in SaleInput) (Sale, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO sales (lead_id, customer, company, email, plan, billing, amount, started_on, owner_id, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE($8::date, current_date), $9, $10) RETURNING id`,
		nullable(in.LeadID), in.Customer, in.Company, normEmail(in.Email), in.Plan, in.Billing, in.Amount, nullable(in.StartedOn), nullable(in.OwnerID), in.Notes).Scan(&id)
	if err != nil {
		return Sale{}, err
	}
	return s.GetSale(ctx, id)
}

// GetSale returns one sale.
func (s *Store) GetSale(ctx context.Context, id string) (Sale, error) {
	sale, err := scanSale(s.pool.QueryRow(ctx, saleSelect+` WHERE s.id = $1`, id))
	return sale, notFound(err)
}

// SaleFilter narrows the sales list. Empty fields do not filter.
type SaleFilter struct {
	Status  string // active, cancelled
	Billing string // monthly, yearly, one_time, or "recurring"
	Owner   string
	Query   string
	Limit   int
}

// ListSales returns sales, newest first.
func (s *Store) ListSales(ctx context.Context, f SaleFilter) ([]Sale, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 200
	}
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	if f.Status != "" {
		add("s.status = ?", f.Status)
	}
	switch f.Billing {
	case "":
	case "recurring":
		where = append(where, "s.billing <> 'one_time'")
	default:
		add("s.billing = ?", f.Billing)
	}
	if f.Owner != "" {
		add("s.owner_id = ?", f.Owner)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		add("(s.customer ILIKE ? OR s.company ILIKE ? OR s.email ILIKE ?)", "%"+escapeLike(q)+"%")
	}
	sql := saleSelect
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, f.Limit)
	sql += fmt.Sprintf(" ORDER BY s.started_on DESC, s.created_at DESC LIMIT $%d", len(args))
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Sale{}
	for rows.Next() {
		sale, err := scanSale(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sale)
	}
	return out, rows.Err()
}

// SalePatch changes the fields that are set. OwnerID "" removes the owner. Cancelling needs a date (EndedOn, or today).
type SalePatch struct {
	Customer, Company, Email, Plan, Billing, Notes *string
	Amount                                         *int64
	StartedOn                                      *string
	OwnerID                                        *string
	Status                                         *string
	EndedOn                                        *string
	CancelReason                                   *string
}

// UpdateSale applies a patch. Reactivating clears the end date and reason.
func (s *Store) UpdateSale(ctx context.Context, id string, p SalePatch) (Sale, error) {
	var owner any
	setOwner := p.OwnerID != nil
	if setOwner {
		owner = nullable(*p.OwnerID)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE sales SET
			customer = COALESCE($2, customer), company = COALESCE($3, company), email = COALESCE($4, email),
			plan = COALESCE($5, plan), billing = COALESCE($6, billing), amount = COALESCE($7, amount),
			started_on = COALESCE($8::date, started_on),
			owner_id = CASE WHEN $9 THEN $10::uuid ELSE owner_id END,
			notes = COALESCE($11, notes),
			ended_on = CASE WHEN $12::text = 'cancelled' THEN COALESCE($13::date, ended_on, current_date) WHEN $12::text = 'active' THEN NULL ELSE ended_on END,
			cancel_reason = CASE WHEN $12::text = 'cancelled' THEN COALESCE($14, cancel_reason) WHEN $12::text = 'active' THEN '' ELSE cancel_reason END,
			status = COALESCE($12, status),
			updated_at = now()
		WHERE id = $1`,
		id, p.Customer, p.Company, p.Email, p.Plan, p.Billing, p.Amount, p.StartedOn, setOwner, owner, p.Notes, p.Status, p.EndedOn, p.CancelReason)
	if err != nil {
		return Sale{}, err
	}
	if tag.RowsAffected() == 0 {
		return Sale{}, ErrNotFound
	}
	return s.GetSale(ctx, id)
}

// DeleteSale removes a sale recorded by mistake.
func (s *Store) DeleteSale(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sales WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SaleForLead returns the active sale recorded for a lead, if any.
func (s *Store) SaleForLead(ctx context.Context, leadID string) (Sale, error) {
	sale, err := scanSale(s.pool.QueryRow(ctx, saleSelect+` WHERE s.lead_id = $1 ORDER BY s.created_at DESC LIMIT 1`, leadID))
	return sale, notFound(err)
}
