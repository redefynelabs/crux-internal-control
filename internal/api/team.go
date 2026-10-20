package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/mail"
	"github.com/redefynelabs/erp-control-plane/internal/store"
)

func (a *API) routes(mux *http.ServeMux) {
	const admin, sales = store.RoleAdmin, store.RoleSales
	signedIn := func(h http.HandlerFunc) http.HandlerFunc { return a.need(h) }
	staff := func(h http.HandlerFunc) http.HandlerFunc { return a.need(h, admin, sales) }
	adminOnly := func(h http.HandlerFunc) http.HandlerFunc { return a.need(h, admin) }

	mux.HandleFunc("POST /v1/auth/login", a.login)
	mux.HandleFunc("GET /v1/auth/invite/{token}", a.inviteInfo)
	mux.HandleFunc("POST /v1/auth/accept-invite", a.acceptInvite)
	mux.HandleFunc("POST /v1/auth/logout", signedIn(a.logout))
	mux.HandleFunc("GET /v1/me", signedIn(a.me))
	mux.HandleFunc("POST /v1/me/password", signedIn(a.changePassword))

	mux.HandleFunc("GET /v1/users", signedIn(a.listUsers))
	mux.HandleFunc("POST /v1/users", adminOnly(a.createUser))
	mux.HandleFunc("PATCH /v1/users/{id}", adminOnly(a.patchUser))
	mux.HandleFunc("POST /v1/users/{id}/invite", adminOnly(a.reinviteUser))

	mux.HandleFunc("GET /v1/stats", staff(a.stats))
	mux.HandleFunc("GET /v1/leads", staff(a.listLeads))
	mux.HandleFunc("POST /v1/leads", staff(a.createLead))
	mux.HandleFunc("GET /v1/leads/{id}", staff(a.getLead))
	mux.HandleFunc("PATCH /v1/leads/{id}", staff(a.patchLead))
	mux.HandleFunc("DELETE /v1/leads/{id}", adminOnly(a.deleteLead))
	mux.HandleFunc("POST /v1/leads/{id}/notes", staff(a.addNote))
	mux.HandleFunc("POST /v1/leads/{id}/calls", staff(a.logCall))
	mux.HandleFunc("POST /v1/leads/{id}/emails", staff(a.sendLeadEmail))
	mux.HandleFunc("POST /v1/leads/{id}/tasks", staff(a.addTask))
	mux.HandleFunc("GET /v1/tasks", staff(a.listTasks))
	mux.HandleFunc("PATCH /v1/tasks/{id}", staff(a.patchTask))
	mux.HandleFunc("DELETE /v1/tasks/{id}", staff(a.deleteTask))

	mux.HandleFunc("GET /v1/sales", staff(a.listSales))
	mux.HandleFunc("POST /v1/sales", staff(a.createSale))
	mux.HandleFunc("GET /v1/sales/{id}", staff(a.getSale))
	mux.HandleFunc("PATCH /v1/sales/{id}", staff(a.patchSale))
	mux.HandleFunc("DELETE /v1/sales/{id}", adminOnly(a.deleteSale))
	mux.HandleFunc("GET /v1/analytics", staff(a.analytics))

	mux.HandleFunc("GET /v1/templates", staff(a.listTemplates))
	mux.HandleFunc("PUT /v1/templates/{id}", adminOnly(a.putTemplate))
	mux.HandleFunc("GET /v1/settings/email", adminOnly(a.getEmailSettings))
	mux.HandleFunc("PUT /v1/settings/email", adminOnly(a.putEmailSettings))
	mux.HandleFunc("POST /v1/settings/email/test", adminOnly(a.testEmail))
	mux.HandleFunc("GET /v1/settings/general", adminOnly(a.getGeneral))
	mux.HandleFunc("PUT /v1/settings/general", adminOnly(a.putGeneral))

	a.entitlementRoutes(mux, staff, adminOnly)

	mux.HandleFunc("GET /v1/demos", staff(a.listDemos))
	mux.HandleFunc("POST /v1/demos/{id}/extend", staff(a.extendDemo))
	mux.HandleFunc("DELETE /v1/demos/{id}", staff(a.endDemo))
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	us, err := a.store.ListUsers(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, us)
}

type userJSON struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type invited struct {
	User      store.User `json:"user"`
	InviteURL string     `json:"invite_url"`
	EmailSent bool       `json:"email_sent"`
}

// invite makes an invite link for a person and, when mail works and the admin app's address is known, emails it.
func (a *API) invite(r *http.Request, u store.User) (invited, error) {
	tok, err := a.store.StartInvite(r.Context(), u.ID, inviteTTL)
	if err != nil {
		return invited{}, err
	}
	out := invited{User: u, InviteURL: "/invite/" + tok}
	if a.cfg.AdminURL != "" && a.mail.Ready(r.Context()) {
		ms, _ := a.mail.Settings(r.Context())
		link := strings.TrimRight(a.cfg.AdminURL, "/") + out.InviteURL
		vars := mail.Vars(u.Name, "", "", who(r).User.Name, ms.Signature)
		body := mail.Render("Hi {{first_name}},\n\n"+who(r).User.Name+" has added you to the team. Set your password here to sign in:\n\n"+link+"\n\nThe link works for 7 days.\n\n{{signature}}", vars)
		if _, err := a.mail.Queue(r.Context(), "", who(r).User.ID, u.Email, "You have been invited to the control plane", body, time.Now()); err == nil {
			out.EmailSent = true
		}
	}
	return out, nil
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var in userJSON
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
	if !contains(store.Roles, in.Role) {
		f["role"] = "Choose a role"
	}
	if len(f) > 0 {
		writeError(w, 422, "validation_failed", "Please check the highlighted fields", f)
		return
	}
	u, err := a.store.CreateUser(r.Context(), strings.TrimSpace(in.Name), in.Email, in.Role, "")
	if errors.Is(err, store.ErrEmailTaken) {
		writeError(w, 422, "validation_failed", "That email already belongs to someone", map[string]string{"email": "Already in use"})
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	out, err := a.invite(r, u)
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 201, out)
}

type userPatchJSON struct {
	Name   *string `json:"name"`
	Role   *string `json:"role"`
	Active *bool   `json:"active"`
}

func (a *API) patchUser(w http.ResponseWriter, r *http.Request) {
	var in userPatchJSON
	if !decode(w, r, &in) {
		return
	}
	if in.Role != nil && !contains(store.Roles, *in.Role) {
		writeError(w, 422, "validation_failed", "Choose a role", map[string]string{"role": "Choose a role"})
		return
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || len(n) > 120 {
			writeError(w, 422, "validation_failed", "Enter a name", map[string]string{"name": "Enter a name"})
			return
		}
		in.Name = &n
	}
	id := r.PathValue("id")
	cur, err := a.store.GetUser(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || isBadID(err) {
		writeError(w, 404, "not_found", "No such person")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	// Never leave the team without an active admin.
	losesAdmin := cur.Role == store.RoleAdmin && cur.Active && ((in.Role != nil && *in.Role != store.RoleAdmin) || (in.Active != nil && !*in.Active))
	if losesAdmin {
		if n, err := a.store.CountActiveAdmins(r.Context()); err != nil {
			a.internal(w, err)
			return
		} else if n <= 1 {
			writeError(w, 409, "last_admin", "There must be at least one active admin")
			return
		}
	}
	u, err := a.store.UpdateUser(r.Context(), id, store.UserPatch{Name: in.Name, Role: in.Role, Active: in.Active})
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, u)
}

func (a *API) reinviteUser(w http.ResponseWriter, r *http.Request) {
	u, err := a.store.GetUser(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) || isBadID(err) {
		writeError(w, 404, "not_found", "No such person")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	out, err := a.invite(r, u)
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, 200, out)
}
