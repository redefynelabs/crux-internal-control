package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/mail"
	"github.com/redefynelabs/erp-control-plane/internal/sales"
	"github.com/redefynelabs/erp-control-plane/internal/store"
)

func (a *API) stats(w http.ResponseWriter, r *http.Request) {
	st, err := a.store.Stats(r.Context(), who(r).User.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, st)
}

func notFoundLead(w http.ResponseWriter, err error) bool {
	if errors.Is(err, store.ErrNotFound) || isBadID(err) {
		writeError(w, 404, "not_found", "No such lead")
		return true
	}
	return false
}

func invalid(w http.ResponseWriter, field, msg string) {
	writeError(w, 422, "validation_failed", msg, map[string]string{field: msg})
}

// resolveOwner turns ?owner=me into the caller's id.
func resolveOwner(v string, r *http.Request) string {
	if v == "me" {
		return who(r).User.ID
	}
	return v
}

func (a *API) listLeads(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.LeadFilter{Status: q.Get("status"), Owner: resolveOwner(q.Get("owner"), r), Interest: q.Get("interest"), Due: q.Get("due"), Query: q.Get("q"), Limit: queryLimit(r)}
	if f.Status != "" && !contains(store.LeadStatuses, f.Status) {
		writeError(w, 400, "bad_request", "Unknown status")
		return
	}
	if f.Interest != "" && !contains(store.Interests, f.Interest) {
		writeError(w, 400, "bad_request", "Unknown interest")
		return
	}
	if f.Due != "" && f.Due != "overdue" && f.Due != "today" {
		writeError(w, 400, "bad_request", "Unknown follow-up filter")
		return
	}
	leads, err := a.store.ListLeads(r.Context(), f)
	if isBadID(err) {
		writeError(w, 400, "bad_request", "Unknown owner")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, leads)
}

type leadCreateJSON struct {
	Name          string `json:"name"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	Company       string `json:"company"`
	Interest      string `json:"interest"`
	PlanInterest  string `json:"plan_interest"`
	TeamSize      string `json:"team_size"`
	ExpectedValue int64  `json:"expected_value"`
	OwnerID       string `json:"owner_id"`
}

func (a *API) createLead(w http.ResponseWriter, r *http.Request) {
	var in leadCreateJSON
	if !decode(w, r, &in) {
		return
	}
	f := map[string]string{}
	if n := strings.TrimSpace(in.Name); n == "" || len(n) > 120 {
		f["name"] = "Enter a name"
	}
	if !validEmail(in.Email) {
		f["email"] = "Enter a valid email address"
	}
	if !contains(store.Interests, in.Interest) {
		f["interest"] = "Choose one of the options"
	}
	if !contains(store.Plans, in.PlanInterest) {
		f["plan_interest"] = "Choose one of the options"
	}
	if in.ExpectedValue < 0 {
		f["expected_value"] = "Cannot be negative"
	}
	if len(in.Phone) > 40 || len(in.Company) > 160 || len(in.TeamSize) > 40 {
		f["phone"] = "Too long"
	}
	if len(f) > 0 {
		writeError(w, 422, "validation_failed", "Please check the highlighted fields", f)
		return
	}
	owner := in.OwnerID
	if owner == "" {
		owner = who(r).User.ID // a lead you add yourself is yours
	}
	l, created, err := a.store.UpsertLead(r.Context(), store.LeadInput{
		Name: strings.TrimSpace(in.Name), Email: in.Email, Phone: strings.TrimSpace(in.Phone), Company: strings.TrimSpace(in.Company), Source: "manual",
		Interest: in.Interest, PlanInterest: in.PlanInterest, TeamSize: in.TeamSize, ExpectedValue: in.ExpectedValue, OwnerID: owner,
	})
	if isBadID(err) {
		invalid(w, "owner_id", "Unknown person")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if created {
		_ = a.store.AddEventBy(r.Context(), l.ID, who(r).actor(), "lead_created", map[string]any{"source": "manual"})
	}
	writeJSON(w, 201, l)
}

// deleteLead removes a lead for good: its history, tasks, emails and demos go with it. A live demo is ended first. It
// refuses a lead that has a sale or a customer installation, so revenue and licences never lose their owner; mark such a
// lead lost instead.
func (a *API) deleteLead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l, err := a.store.GetLead(ctx, r.PathValue("id"))
	if notFoundLead(w, err) {
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	sales, installs, err := a.store.LeadDeleteBlockers(ctx, l.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	if sales > 0 || installs > 0 {
		writeError(w, 409, "has_records", "This lead has a sale or a customer installation, so it cannot be deleted. Mark it lost instead")
		return
	}
	live, err := a.store.LiveDemoIDs(ctx, l.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	for _, id := range live {
		// Remove the containers and database first: a demo row that is gone could never be cleaned up by the reaper.
		if err := a.demos.End(ctx, id); err != nil {
			a.internal(w, err)
			return
		}
	}
	if err := a.store.DeleteLead(ctx, l.ID); err != nil {
		a.internal(w, err)
		return
	}
	p := who(r)
	if err := a.store.Audit(ctx, p.User.ID, p.User.Email, "", "lead.deleted", map[string]any{"name": l.Name, "email": l.Email, "company": l.Company, "status": l.Status}); err != nil {
		a.log.Warn("could not record the lead deletion", "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) getLead(w http.ResponseWriter, r *http.Request) {
	l, err := a.store.GetLead(r.Context(), r.PathValue("id"))
	if notFoundLead(w, err) {
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	events, err := a.store.Events(r.Context(), l.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	tasks, err := a.store.ListTasks(r.Context(), store.TaskFilter{LeadID: l.ID, State: "all"})
	if err != nil {
		a.internal(w, err)
		return
	}
	emails, err := a.store.LeadEmails(r.Context(), l.ID)
	if err != nil {
		a.internal(w, err)
		return
	}
	var sale *store.Sale
	if sl, err := a.store.SaleForLead(r.Context(), l.ID); err == nil {
		sale = &sl
	} else if !errors.Is(err, store.ErrNotFound) {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"lead": l, "events": events, "tasks": tasks, "emails": emails, "sale": sale})
}

type leadPatchJSON struct {
	Status        *string `json:"status"`
	Notes         *string `json:"notes"`
	OwnerID       *string `json:"owner_id"`
	Interest      *string `json:"interest"`
	PlanInterest  *string `json:"plan_interest"`
	TeamSize      *string `json:"team_size"`
	ExpectedValue *int64  `json:"expected_value"`
	LostReason    *string `json:"lost_reason"`
	Name          *string `json:"name"`
	Phone         *string `json:"phone"`
	Company       *string `json:"company"`
}

func (a *API) patchLead(w http.ResponseWriter, r *http.Request) {
	var in leadPatchJSON
	if !decode(w, r, &in) {
		return
	}
	switch {
	case in.Status != nil && !contains(store.LeadStatuses, *in.Status):
		invalid(w, "status", "Unknown status")
	case in.Notes != nil && len(*in.Notes) > 10000:
		invalid(w, "notes", "Too long")
	case in.Interest != nil && !contains(store.Interests, *in.Interest):
		invalid(w, "interest", "Choose one of the options")
	case in.PlanInterest != nil && !contains(store.Plans, *in.PlanInterest):
		invalid(w, "plan_interest", "Choose one of the options")
	case in.ExpectedValue != nil && *in.ExpectedValue < 0:
		invalid(w, "expected_value", "Cannot be negative")
	case in.LostReason != nil && len(*in.LostReason) > 500:
		invalid(w, "lost_reason", "Too long")
	case in.Name != nil && (strings.TrimSpace(*in.Name) == "" || len(*in.Name) > 120):
		invalid(w, "name", "Enter a name")
	case (in.Phone != nil && len(*in.Phone) > 40) || (in.Company != nil && len(*in.Company) > 160) || (in.TeamSize != nil && len(*in.TeamSize) > 40):
		invalid(w, "phone", "Too long")
	default:
		l, err := a.store.UpdateLead(r.Context(), r.PathValue("id"), store.LeadPatch{
			Status: in.Status, Notes: in.Notes, OwnerID: in.OwnerID, Interest: in.Interest, PlanInterest: in.PlanInterest, TeamSize: in.TeamSize,
			ExpectedValue: in.ExpectedValue, LostReason: in.LostReason, Name: in.Name, Phone: in.Phone, Company: in.Company,
		}, who(r).actor())
		if notFoundLead(w, err) {
			return
		}
		if isBadID(err) {
			invalid(w, "owner_id", "Unknown person")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		writeJSON(w, 200, l)
	}
}

type textJSON struct {
	Text string `json:"text"`
}

func (a *API) addNote(w http.ResponseWriter, r *http.Request) {
	var in textJSON
	if !decode(w, r, &in) {
		return
	}
	text := strings.TrimSpace(in.Text)
	if text == "" || len(text) > 5000 {
		invalid(w, "text", "Write a note up to 5000 characters")
		return
	}
	a.addEvent(w, r, "note", map[string]any{"text": text}, false)
}

type callJSON struct {
	Outcome string `json:"outcome"`
	Text    string `json:"text"`
}

func (a *API) logCall(w http.ResponseWriter, r *http.Request) {
	var in callJSON
	if !decode(w, r, &in) {
		return
	}
	if !contains([]string{"reached", "no_answer", "voicemail", "wrong_number"}, in.Outcome) {
		invalid(w, "outcome", "Choose what happened")
		return
	}
	if len(in.Text) > 5000 {
		invalid(w, "text", "Too long")
		return
	}
	a.addEvent(w, r, "call", map[string]any{"outcome": in.Outcome, "text": strings.TrimSpace(in.Text)}, true)
}

// addEvent records something a person did on a lead and answers with the new entry.
func (a *API) addEvent(w http.ResponseWriter, r *http.Request, kind string, detail map[string]any, contact bool) {
	id := r.PathValue("id")
	if _, err := a.store.GetLead(r.Context(), id); notFoundLead(w, err) {
		return
	} else if err != nil {
		a.internal(w, err)
		return
	}
	if err := a.store.AddEventBy(r.Context(), id, who(r).actor(), kind, detail); err != nil {
		a.internal(w, err)
		return
	}
	if contact {
		_ = a.store.TouchContact(r.Context(), id)
	}
	writeJSON(w, 201, store.Event{Kind: kind, At: time.Now(), ActorName: who(r).User.Name})
}

type emailJSON struct {
	TemplateID string `json:"template_id"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
}

func (a *API) sendLeadEmail(w http.ResponseWriter, r *http.Request) {
	var in emailJSON
	if !decode(w, r, &in) {
		return
	}
	subject, body := strings.TrimSpace(in.Subject), strings.TrimSpace(in.Body)
	switch {
	case subject == "" || len(subject) > 200:
		invalid(w, "subject", "Enter a subject up to 200 characters")
		return
	case body == "" || len(body) > 20000:
		invalid(w, "body", "Write a message")
		return
	}
	l, err := a.store.GetLead(r.Context(), r.PathValue("id"))
	if notFoundLead(w, err) {
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	ms, err := a.mail.Settings(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	demoURL := ""
	if ds, err := a.store.ListDemos(r.Context(), "active", 500); err == nil {
		for _, d := range ds {
			if d.LeadID == l.ID {
				demoURL = d.URL
			}
		}
	}
	owner := l.OwnerName
	if p := who(r); p.User.Name != "" {
		owner = p.User.Name
	}
	vars := mail.Vars(l.Name, l.Company, demoURL, owner, ms.Signature)
	e, err := a.mail.Queue(r.Context(), l.ID, who(r).User.ID, l.Email, mail.Render(subject, vars), mail.Render(body, vars), time.Now())
	if errors.Is(err, mail.ErrNotConfigured) {
		writeError(w, 409, "email_not_configured", "Email is not set up yet. An admin can do it in Settings")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	// Send right away so the person sees the result; the worker retries if this attempt fails.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		a.mail.Work(ctx)
	}()
	writeJSON(w, 201, e)
}

type taskJSON struct {
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	DueAt      string `json:"due_at"`
	AssigneeID string `json:"assignee_id"`
}

func (a *API) addTask(w http.ResponseWriter, r *http.Request) {
	var in taskJSON
	if !decode(w, r, &in) {
		return
	}
	due, err := time.Parse(time.RFC3339, in.DueAt)
	switch {
	case strings.TrimSpace(in.Title) == "" || len(in.Title) > 200:
		invalid(w, "title", "Enter what needs doing")
		return
	case !contains(store.TaskKinds, in.Kind):
		invalid(w, "kind", "Choose one of the options")
		return
	case err != nil:
		invalid(w, "due_at", "Choose a date and time")
		return
	}
	assignee := in.AssigneeID
	if assignee == "" {
		assignee = who(r).User.ID
	}
	t, err := a.store.CreateTask(r.Context(), r.PathValue("id"), strings.TrimSpace(in.Title), in.Kind, due, assignee, who(r).User.ID)
	if isBadID(err) {
		invalid(w, "assignee_id", "Unknown person")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "violates foreign key constraint \"lead_tasks_lead_id_fkey\"") {
			writeError(w, 404, "not_found", "No such lead")
			return
		}
		a.internal(w, err)
		return
	}
	_ = a.store.AddEventBy(r.Context(), t.LeadID, who(r).actor(), "task_added", map[string]any{"title": t.Title, "due_at": t.DueAt})
	writeJSON(w, 201, t)
}

func (a *API) listTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	assignee := q.Get("assignee")
	if assignee == "me" {
		assignee = who(r).User.ID
	}
	if assignee == "all" {
		assignee = ""
	}
	state := q.Get("state")
	if state != "" && !contains([]string{"open", "done", "overdue", "today", "all"}, state) {
		writeError(w, 400, "bad_request", "Unknown state")
		return
	}
	ts, err := a.store.ListTasks(r.Context(), store.TaskFilter{Assignee: assignee, State: state, Limit: queryLimit(r)})
	if isBadID(err) {
		writeError(w, 400, "bad_request", "Unknown person")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, ts)
}

type taskPatchJSON struct {
	Done       *bool   `json:"done"`
	DueAt      *string `json:"due_at"`
	Title      *string `json:"title"`
	AssigneeID *string `json:"assignee_id"`
}

func (a *API) patchTask(w http.ResponseWriter, r *http.Request) {
	var in taskPatchJSON
	if !decode(w, r, &in) {
		return
	}
	p := store.TaskPatch{Done: in.Done, Title: in.Title, AssigneeID: in.AssigneeID}
	if in.DueAt != nil {
		due, err := time.Parse(time.RFC3339, *in.DueAt)
		if err != nil {
			invalid(w, "due_at", "Choose a date and time")
			return
		}
		p.DueAt = &due
	}
	if in.Title != nil && (strings.TrimSpace(*in.Title) == "" || len(*in.Title) > 200) {
		invalid(w, "title", "Enter what needs doing")
		return
	}
	t, err := a.store.UpdateTask(r.Context(), r.PathValue("id"), p)
	if errors.Is(err, store.ErrNotFound) || isBadID(err) {
		writeError(w, 404, "not_found", "No such task")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if in.Done != nil && *in.Done {
		_ = a.store.AddEventBy(r.Context(), t.LeadID, who(r).actor(), "task_done", map[string]any{"title": t.Title})
		if t.Kind != "other" {
			_ = a.store.TouchContact(r.Context(), t.LeadID)
		}
	}
	writeJSON(w, 200, t)
}

func (a *API) deleteTask(w http.ResponseWriter, r *http.Request) {
	err := a.store.DeleteTask(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) || isBadID(err) {
		writeError(w, 404, "not_found", "No such task")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listTemplates(w http.ResponseWriter, r *http.Request) {
	ts, err := a.store.ListTemplates(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, ts)
}

type templateJSON struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (a *API) putTemplate(w http.ResponseWriter, r *http.Request) {
	var in templateJSON
	if !decode(w, r, &in) {
		return
	}
	switch {
	case strings.TrimSpace(in.Name) == "" || len(in.Name) > 120:
		invalid(w, "name", "Enter a name")
	case strings.TrimSpace(in.Subject) == "" || len(in.Subject) > 200:
		invalid(w, "subject", "Enter a subject up to 200 characters")
	case strings.TrimSpace(in.Body) == "" || len(in.Body) > 20000:
		invalid(w, "body", "Write the message")
	default:
		t, err := a.store.UpdateTemplate(r.Context(), r.PathValue("id"), strings.TrimSpace(in.Name), strings.TrimSpace(in.Subject), in.Body)
		if errors.Is(err, store.ErrNotFound) || isBadID(err) {
			writeError(w, 404, "not_found", "No such template")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		writeJSON(w, 200, t)
	}
}

// emailView is the email settings as a browser sees them: never the password.
type emailView struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Security    string `json:"security"`
	Username    string `json:"username"`
	HasPassword bool   `json:"has_password"`
	FromName    string `json:"from_name"`
	FromEmail   string `json:"from_email"`
	ReplyTo     string `json:"reply_to"`
	Signature   string `json:"signature"`
}

func viewOf(s mail.Settings) emailView {
	return emailView{s.Enabled, s.Host, s.Port, s.Security, s.Username, s.PasswordEnc != "", s.FromName, s.FromEmail, s.ReplyTo, s.Signature}
}

func (a *API) getEmailSettings(w http.ResponseWriter, r *http.Request) {
	s, err := a.mail.Settings(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, viewOf(s))
}

type emailSettingsJSON struct {
	Enabled   bool   `json:"enabled"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Security  string `json:"security"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	FromName  string `json:"from_name"`
	FromEmail string `json:"from_email"`
	ReplyTo   string `json:"reply_to"`
	Signature string `json:"signature"`
}

func (a *API) putEmailSettings(w http.ResponseWriter, r *http.Request) {
	var in emailSettingsJSON
	if !decode(w, r, &in) {
		return
	}
	s := mail.Settings{Enabled: in.Enabled, Host: strings.TrimSpace(in.Host), Port: in.Port, Security: in.Security, Username: strings.TrimSpace(in.Username),
		FromName: strings.TrimSpace(in.FromName), FromEmail: strings.TrimSpace(in.FromEmail), ReplyTo: strings.TrimSpace(in.ReplyTo), Signature: in.Signature}
	// A form saved while switched off may be half filled in; only a switched-on form must be complete.
	if f := s.Validate(); len(f) > 0 && s.Enabled {
		writeError(w, 422, "validation_failed", "Please check the highlighted fields", f)
		return
	}
	saved, err := a.mail.Save(r.Context(), s, in.Password)
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, viewOf(saved))
}

type testJSON struct {
	To string `json:"to"`
}

func (a *API) testEmail(w http.ResponseWriter, r *http.Request) {
	var in testJSON
	if !decode(w, r, &in) {
		return
	}
	if !validEmail(in.To) {
		invalid(w, "to", "Enter a valid email address")
		return
	}
	err := a.mail.SendNow(r.Context(), in.To, "Test email from the control plane", "If you can read this, email is set up correctly.\n\nSent by "+who(r).User.Name+".")
	if err != nil {
		writeError(w, 422, "email_failed", "The test email could not be sent: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *API) getGeneral(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.sales.General(r.Context()))
}

func (a *API) putGeneral(w http.ResponseWriter, r *http.Request) {
	var in sales.General
	if !decode(w, r, &in) {
		return
	}
	if f := in.Validate(); len(f) > 0 {
		writeError(w, 422, "validation_failed", "Please check the highlighted fields", f)
		return
	}
	if err := a.store.PutSetting(r.Context(), sales.SettingsKey, in); err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, in)
}
