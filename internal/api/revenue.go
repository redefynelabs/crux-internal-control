package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/store"
)

func (a *API) analytics(w http.ResponseWriter, r *http.Request) {
	months, _ := strconv.Atoi(r.URL.Query().Get("months"))
	if months == 0 {
		months = 12
	}
	an, err := a.store.Analytics(r.Context(), months)
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, an)
}

func notFoundSale(w http.ResponseWriter, err error) bool {
	if errors.Is(err, store.ErrNotFound) || isBadID(err) {
		writeError(w, 404, "not_found", "No such sale")
		return true
	}
	return false
}

func (a *API) listSales(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status, billing := q.Get("status"), q.Get("billing")
	if status != "" && status != store.SaleActive && status != store.SaleCancelled {
		writeError(w, 400, "bad_request", "Unknown status")
		return
	}
	if billing != "" && billing != "recurring" && !contains(store.Billings, billing) {
		writeError(w, 400, "bad_request", "Unknown billing")
		return
	}
	ss, err := a.store.ListSales(r.Context(), store.SaleFilter{Status: status, Billing: billing, Owner: resolveOwner(q.Get("owner"), r), Query: q.Get("q"), Limit: queryLimit(r)})
	if isBadID(err) {
		writeError(w, 400, "bad_request", "Unknown person")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, ss)
}

func (a *API) getSale(w http.ResponseWriter, r *http.Request) {
	s, err := a.store.GetSale(r.Context(), r.PathValue("id"))
	if notFoundSale(w, err) {
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, s)
}

type saleJSON struct {
	LeadID    string `json:"lead_id"`
	Customer  string `json:"customer"`
	Company   string `json:"company"`
	Email     string `json:"email"`
	Plan      string `json:"plan"`
	Billing   string `json:"billing"`
	Amount    int64  `json:"amount"`
	StartedOn string `json:"started_on"`
	OwnerID   string `json:"owner_id"`
	Notes     string `json:"notes"`
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// checkSale returns the field and message of the first thing wrong with a sale's values, if any.
func checkSale(customer, email, plan, billing string, amount int64) (field, msg string) {
	switch {
	case strings.TrimSpace(customer) == "" || len(customer) > 160:
		return "customer", "Enter the customer's name"
	case email != "" && !validEmail(email):
		return "email", "Enter a valid email address"
	case !contains(store.SalePlans, plan):
		return "plan", "Choose one of the options"
	case !contains(store.Billings, billing):
		return "billing", "Choose how it is billed"
	case amount <= 0 || amount > 1_000_000_000:
		return "amount", "Enter the amount"
	}
	return "", ""
}

func (a *API) createSale(w http.ResponseWriter, r *http.Request) {
	var in saleJSON
	if !decode(w, r, &in) {
		return
	}
	if f, m := checkSale(in.Customer, in.Email, in.Plan, in.Billing, in.Amount); f != "" {
		invalid(w, f, m)
		return
	}
	if in.StartedOn != "" && !validDate(in.StartedOn) {
		invalid(w, "started_on", "Choose a date")
		return
	}
	if len(in.Company) > 160 || len(in.Notes) > 5000 {
		invalid(w, "company", "Too long")
		return
	}
	if in.OwnerID == "" {
		in.OwnerID = who(r).User.ID
	}
	sale, err := a.store.CreateSale(r.Context(), store.SaleInput{
		LeadID: in.LeadID, Customer: strings.TrimSpace(in.Customer), Company: strings.TrimSpace(in.Company), Email: in.Email, Plan: in.Plan,
		Billing: in.Billing, Amount: in.Amount, StartedOn: in.StartedOn, OwnerID: in.OwnerID, Notes: in.Notes,
	})
	if isBadID(err) || (err != nil && strings.Contains(err.Error(), "violates foreign key constraint")) {
		invalid(w, "owner_id", "Unknown lead or person")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	// A sale closes its lead: it moves to won and keeps what was sold in its history.
	if sale.LeadID != nil {
		ctx, by := r.Context(), who(r).actor()
		won := store.LeadWon
		v := sale.Amount
		if sale.Billing == store.BillingMonthly {
			v *= 12
		}
		_, _ = a.store.UpdateLead(ctx, *sale.LeadID, store.LeadPatch{Status: &won, ExpectedValue: &v}, by)
		_ = a.store.AddEventBy(ctx, *sale.LeadID, by, "sale_recorded", map[string]any{"amount": sale.Amount, "billing": sale.Billing, "plan": sale.Plan})
	}
	writeJSON(w, 201, sale)
}

type salePatchJSON struct {
	Customer     *string `json:"customer"`
	Company      *string `json:"company"`
	Email        *string `json:"email"`
	Plan         *string `json:"plan"`
	Billing      *string `json:"billing"`
	Amount       *int64  `json:"amount"`
	StartedOn    *string `json:"started_on"`
	OwnerID      *string `json:"owner_id"`
	Notes        *string `json:"notes"`
	Status       *string `json:"status"`
	EndedOn      *string `json:"ended_on"`
	CancelReason *string `json:"cancel_reason"`
}

func (a *API) patchSale(w http.ResponseWriter, r *http.Request) {
	var in salePatchJSON
	if !decode(w, r, &in) {
		return
	}
	switch {
	case in.Customer != nil && (strings.TrimSpace(*in.Customer) == "" || len(*in.Customer) > 160):
		invalid(w, "customer", "Enter the customer's name")
	case in.Email != nil && *in.Email != "" && !validEmail(*in.Email):
		invalid(w, "email", "Enter a valid email address")
	case in.Plan != nil && !contains(store.SalePlans, *in.Plan):
		invalid(w, "plan", "Choose one of the options")
	case in.Billing != nil && !contains(store.Billings, *in.Billing):
		invalid(w, "billing", "Choose how it is billed")
	case in.Amount != nil && (*in.Amount <= 0 || *in.Amount > 1_000_000_000):
		invalid(w, "amount", "Enter the amount")
	case in.StartedOn != nil && !validDate(*in.StartedOn):
		invalid(w, "started_on", "Choose a date")
	case in.EndedOn != nil && !validDate(*in.EndedOn):
		invalid(w, "ended_on", "Choose a date")
	case in.Status != nil && *in.Status != store.SaleActive && *in.Status != store.SaleCancelled:
		invalid(w, "status", "Unknown status")
	case (in.Notes != nil && len(*in.Notes) > 5000) || (in.CancelReason != nil && len(*in.CancelReason) > 500) || (in.Company != nil && len(*in.Company) > 160):
		invalid(w, "notes", "Too long")
	default:
		old, err := a.store.GetSale(r.Context(), r.PathValue("id"))
		if notFoundSale(w, err) {
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		if in.Email != nil {
			e := strings.ToLower(strings.TrimSpace(*in.Email))
			in.Email = &e
		}
		sale, err := a.store.UpdateSale(r.Context(), old.ID, store.SalePatch{
			Customer: in.Customer, Company: in.Company, Email: in.Email, Plan: in.Plan, Billing: in.Billing, Notes: in.Notes, Amount: in.Amount,
			StartedOn: in.StartedOn, OwnerID: in.OwnerID, Status: in.Status, EndedOn: in.EndedOn, CancelReason: in.CancelReason,
		})
		if isBadID(err) || (err != nil && strings.Contains(err.Error(), "violates foreign key constraint")) {
			invalid(w, "owner_id", "Unknown person")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		if sale.LeadID != nil && in.Status != nil && *in.Status != old.Status {
			kind := "sale_cancelled"
			if *in.Status == store.SaleActive {
				kind = "sale_reactivated"
			}
			_ = a.store.AddEventBy(r.Context(), *sale.LeadID, who(r).actor(), kind, map[string]any{"reason": sale.CancelReason})
		}
		writeJSON(w, 200, sale)
	}
}

func (a *API) deleteSale(w http.ResponseWriter, r *http.Request) {
	err := a.store.DeleteSale(r.Context(), r.PathValue("id"))
	if notFoundSale(w, err) {
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
