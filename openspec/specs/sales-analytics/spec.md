# sales-analytics Specification

## Purpose

Records what was sold and shows recurring revenue, churn and sales performance.

## Requirements

### Requirement: Sale record
A sale SHALL have a customer, billing (`monthly`, `yearly` or `one_time`), a non-negative whole-unit amount, a start date, a status (`active` or `cancelled`) and optionally a plan, company, email, owner and linked lead. Creating one linked to a lead SHALL set that lead to `won`. Only admins SHALL delete a sale.

#### Scenario: Cancel and return
- **WHEN** a sale is set to `cancelled` with a reason and later set to `active`
- **THEN** cancelling stamps `ended_on` and removes its value from MRR; reactivating clears `ended_on` and the reason and restores it

### Requirement: Revenue figures
`GET /v1/analytics?months=N` (default 12) SHALL return MRR (active monthly amounts plus one twelfth of active yearly amounts), ARR (12 × MRR), customers, ARPA, new and churned MRR over 30 days, churn rate over 90 days, 12-month one-time revenue (never part of MRR), pipeline and stage-weighted pipeline, win rate, average deal, average days to close, task completion figures, an N-month series, the funnel, leads and wins by source, MRR by plan, and a per-owner table.

#### Scenario: Mixed sales
- **WHEN** a 10,000 monthly sale, a 120,000 yearly sale and a 250,000 one-time sale are recorded
- **THEN** MRR is 20,000, ARR 240,000, customers 2 and 12-month one-time revenue 250,000
