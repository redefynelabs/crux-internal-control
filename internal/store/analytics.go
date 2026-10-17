package store

import (
	"context"
	"fmt"
	"math"
)

var sprintf = fmt.Sprintf

// MonthPoint is one month of the trend charts.
type MonthPoint struct {
	Month      string `json:"month"` // YYYY-MM
	MRR        int64  `json:"mrr"`   // recurring revenue at the end of the month (today, for this month)
	NewMRR     int64  `json:"new_mrr"`
	ChurnedMRR int64  `json:"churned_mrr"`
	Customers  int    `json:"customers"`
	OneTime    int64  `json:"one_time"`
	NewLeads   int    `json:"new_leads"`
	Won        int    `json:"won"`
	Demos      int    `json:"demos"`
}

// FunnelStep is how many leads got as far as a stage.
type FunnelStep struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// SourceRow is leads and wins by where they came from.
type SourceRow struct {
	Source string `json:"source"`
	Leads  int    `json:"leads"`
	Won    int    `json:"won"`
}

// PlanRow is recurring revenue by plan.
type PlanRow struct {
	Plan      string `json:"plan"`
	MRR       int64  `json:"mrr"`
	Customers int    `json:"customers"`
}

// OwnerRow is how each salesperson is doing.
type OwnerRow struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Leads        int    `json:"leads"`
	Won          int    `json:"won"`
	SoldMRR      int64  `json:"sold_mrr"`
	OpenTasks    int    `json:"open_tasks"`
	OverdueTasks int    `json:"overdue_tasks"`
	DoneTasks30  int    `json:"done_tasks_30"`
}

// Analytics is everything the analytics page shows.
type Analytics struct {
	MRR              int64   `json:"mrr"`
	ARR              int64   `json:"arr"`
	Customers        int     `json:"customers"`
	ARPA             int64   `json:"arpa"` // average recurring revenue per customer, monthly
	NewMRR30         int64   `json:"new_mrr_30"`
	ChurnedMRR30     int64   `json:"churned_mrr_30"`
	ChurnRate90      float64 `json:"churn_rate_90"` // percent of customers lost in 90 days
	OneTime12m       int64   `json:"one_time_12m"`
	PipelineValue    int64   `json:"pipeline_value"`
	WeightedPipeline int64   `json:"weighted_pipeline"`
	OpenLeads        int     `json:"open_leads"`
	WinRate          float64 `json:"win_rate"` // percent of closed leads that were won
	AvgDeal          int64   `json:"avg_deal"` // average yearly value of a recurring sale
	AvgDaysToClose   float64 `json:"avg_days_to_close"`
	TasksDone30      int     `json:"tasks_done_30"`
	TasksOnTime30    float64 `json:"tasks_on_time_30"` // percent of tasks finished by their due time

	Series  []MonthPoint `json:"series"`
	Funnel  []FunnelStep `json:"funnel"`
	Sources []SourceRow  `json:"sources"`
	Plans   []PlanRow    `json:"plans"`
	Owners  []OwnerRow   `json:"owners"`
}

// stageWeight is the chance a lead in each open stage turns into a sale, for the weighted pipeline.
var stageWeight = map[string]float64{
	LeadNew: 0.05, LeadDemoActive: 0.1, LeadDemoEnded: 0.1, LeadContacted: 0.2, LeadQualified: 0.4, LeadProposal: 0.7,
}

// stageRank is how far along a status is; a lead that was lost keeps the rank of the furthest stage it reached.
const stageRank = `CASE %s WHEN 'contacted' THEN 2 WHEN 'qualified' THEN 3 WHEN 'proposal' THEN 4 WHEN 'won' THEN 5 ELSE 0 END`

func pct(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return math.Round(float64(part)/float64(whole)*1000) / 10
}

// Analytics computes the figures over the last months months (3 to 24).
func (s *Store) Analytics(ctx context.Context, months int) (Analytics, error) {
	if months < 3 {
		months = 3
	}
	if months > 24 {
		months = 24
	}
	var a Analytics

	// Month by month. A sale counts toward a month's recurring revenue while it has started and not yet ended on the
	// month's closing day (today, for the current month).
	rows, err := s.pool.Query(ctx, `
		WITH months AS (
			SELECT m::date AS start, LEAST((m + interval '1 month')::date, current_date + 1) AS stop
			FROM generate_series(date_trunc('month', now()) - make_interval(months => $1 - 1), date_trunc('month', now()), interval '1 month') m
		)
		SELECT to_char(mo.start, 'YYYY-MM'),
		  COALESCE((SELECT sum(`+mrrExpr+`) FROM sales s WHERE s.started_on < mo.stop AND (s.ended_on IS NULL OR s.ended_on >= mo.stop)), 0)::bigint,
		  COALESCE((SELECT sum(`+mrrExpr+`) FROM sales s WHERE s.started_on >= mo.start AND s.started_on < mo.stop), 0)::bigint,
		  COALESCE((SELECT sum(`+mrrExpr+`) FROM sales s WHERE s.ended_on >= mo.start AND s.ended_on < mo.stop), 0)::bigint,
		  (SELECT count(*) FROM sales s WHERE s.billing <> 'one_time' AND s.started_on < mo.stop AND (s.ended_on IS NULL OR s.ended_on >= mo.stop)),
		  COALESCE((SELECT sum(s.amount) FROM sales s WHERE s.billing = 'one_time' AND s.status = 'active' AND s.started_on >= mo.start AND s.started_on < mo.stop), 0)::bigint,
		  (SELECT count(*) FROM leads l WHERE l.created_at >= mo.start AND l.created_at < mo.start + interval '1 month'),
		  (SELECT count(*) FROM sales s WHERE s.started_on >= mo.start AND s.started_on < mo.stop),
		  (SELECT count(*) FROM demos d WHERE d.created_at >= mo.start AND d.created_at < mo.start + interval '1 month')
		FROM months mo ORDER BY mo.start`, months)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var p MonthPoint
		if err := rows.Scan(&p.Month, &p.MRR, &p.NewMRR, &p.ChurnedMRR, &p.Customers, &p.OneTime, &p.NewLeads, &p.Won, &p.Demos); err != nil {
			rows.Close()
			return a, err
		}
		a.Series = append(a.Series, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return a, err
	}
	if n := len(a.Series); n > 0 {
		last := a.Series[n-1]
		a.MRR, a.Customers = last.MRR, last.Customers
	}
	a.ARR = a.MRR * 12
	if a.Customers > 0 {
		a.ARPA = a.MRR / int64(a.Customers)
	}

	var lost90, atStart90 int
	err = s.pool.QueryRow(ctx, `
		SELECT
		  COALESCE((SELECT sum(`+mrrExpr+`) FROM sales s WHERE s.started_on >= current_date - 30), 0)::bigint,
		  COALESCE((SELECT sum(`+mrrExpr+`) FROM sales s WHERE s.ended_on >= current_date - 30), 0)::bigint,
		  (SELECT count(*) FROM sales s WHERE s.billing <> 'one_time' AND s.ended_on >= current_date - 90),
		  (SELECT count(*) FROM sales s WHERE s.billing <> 'one_time' AND s.started_on < current_date - 90 AND (s.ended_on IS NULL OR s.ended_on >= current_date - 90)),
		  COALESCE((SELECT sum(s.amount) FROM sales s WHERE s.billing = 'one_time' AND s.status = 'active' AND s.started_on >= current_date - 365), 0)::bigint,
		  COALESCE((SELECT round(avg(CASE s.billing WHEN 'monthly' THEN s.amount * 12 ELSE s.amount END)) FROM sales s WHERE s.billing <> 'one_time'), 0)::bigint,
		  COALESCE((SELECT avg(s.started_on - l.created_at::date) FROM sales s JOIN leads l ON l.id = s.lead_id), 0)::float8,
		  (SELECT count(*) FROM lead_tasks WHERE done_at >= now() - interval '30 days')`).
		Scan(&a.NewMRR30, &a.ChurnedMRR30, &lost90, &atStart90, &a.OneTime12m, &a.AvgDeal, &a.AvgDaysToClose, &a.TasksDone30)
	if err != nil {
		return a, err
	}
	var onTime int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM lead_tasks WHERE done_at >= now() - interval '30 days' AND done_at <= due_at`).Scan(&onTime); err != nil {
		return a, err
	}
	a.TasksOnTime30 = pct(onTime, a.TasksDone30)
	a.ChurnRate90 = pct(lost90, atStart90)
	a.AvgDaysToClose = math.Round(a.AvgDaysToClose*10) / 10

	// Pipeline and win rate from where leads stand now.
	prows, err := s.pool.Query(ctx, `SELECT status, count(*), COALESCE(sum(expected_value), 0)::bigint FROM leads GROUP BY status`)
	if err != nil {
		return a, err
	}
	var won, lost int
	var weighted float64
	for prows.Next() {
		var st string
		var n int
		var v int64
		if err := prows.Scan(&st, &n, &v); err != nil {
			prows.Close()
			return a, err
		}
		switch st {
		case LeadWon:
			won = n
		case LeadLost:
			lost = n
		default:
			a.OpenLeads += n
			if st == LeadQualified || st == LeadProposal {
				a.PipelineValue += v
			}
			weighted += float64(v) * stageWeight[st]
		}
	}
	prows.Close()
	if err := prows.Err(); err != nil {
		return a, err
	}
	a.WeightedPipeline = int64(math.Round(weighted))
	a.WinRate = pct(won, won+lost)

	// Funnel: how far each lead ever got, counting the history of ones later lost.
	var total, demo, contacted, qualified, proposal, wonN int
	err = s.pool.QueryRow(ctx, `
		WITH reach AS (
			SELECT l.id,
			  GREATEST(`+sprintf(stageRank, "l.status")+`, CASE WHEN l.last_contact_at IS NOT NULL THEN 2 ELSE 0 END,
			    COALESCE((SELECT max(`+sprintf(stageRank, "e.detail->>'status'")+`) FROM lead_events e WHERE e.lead_id = l.id AND e.kind = 'status_changed'), 0)) AS rk,
			  EXISTS (SELECT 1 FROM demos d WHERE d.lead_id = l.id) AS had_demo
			FROM leads l
		)
		SELECT count(*), count(*) FILTER (WHERE had_demo), count(*) FILTER (WHERE rk >= 2), count(*) FILTER (WHERE rk >= 3),
		       count(*) FILTER (WHERE rk >= 4), count(*) FILTER (WHERE rk >= 5) FROM reach`).Scan(&total, &demo, &contacted, &qualified, &proposal, &wonN)
	if err != nil {
		return a, err
	}
	a.Funnel = []FunnelStep{
		{"leads", "Leads", total}, {"demo", "Tried a demo", demo}, {"contacted", "Contacted", contacted},
		{"qualified", "Qualified", qualified}, {"proposal", "Proposal", proposal}, {"won", "Won", wonN},
	}

	if a.Sources, err = queryRows(ctx, s, `
		SELECT source, count(*), count(*) FILTER (WHERE status = 'won') FROM leads GROUP BY source ORDER BY 2 DESC, 1`,
		func(r scanner, x *SourceRow) error { return r.Scan(&x.Source, &x.Leads, &x.Won) }); err != nil {
		return a, err
	}
	if a.Plans, err = queryRows(ctx, s, `
		SELECT s.plan, sum(`+mrrExpr+`)::bigint, count(*) FROM sales s
		WHERE s.status = 'active' AND s.billing <> 'one_time' GROUP BY s.plan ORDER BY 2 DESC`,
		func(r scanner, x *PlanRow) error { return r.Scan(&x.Plan, &x.MRR, &x.Customers) }); err != nil {
		return a, err
	}
	if a.Owners, err = queryRows(ctx, s, `
		SELECT u.id, u.name,
		  (SELECT count(*) FROM leads l WHERE l.owner_id = u.id),
		  (SELECT count(*) FROM leads l WHERE l.owner_id = u.id AND l.status = 'won'),
		  COALESCE((SELECT sum(`+mrrExpr+`) FROM sales s WHERE s.owner_id = u.id AND s.status = 'active'), 0)::bigint,
		  (SELECT count(*) FROM lead_tasks t WHERE t.assignee_id = u.id AND t.done_at IS NULL),
		  (SELECT count(*) FROM lead_tasks t WHERE t.assignee_id = u.id AND t.done_at IS NULL AND t.due_at < now()),
		  (SELECT count(*) FROM lead_tasks t WHERE t.assignee_id = u.id AND t.done_at >= now() - interval '30 days')
		FROM users u WHERE u.active AND u.password_hash <> '' ORDER BY 5 DESC, 4 DESC, lower(u.name)`,
		func(r scanner, x *OwnerRow) error {
			return r.Scan(&x.ID, &x.Name, &x.Leads, &x.Won, &x.SoldMRR, &x.OpenTasks, &x.OverdueTasks, &x.DoneTasks30)
		}); err != nil {
		return a, err
	}
	return a, nil
}

type scanner interface{ Scan(dest ...any) error }

// queryRows runs a query and scans every row into a T.
func queryRows[T any](ctx context.Context, s *Store, sql string, scan func(scanner, *T) error) ([]T, error) {
	rows, err := s.pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var x T
		if err := scan(rows, &x); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
